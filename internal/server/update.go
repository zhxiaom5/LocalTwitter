package server

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ErrAuthRequired = errors.New("auth is required")

type DownloadTask struct {
	Creator        Creator
	Username       string
	OutputDir      string
	VideoDir       string
	Auth           UpdateAuthSecret
	ReportProgress func(DownloadProgress)
}

type DownloadProgress struct {
	TotalTweets      int
	ProcessedTweets  int
	DownloadedVideos int
	SkippedTweets    int
	DownloadSpeedBps int64
	LastMessage      string
}

type DownloadResult struct {
	AddedWorks int
}

type UpdateDownloadTarget struct {
	RootDir  string
	VideoDir string
	Notice   string
}

type Downloader interface {
	UpdateCreator(context.Context, DownloadTask) (DownloadResult, error)
	ValidateAuth(context.Context, UpdateAuthSecret) error
}

type readyDownloader interface {
	Ready(context.Context) error
}

type CommandDownloader struct {
	path string
}

func NewCommandDownloader() CommandDownloader {
	if path := strings.TrimSpace(os.Getenv("LOCALTWITTER_DOWNLOADER")); path != "" {
		return CommandDownloader{path: path}
	}
	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		for _, candidate := range []string{
			filepath.Join(exeDir, "twitter-downloader", "start_download.sh"),
			filepath.Join(exeDir, "..", "packaging", "fnos", "localtwitter", "app", "twitter-downloader", "start_download.sh"),
		} {
			if fileExecutable(candidate) {
				return CommandDownloader{path: candidate}
			}
		}
		return CommandDownloader{path: filepath.Join(exeDir, "twitter-downloader", "start_download.sh")}
	}
	return CommandDownloader{path: filepath.Join("twitter-downloader", "start_download.sh")}
}

func (d CommandDownloader) Ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !fileExecutable(d.path) {
		return errors.New("内置下载器不可用，请确认程序目录包含 twitter-downloader/start_download.sh，或设置 LOCALTWITTER_DOWNLOADER")
	}
	return nil
}

func (d CommandDownloader) UpdateCreator(ctx context.Context, task DownloadTask) (DownloadResult, error) {
	return TwitterDownloader{}.UpdateCreator(ctx, task)
}

func (d CommandDownloader) ValidateAuth(ctx context.Context, auth UpdateAuthSecret) error {
	return TwitterDownloader{}.ValidateAuth(ctx, auth)
}

func validateTwitterAuth(auth UpdateAuthSecret) error {
	if strings.TrimSpace(auth.CookieHeader) != "" {
		return nil
	}
	if strings.TrimSpace(auth.AuthToken) == "" || strings.TrimSpace(auth.CT0) == "" {
		return ErrAuthRequired
	}
	return nil
}

func fileExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

type UpdateManager struct {
	store      *Store
	hub        *EventHub
	downloader Downloader

	mu              sync.Mutex
	activeWorkers   int
	status          UpdateStatus
	stop            chan struct{}
	currentCancels  map[int64]context.CancelFunc
	currentCreators map[int64]string
	resetVersion    int64
}

func NewUpdateManager(store *Store, hub *EventHub, downloader Downloader) *UpdateManager {
	if downloader == nil {
		if strings.TrimSpace(os.Getenv("LOCALTWITTER_DOWNLOADER")) != "" {
			downloader = NewCommandDownloader()
		} else {
			downloader = TwitterDownloader{}
		}
	}
	manager := &UpdateManager{
		store:           store,
		hub:             hub,
		downloader:      downloader,
		status:          UpdateStatus{State: "idle", Errors: []string{}},
		stop:            make(chan struct{}),
		currentCancels:  map[int64]context.CancelFunc{},
		currentCreators: map[int64]string{},
	}
	go manager.scheduleLoop()
	return manager
}

func (m *UpdateManager) Close() {
	close(m.stop)
}

func (m *UpdateManager) EnqueueImmediate(creatorIDs []int64, runID int64) (UpdateStatus, error) {
	userID, err := m.store.superAdminUserID()
	if err != nil {
		return UpdateStatus{}, err
	}
	return m.EnqueueImmediateForUser(creatorIDs, runID, userID)
}

func (m *UpdateManager) EnqueueImmediateForUser(creatorIDs []int64, runID, actorUserID int64) (UpdateStatus, error) {
	if err := m.ensureDownloaderReady(context.Background()); err != nil {
		return UpdateStatus{}, err
	}
	jobs, err := m.store.AddUpdateJobsForUser(creatorIDs, 0, runID, actorUserID, "manual")
	if err != nil {
		return UpdateStatus{}, err
	}
	m.mu.Lock()
	m.status.TotalCreators += len(jobs)
	m.status.PendingCreators += len(jobs)
	m.status.State = "queued"
	m.refreshJobsLocked()
	status := m.snapshotLocked()
	m.mu.Unlock()
	m.publish("queued", status, nil)
	m.startWorkers(actorUserID)
	return status, nil
}

func (m *UpdateManager) EnqueueQueueNow(queueID int64) (UpdateRun, error) {
	userID, err := m.store.superAdminUserID()
	if err != nil {
		return UpdateRun{}, err
	}
	return m.EnqueueQueueNowForUser(queueID, userID)
}

func (m *UpdateManager) EnqueueQueueNowForUser(queueID, actorUserID int64) (UpdateRun, error) {
	return m.enqueueQueueNowForUser(queueID, actorUserID, "manual")
}

func (m *UpdateManager) enqueueQueueNowForUser(queueID, actorUserID int64, source string) (UpdateRun, error) {
	source = normalizeUpdateJobSource(source)
	if err := m.ensureDownloaderReady(context.Background()); err != nil {
		return UpdateRun{}, err
	}
	queue, err := m.store.UpdateQueue(queueID)
	if err != nil {
		return UpdateRun{}, err
	}
	creators, err := m.store.CreatorsForUpdateQueue(queueID)
	if err != nil {
		return UpdateRun{}, err
	}
	run, err := m.store.CreateUpdateRunForUser(queue, len(creators), actorUserID)
	if err != nil {
		return UpdateRun{}, err
	}
	ids := make([]int64, 0, len(creators))
	for _, creator := range creators {
		ids = append(ids, creator.ID)
	}
	jobs, err := m.store.AddUpdateJobsForUser(ids, queueID, run.ID, actorUserID, source)
	if err != nil {
		return UpdateRun{}, err
	}
	m.mu.Lock()
	m.status.TotalCreators += len(jobs)
	m.status.PendingCreators += len(jobs)
	m.status.State = "queued"
	m.refreshJobsLocked()
	status := m.snapshotLocked()
	m.mu.Unlock()
	m.publish("queue_queued", status, nil)
	m.startWorkers(actorUserID)
	return run, nil
}

func (m *UpdateManager) ensureDownloaderReady(ctx context.Context) error {
	if checker, ok := m.downloader.(readyDownloader); ok {
		return checker.Ready(ctx)
	}
	return nil
}

func (m *UpdateManager) Status() UpdateStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshJobsLocked()
	return m.snapshotLocked()
}

func (m *UpdateManager) ResetImmediate(ctx context.Context) (UpdateStatus, error) {
	m.mu.Lock()
	m.resetVersion++
	for _, cancel := range m.currentCancels {
		cancel()
	}
	m.currentCancels = map[int64]context.CancelFunc{}
	m.currentCreators = map[int64]string{}
	m.status = UpdateStatus{State: "idle", Errors: []string{}}
	version := m.resetVersion
	m.mu.Unlock()
	if err := m.store.ClearUpdateJobs(); err != nil {
		return UpdateStatus{}, err
	}
	m.mu.Lock()
	if m.resetVersion == version {
		m.refreshJobsLocked()
	}
	status := m.snapshotLocked()
	m.mu.Unlock()
	m.publish("reset", status, nil)
	_ = ctx
	return status, nil
}

func (m *UpdateManager) ValidateAuth(ctx context.Context) error {
	userID, err := m.store.superAdminUserID()
	if err != nil {
		return err
	}
	return m.ValidateAuthForUser(ctx, userID)
}

func (m *UpdateManager) ValidateAuthForUser(ctx context.Context, userID int64) error {
	auth, err := m.store.updateAuthSecretForUser(userID)
	if err != nil {
		return err
	}
	if err := m.downloader.ValidateAuth(ctx, auth); err != nil {
		_ = m.store.MarkUpdateAuthValidationForUser(userID, "invalid", err.Error())
		return err
	}
	return m.store.MarkUpdateAuthValidationForUser(userID, "valid", "Auth 可用")
}

func (m *UpdateManager) startWorkers(userID int64) {
	auth, err := m.store.updateAuthSecretForUser(userID)
	concurrency := normalizeDownloaderConcurrency(1)
	if err == nil {
		concurrency = normalizeDownloaderConcurrency(auth.DownloaderConcurrency)
	}
	m.mu.Lock()
	toStart := concurrency - m.activeWorkers
	if toStart < 0 {
		toStart = 0
	}
	for i := 0; i < toStart; i++ {
		m.activeWorkers++
		go m.runWorker()
	}
	m.mu.Unlock()
}

func (m *UpdateManager) runWorker() {
	defer func() {
		m.mu.Lock()
		if m.activeWorkers > 0 {
			m.activeWorkers--
		}
		if m.activeWorkers == 0 {
			m.status.Running = false
			m.status.PendingCreators = 0
			m.status.CurrentCreator = ""
			if m.status.State != "failed" {
				m.status.State = "idle"
			}
			m.refreshJobsLocked()
			status := m.snapshotLocked()
			m.mu.Unlock()
			m.publish("completed", status, nil)
			return
		}
		m.mu.Unlock()
	}()
	for {
		job, err := m.store.ClaimNextPendingUpdateJob()
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		if err != nil {
			m.recordError("读取更新任务失败: " + err.Error())
			continue
		}
		m.execute(job)
	}
}

func (m *UpdateManager) execute(job UpdateJob) {
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.status.Running = true
	m.status.State = "running"
	m.currentCancels[job.ID] = cancel
	m.currentCreators[job.ID] = job.CreatorName
	m.status.CurrentCreator = strings.Join(currentCreatorNames(m.currentCreators), ", ")
	version := m.resetVersion
	if m.status.PendingCreators > 0 {
		m.status.PendingCreators--
	}
	m.refreshJobsLocked()
	status := m.snapshotLocked()
	m.mu.Unlock()
	m.publish("started", status, &job)

	addedWorks, err := m.updateCreator(ctx, job)
	cancel()
	m.mu.Lock()
	if m.resetVersion == version {
		delete(m.currentCancels, job.ID)
		delete(m.currentCreators, job.ID)
		m.status.CurrentCreator = strings.Join(currentCreatorNames(m.currentCreators), ", ")
	}
	if m.resetVersion != version {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	if err != nil {
		userMessage, detail := classifyUpdateError(err)
		_ = m.store.FinishUpdateJob(job.ID, "failed", 0, userMessage, detail)
		m.mu.Lock()
		m.status.FailedCreators++
		m.status.Errors = append(m.status.Errors, job.CreatorName+": "+userMessage)
		m.recomputeProgressLocked()
		m.refreshJobsLocked()
		status := m.snapshotLocked()
		m.mu.Unlock()
		m.publish("failed", status, &job)
		return
	}
	_ = m.store.FinishUpdateJob(job.ID, "succeeded", addedWorks, "")
	m.mu.Lock()
	m.status.SucceededCreators++
	m.status.AddedWorks += addedWorks
	m.recomputeProgressLocked()
	m.refreshJobsLocked()
	status = m.snapshotLocked()
	m.mu.Unlock()
	m.publish("succeeded", status, &job)
}

func (m *UpdateManager) RetryJob(id int64) (UpdateStatus, UpdateJob, bool, error) {
	userID, err := m.store.superAdminUserID()
	if err != nil {
		return UpdateStatus{}, UpdateJob{}, false, err
	}
	return m.RetryJobForUser(id, userID)
}

func (m *UpdateManager) RetryJobForUser(id, actorUserID int64) (UpdateStatus, UpdateJob, bool, error) {
	if err := m.ensureDownloaderReady(context.Background()); err != nil {
		return UpdateStatus{}, UpdateJob{}, false, err
	}
	job, retried, err := m.store.RetryUpdateJobForUser(id, actorUserID)
	if err != nil {
		return UpdateStatus{}, UpdateJob{}, false, err
	}
	m.mu.Lock()
	if retried {
		if m.status.TotalCreators == 0 {
			m.status.TotalCreators = 1
		}
		if m.status.FailedCreators > 0 {
			m.status.FailedCreators--
		}
		m.status.PendingCreators++
		m.status.State = "queued"
		m.recomputeProgressLocked()
	}
	m.refreshJobsLocked()
	status := m.snapshotLocked()
	m.mu.Unlock()
	m.publish("retry", status, &job)
	if retried {
		ownerID := job.CookieOwnerUserID
		if ownerID <= 0 {
			ownerID = job.ActorUserID
		}
		if ownerID <= 0 {
			ownerID, _ = m.store.superAdminUserID()
		}
		m.startWorkers(ownerID)
	}
	return status, job, retried, nil
}

func (m *UpdateManager) updateCreator(ctx context.Context, job UpdateJob) (int, error) {
	creator, err := m.store.Creator(job.CreatorID)
	if err != nil {
		return 0, err
	}
	username, profileURL, ok := canonicalTwitterIdentity(creator.Name, creator.TwitterUsername, creator.TwitterProfileURL)
	if !ok {
		return 0, errors.New("无法解析 Twitter 作者账号")
	}
	dir, err := m.store.Directory(creator.DirectoryID)
	if err != nil {
		return 0, err
	}
	beforeCount, err := m.store.CreatorWorkCount(creator.ID)
	if err != nil {
		return 0, err
	}
	authOwnerID := job.CookieOwnerUserID
	if authOwnerID <= 0 {
		authOwnerID = job.ActorUserID
	}
	if authOwnerID <= 0 {
		authOwnerID, _ = m.store.superAdminUserID()
	}
	auth, err := m.store.updateAuthSecretForUser(authOwnerID)
	if err != nil {
		return 0, err
	}
	target, err := resolveUpdateDownloadTarget(creator, dir, auth)
	if err != nil {
		return 0, err
	}
	if err := ensureWritableDirectory(target.RootDir); err != nil {
		return 0, err
	}
	_ = m.store.UpdateJobDownloadInfo(job.ID, target.VideoDir, target.Notice)
	progress := func(value DownloadProgress) {
		_ = m.store.UpdateJobProgress(job.ID, value)
		m.mu.Lock()
		m.refreshJobsLocked()
		status := m.snapshotLocked()
		m.mu.Unlock()
		current, err := m.store.UpdateJob(job.ID)
		if err != nil {
			m.publish("progress", status, &job)
			return
		}
		m.publish("progress", status, &current)
	}
	if shouldRefreshProfileBeforeUpdate(m.downloader) {
		progress(DownloadProgress{LastMessage: "正在刷新推主资料"})
		if err := RefreshCreatorProfile(ctx, m.store, creator, auth, nil); err == nil {
			if refreshed, err := m.store.Creator(creator.ID); err == nil {
				creator = refreshed
			}
		}
	}
	result, err := m.downloader.UpdateCreator(ctx, DownloadTask{Creator: creator, Username: username, OutputDir: target.RootDir, VideoDir: target.VideoDir, Auth: auth, ReportProgress: progress})
	downloadErr := err
	mediaDir := target.VideoDir
	works, err := scanCreatorWithoutVideoRepair(mediaDir)
	if err != nil {
		if downloadErr != nil {
			return result.AddedWorks, downloadErr
		}
		if !hasDownloadArtifacts(mediaDir) {
			return 0, userFacingUpdateError{
				Message: "未发现可下载视频或全部已跳过",
				Detail:  "下载器完成但未生成可入库视频文件；可能该账号没有视频、全部已存在或被过滤。",
			}
		}
		return 0, userFacingUpdateError{
			Message: "下载完成但入库失败，请检查 twmd 文件结构",
			Detail:  "自动入库失败，请检查文件命名、目录权限或扫描器识别规则: " + err.Error(),
		}
	}
	if len(works) == 0 {
		if downloadErr != nil {
			return result.AddedWorks, downloadErr
		}
		if hasDownloadArtifacts(mediaDir) {
			return 0, userFacingUpdateError{
				Message: "下载完成但入库失败，请检查 twmd 文件结构",
				Detail:  "下载目录存在文件，但扫描器没有识别到可入库视频作品。",
			}
		}
		return 0, userFacingUpdateError{
			Message: "未发现可下载视频或全部已跳过",
			Detail:  "下载器完成但未生成可入库视频文件；可能该账号没有视频、全部已存在或被过滤。",
		}
	}
	identity := CreatorIdentity{TwitterUsername: username, ProfileURL: profileURL}
	if strings.TrimSpace(creator.TwitterUserID) != "" {
		identity.TwitterUserID = creator.TwitterUserID
	}
	updatedCreator, _, err := m.store.UpsertCreatorWithWorksWithIdentity(dir.ID, creator.Name, identity, works)
	if err != nil {
		if downloadErr != nil {
			return result.AddedWorks, downloadErr
		}
		return 0, userFacingUpdateError{Message: "下载完成但入库失败，请检查 twmd 文件结构", Detail: "自动入库失败，写入数据库失败: " + err.Error()}
	}
	afterCount, err := m.store.CreatorWorkCount(updatedCreator.ID)
	if err != nil {
		if downloadErr != nil {
			return result.AddedWorks, downloadErr
		}
		return 0, userFacingUpdateError{Message: "下载完成但入库失败，请检查 twmd 文件结构", Detail: "自动入库失败，读取作品统计失败: " + err.Error()}
	}
	if afterCount > beforeCount {
		result.AddedWorks = afterCount - beforeCount
	} else {
		result.AddedWorks = 0
	}
	if downloadErr != nil {
		return result.AddedWorks, downloadErr
	}
	return result.AddedWorks, nil
}

func shouldRefreshProfileBeforeUpdate(downloader Downloader) bool {
	switch downloader.(type) {
	case TwitterDownloader, CommandDownloader:
		return true
	default:
		return false
	}
}

func normalizeUpdateDownloadMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "custom":
		return "custom"
	default:
		return "original"
	}
}

func resolveUpdateDownloadTarget(creator Creator, directory Directory, auth UpdateAuthSecret) (UpdateDownloadTarget, error) {
	username, _, ok := canonicalTwitterIdentity(creator.Name, creator.TwitterUsername, creator.TwitterProfileURL)
	if !ok {
		return UpdateDownloadTarget{}, errors.New("无法解析 Twitter 作者账号")
	}
	customRoot := strings.TrimSpace(auth.DownloadDirectory)
	mode := normalizeUpdateDownloadMode(auth.DownloadMode)
	if mode == "custom" {
		if customRoot == "" {
			return UpdateDownloadTarget{}, errors.New("请先设置自定义更新保存目录")
		}
		root, videoDir := normalizeTwitterDownloadTarget(customRoot, username)
		return UpdateDownloadTarget{RootDir: root, VideoDir: videoDir}, nil
	}
	root, videoDir, ok := originalDownloadTarget(directory.Path, username)
	if ok {
		return UpdateDownloadTarget{RootDir: root, VideoDir: videoDir}, nil
	}
	if customRoot != "" {
		root, videoDir := normalizeTwitterDownloadTarget(customRoot, username)
		return UpdateDownloadTarget{
			RootDir:  root,
			VideoDir: videoDir,
			Notice:   "未找到原始目录，已使用自定义保存目录",
		}, nil
	}
	return UpdateDownloadTarget{}, errors.New("该博主没有原始目录，请先在更新管理中设置保存目录")
}

func originalDownloadTarget(path string, username string) (string, string, bool) {
	root, videoDir := normalizeTwitterDownloadTarget(path, username)
	if directoryExists(videoDir) || directoryExists(filepath.Join(root, username)) {
		return root, chooseExistingCreatorVideoDir(root, username), true
	}
	return "", "", false
}

func normalizeTwitterDownloadTarget(path string, username string) (string, string) {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return clean, filepath.Join(clean, username, username, "video")
	}
	root := clean
	if strings.EqualFold(filepath.Base(clean), "video") && strings.EqualFold(filepath.Base(filepath.Dir(clean)), username) {
		parent := filepath.Dir(clean)
		grandparent := filepath.Dir(parent)
		if strings.EqualFold(filepath.Base(grandparent), username) {
			root = filepath.Dir(grandparent)
		} else {
			root = grandparent
		}
	} else if strings.EqualFold(filepath.Base(clean), username) {
		parent := filepath.Dir(clean)
		if strings.EqualFold(filepath.Base(parent), username) {
			root = filepath.Dir(parent)
		} else {
			root = parent
		}
	}
	return root, chooseExistingCreatorVideoDir(root, username)
}

func chooseExistingCreatorVideoDir(root string, username string) string {
	nested := filepath.Join(root, username, username, "video")
	flat := filepath.Join(root, username, "video")
	if directoryExists(nested) {
		return nested
	}
	if directoryExists(flat) {
		return flat
	}
	return nested
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

type userFacingUpdateError struct {
	Message string
	Detail  string
}

func (e userFacingUpdateError) Error() string {
	return e.Message
}

func classifyUpdateError(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	var retryErr videoDownloadRetryError
	if errors.As(err, &retryErr) {
		detail := retryErr.Error()
		lower := strings.ToLower(detail)
		if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") || strings.Contains(lower, "读取超时") {
			return "视频下载超时，已自动重试后仍失败", detail
		}
		return "视频下载失败，已自动重试后仍失败", detail
	}
	var friendly userFacingUpdateError
	if errors.As(err, &friendly) {
		detail := friendly.Detail
		if strings.TrimSpace(detail) == "" {
			detail = friendly.Message
		}
		return friendly.Message, detail
	}
	detail := err.Error()
	lower := strings.ToLower(detail)
	if strings.Contains(lower, "proxy") || strings.Contains(lower, "代理") || strings.Contains(lower, "socks") || strings.Contains(lower, "dial tcp") || strings.Contains(lower, "i/o timeout") || strings.Contains(lower, "timeout") || strings.Contains(lower, "no such host") || strings.Contains(lower, "dns") || strings.Contains(lower, "tls") || strings.Contains(lower, "connection refused") || strings.Contains(lower, "network") {
		return "网络或代理不可用，请检查代理地址", detail
	}
	if strings.Contains(lower, "401") || strings.Contains(lower, "403") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "could not authenticate") || strings.Contains(lower, "登录配置不可用") || strings.Contains(lower, "auth") {
		return "Twitter 登录配置不可用，请更新完整 Cookie 或重新登录 X", detail
	}
	return detail, detail
}

func hasDownloadArtifacts(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return true
		}
	}
	return false
}

func (m *UpdateManager) recordError(message string) {
	m.mu.Lock()
	m.status.Errors = append(m.status.Errors, message)
	m.status.State = "failed"
	status := m.snapshotLocked()
	m.mu.Unlock()
	m.publish("error", status, nil)
}

func (m *UpdateManager) scheduleLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-ticker.C:
			m.enqueueDueQueues(now.UTC())
		}
	}
}

func (m *UpdateManager) enqueueDueQueues(now time.Time) {
	queues, err := m.store.UpdateQueues()
	if err != nil {
		m.recordError("读取定时队列失败: " + err.Error())
		return
	}
	for _, queue := range queues {
		if !queue.Enabled || strings.TrimSpace(queue.NextRunAt) == "" {
			continue
		}
		next, err := time.Parse(time.RFC3339, queue.NextRunAt)
		if err != nil || next.After(now) {
			continue
		}
		userID, err := m.store.superAdminUserID()
		if err != nil {
			m.recordError("读取定时队列执行用户失败: " + err.Error())
			continue
		}
		_, _ = m.enqueueQueueNowForUser(queue.ID, userID, "scheduled")
		nextRun := NextCronRun(queue.Cron, now)
		_, _ = m.store.UpdateUpdateQueue(queue.ID, "", queue.Cron, nil, nil)
		if !nextRun.IsZero() {
			_, _ = m.store.db.Exec(`UPDATE update_queues SET next_run_at=? WHERE id=?`, nextRun.Format(time.RFC3339), queue.ID)
		}
	}
}

func (m *UpdateManager) publish(kind string, status UpdateStatus, job *UpdateJob) {
	if m.hub != nil {
		m.hub.PublishUpdate(UpdateEvent{Type: kind, Status: status, Job: job})
	}
}

func (m *UpdateManager) refreshJobsLocked() {
	jobs, err := m.store.UpdateJobs(30)
	if err == nil {
		m.status.Jobs = jobs
	}
	runningJobs, err := m.store.RunningUpdateJobs()
	if err == nil {
		m.status.RunningJobs = runningJobs
	}
}

func (m *UpdateManager) recomputeProgressLocked() {
	done := m.status.SucceededCreators + m.status.FailedCreators
	if m.status.TotalCreators > 0 {
		m.status.Progress = int(float64(done) / float64(m.status.TotalCreators) * 100)
	}
}

func (m *UpdateManager) snapshotLocked() UpdateStatus {
	status := m.status
	status.Errors = append([]string(nil), m.status.Errors...)
	status.RunningJobs = append([]UpdateJob(nil), m.status.RunningJobs...)
	status.Jobs = append([]UpdateJob(nil), m.status.Jobs...)
	return status
}

func currentCreatorNames(values map[int64]string) []string {
	names := make([]string, 0, len(values))
	for _, name := range values {
		if strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	return names
}

func normalizeDownloaderConcurrency(value int) int {
	if value <= 0 {
		return 1
	}
	if value > 8 {
		return 8
	}
	return value
}
