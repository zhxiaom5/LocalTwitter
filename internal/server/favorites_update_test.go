package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFavoriteFoldersAndStatuses(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	creator, _, err := store.UpsertCreatorWithWorks(dir.ID, "creator", []WorkInput{{
		AwemeID: "1001",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(t.TempDir(), "clip.mp4"), FileName: "clip.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	works, err := store.Feed(0, 0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	workFolders, err := store.FavoriteFolders("work")
	if err != nil {
		t.Fatal(err)
	}
	creatorFolders, err := store.FavoriteFolders("creator")
	if err != nil {
		t.Fatal(err)
	}
	if len(workFolders) != 1 || len(creatorFolders) != 1 {
		t.Fatalf("default favorite folders missing: works=%#v creators=%#v", workFolders, creatorFolders)
	}
	if err := store.AddWorkFavorite(works[0].ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := store.AddCreatorFavorite(creator.ID, 0); err != nil {
		t.Fatal(err)
	}
	status, err := store.FavoriteStatus(works[0].ID, creator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.WorkFavorited || !status.CreatorFavorited {
		t.Fatalf("favorite status = %#v", status)
	}
	favoriteWorks, err := store.FavoriteWorks(workFolders[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	favoriteCreators, err := store.FavoriteCreators(creatorFolders[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(favoriteWorks) != 1 || len(favoriteCreators) != 1 {
		t.Fatalf("favorite lists works=%#v creators=%#v", favoriteWorks, favoriteCreators)
	}
}

func TestTwitterCreatorNameIsReadyForOnlineUpdate(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpsertCreatorWithWorks(dir.ID, "Khazix0918", []WorkInput{{
		AwemeID: "1001",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(t.TempDir(), "clip.mp4"), FileName: "clip.mp4"}},
	}}); err != nil {
		t.Fatal(err)
	}
	creators, err := store.Creators()
	if err != nil {
		t.Fatal(err)
	}
	if len(creators) != 1 {
		t.Fatalf("creators = %#v", creators)
	}
	creator := creators[0]
	if creator.OnlineIdentityStatus != "ready" || creator.TwitterUsername != "Khazix0918" || creator.TwitterProfileURL != "https://x.com/Khazix0918" {
		t.Fatalf("creator identity = %#v", creator)
	}
}

func TestBackfillTwitterCreatorIdentityFromExistingName(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO creators(directory_id, name, work_count, online_identity_status) VALUES(?, ?, 1, 'missing')`, dir.ID, "DamiDamie233"); err != nil {
		t.Fatal(err)
	}
	if err := store.backfillTwitterCreatorIdentity(); err != nil {
		t.Fatal(err)
	}
	creators, err := store.Creators()
	if err != nil {
		t.Fatal(err)
	}
	if len(creators) != 1 || creators[0].OnlineIdentityStatus != "ready" || creators[0].TwitterUsername != "DamiDamie233" {
		t.Fatalf("creators = %#v", creators)
	}
}

func TestUpdateAuthSettingsReturnSavedValuesAndQueuesRun(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ensureCreatorVideoDir(t, dir.Path, "twuser")
	creator, _, err := store.UpsertCreatorWithWorksWithIdentity(dir.ID, "twuser", CreatorIdentity{TwitterUsername: "twuser", ProfileURL: "https://x.com/twuser"}, []WorkInput{{
		AwemeID: "1001",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(t.TempDir(), "clip.mp4"), FileName: "clip.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUpdateAuth(UpdateAuthInput{AuthToken: stringPtr("auth-one"), CT0: stringPtr("ct0-one"), FetchLimit: intPtr(50), MediaOnly: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	auth, err := store.UpdateAuth()
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Configured || auth.AuthToken != "auth-one" || auth.CT0 != "ct0-one" || auth.FetchLimit != 50 || !auth.MediaOnly {
		t.Fatalf("auth = %#v", auth)
	}
	manager := NewUpdateManager(store, NewEventHub(), fakeDownloader{addedWorks: 2})
	if _, err := manager.EnqueueImmediate([]int64{creator.ID}, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool {
		status := manager.Status()
		return !status.Running && status.PendingCreators == 0
	})
	status := manager.Status()
	if status.SucceededCreators != 1 || status.AddedWorks != 2 {
		t.Fatalf("status = %#v", status)
	}
}

func TestSaveUpdateAuthPreservesOmittedFieldsAndClearsExplicitEmpty(t *testing.T) {
	store := newTestStore(t)
	if err := store.SaveUpdateAuth(UpdateAuthInput{
		AuthToken: stringPtr("auth-one"),
		CT0:       stringPtr("ct0-one"),
		Proxy:     stringPtr("socks5://127.0.0.1:1080"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUpdateAuth(UpdateAuthInput{Proxy: stringPtr("socks5://127.0.0.1:1080"), FetchLimit: intPtr(88)}); err != nil {
		t.Fatal(err)
	}
	auth, err := store.UpdateAuth()
	if err != nil {
		t.Fatal(err)
	}
	if auth.AuthToken != "auth-one" || auth.CT0 != "ct0-one" || auth.Proxy != "socks5://127.0.0.1:1080" || auth.FetchLimit != 88 {
		t.Fatalf("auth after partial patch = %#v", auth)
	}
	if err := store.SaveUpdateAuth(UpdateAuthInput{AuthToken: stringPtr("")}); err != nil {
		t.Fatal(err)
	}
	auth, err = store.UpdateAuth()
	if err != nil {
		t.Fatal(err)
	}
	if auth.AuthToken != "" || auth.CT0 != "ct0-one" || auth.Configured {
		t.Fatalf("auth after clear = %#v", auth)
	}
}

func TestSaveUpdateAuthStoresFullCookieWithoutReturningIt(t *testing.T) {
	store := newTestStore(t)
	fullCookie := "auth_token=auth-one; ct0=ct0-one; guest_id=guest-one"
	if err := store.SaveUpdateAuth(UpdateAuthInput{CookieHeader: stringPtr(fullCookie)}); err != nil {
		t.Fatal(err)
	}
	auth, err := store.UpdateAuth()
	if err != nil {
		t.Fatal(err)
	}
	if !auth.CookieHeaderSaved || auth.CookieHeader != "" || !auth.Configured {
		t.Fatalf("public auth = %#v", auth)
	}
	secret, err := store.updateAuthSecret()
	if err != nil {
		t.Fatal(err)
	}
	if secret.CookieHeader != fullCookie {
		t.Fatalf("secret cookie header was not preserved")
	}
}

func TestSaveUpdateAuthStoresDownloadDirectorySettings(t *testing.T) {
	store := newTestStore(t)
	customDir := t.TempDir()
	if err := store.SaveUpdateAuth(UpdateAuthInput{DownloadMode: stringPtr("custom"), DownloadDirectory: stringPtr(customDir)}); err != nil {
		t.Fatal(err)
	}
	auth, err := store.UpdateAuth()
	if err != nil {
		t.Fatal(err)
	}
	if auth.DownloadMode != "custom" || auth.DownloadDirectory != customDir {
		t.Fatalf("auth download settings = %#v", auth)
	}
	secret, err := store.updateAuthSecret()
	if err != nil {
		t.Fatal(err)
	}
	if secret.DownloadMode != "custom" || secret.DownloadDirectory != customDir {
		t.Fatalf("secret download settings = %#v", secret)
	}
}

func TestSaveUpdateAuthStoresDownloaderConcurrency(t *testing.T) {
	store := newTestStore(t)
	if err := store.SaveUpdateAuth(UpdateAuthInput{DownloaderConcurrency: intPtr(3)}); err != nil {
		t.Fatal(err)
	}
	auth, err := store.UpdateAuth()
	if err != nil {
		t.Fatal(err)
	}
	if auth.DownloaderConcurrency != 3 {
		t.Fatalf("downloader concurrency = %d, want 3", auth.DownloaderConcurrency)
	}
	if err := store.SaveUpdateAuth(UpdateAuthInput{DownloaderConcurrency: intPtr(0)}); err != nil {
		t.Fatal(err)
	}
	auth, err = store.UpdateAuth()
	if err != nil {
		t.Fatal(err)
	}
	if auth.DownloaderConcurrency != 1 {
		t.Fatalf("zero concurrency should fall back to 1, got %#v", auth)
	}
	if err := store.SaveUpdateAuth(UpdateAuthInput{DownloaderConcurrency: intPtr(99)}); err != nil {
		t.Fatal(err)
	}
	auth, err = store.UpdateAuth()
	if err != nil {
		t.Fatal(err)
	}
	if auth.DownloaderConcurrency != 8 {
		t.Fatalf("large concurrency should clamp to 8, got %#v", auth)
	}
}

func TestParseTwitterCookiesSupportsHeaderAndJSON(t *testing.T) {
	headerCookies, err := parseTwitterCookies("auth_token=auth-one; ct0=ct0-one; guest_id=guest-one")
	if err != nil {
		t.Fatal(err)
	}
	if len(headerCookies) != 3 || headerCookies[0].Name != "auth_token" || headerCookies[1].Name != "ct0" {
		t.Fatalf("header cookies = %#v", headerCookies)
	}
	jsonCookies, err := parseTwitterCookies(`[{"name":"auth_token","value":"auth-one"},{"name":"ct0","value":"ct0-one"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(jsonCookies) != 2 || jsonCookies[0].Name != "auth_token" || jsonCookies[1].Name != "ct0" {
		t.Fatalf("json cookies = %#v", jsonCookies)
	}
}

func TestResolveUpdateDownloadTargetNormalizesOriginalDirectories(t *testing.T) {
	root := t.TempDir()
	username := "dahuoluowan"
	if err := os.MkdirAll(filepath.Join(root, username, "video"), 0o755); err != nil {
		t.Fatal(err)
	}
	creator := Creator{Name: username, TwitterUsername: username}
	for _, inputPath := range []string{
		root,
		filepath.Join(root, username),
		filepath.Join(root, username, "video"),
	} {
		target, err := resolveUpdateDownloadTarget(creator, Directory{Path: inputPath}, UpdateAuthSecret{DownloadMode: "original"})
		if err != nil {
			t.Fatalf("resolve %s: %v", inputPath, err)
		}
		if target.RootDir != root || target.VideoDir != filepath.Join(root, username, "video") {
			t.Fatalf("target for %s = %#v", inputPath, target)
		}
	}
}

func TestResolveUpdateDownloadTargetPrefersNestedWhenBothLayoutsExist(t *testing.T) {
	root := t.TempDir()
	username := "dahuoluowan"
	nested := filepath.Join(root, username, username, "video")
	flat := filepath.Join(root, username, "video")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(flat, 0o755); err != nil {
		t.Fatal(err)
	}
	creator := Creator{Name: username, TwitterUsername: username}
	target, err := resolveUpdateDownloadTarget(creator, Directory{Path: root}, UpdateAuthSecret{DownloadMode: "original"})
	if err != nil {
		t.Fatal(err)
	}
	if target.VideoDir != nested {
		t.Fatalf("video dir = %q, want nested %q", target.VideoDir, nested)
	}
}

func TestResolveUpdateDownloadTargetDefaultsCustomDirectoryToNestedForNewCreator(t *testing.T) {
	root := t.TempDir()
	username := "newcreator"
	creator := Creator{Name: username, TwitterUsername: username}
	target, err := resolveUpdateDownloadTarget(creator, Directory{Path: root}, UpdateAuthSecret{DownloadMode: "custom", DownloadDirectory: root})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, username, username, "video")
	if target.VideoDir != want {
		t.Fatalf("video dir = %q, want %q", target.VideoDir, want)
	}
}

func TestUpdateManagerFallsBackToCustomDownloadDirectoryWhenOriginalMissing(t *testing.T) {
	store := newTestStore(t)
	originalRoot := t.TempDir()
	customRoot := t.TempDir()
	dir, err := store.SaveDirectory(originalRoot)
	if err != nil {
		t.Fatal(err)
	}
	creator, _, err := store.UpsertCreatorWithWorksWithIdentity(dir.ID, "newcreator", CreatorIdentity{TwitterUsername: "newcreator", ProfileURL: "https://x.com/newcreator"}, []WorkInput{{
		AwemeID: "1001",
		Title:   "seed",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(t.TempDir(), "seed.mp4"), FileName: "seed.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUpdateAuth(UpdateAuthInput{
		AuthToken:         stringPtr("auth-one"),
		CT0:               stringPtr("ct0-one"),
		DownloadMode:      stringPtr("original"),
		DownloadDirectory: stringPtr(customRoot),
	}); err != nil {
		t.Fatal(err)
	}
	outputDirs := make(chan string, 1)
	manager := NewUpdateManager(store, NewEventHub(), fakeDownloader{addedWorks: 1, outputDirs: outputDirs})
	if _, err := manager.EnqueueImmediate([]int64{creator.ID}, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool {
		status := manager.Status()
		return !status.Running && status.PendingCreators == 0
	})
	select {
	case outputDir := <-outputDirs:
		if outputDir != customRoot {
			t.Fatalf("output dir = %q, want %q", outputDir, customRoot)
		}
	default:
		t.Fatal("downloader was not called")
	}
	jobs, err := store.UpdateJobs(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) == 0 || jobs[0].DownloadDirectory != filepath.Join(customRoot, "newcreator", "newcreator", "video") || jobs[0].DownloadNotice == "" {
		t.Fatalf("job download info = %#v", jobs)
	}
}

func TestUpdateJobsPagePaginatesNewestFirst(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	creator, _, err := store.UpsertCreatorWithWorks(dir.ID, "twuser", []WorkInput{{
		AwemeID: "1001",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(t.TempDir(), "clip.mp4"), FileName: "clip.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		jobs, err := store.AddUpdateJobs([]int64{creator.ID}, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 1 {
			t.Fatalf("jobs = %#v", jobs)
		}
		if err := store.FinishUpdateJob(jobs[0].ID, "succeeded", i, ""); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.UpdateJobsPage(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Page != 1 || first.PageSize != 10 || first.Total != 12 || first.TotalPages != 2 || len(first.Jobs) != 10 {
		t.Fatalf("first page = %#v", first)
	}
	if first.Jobs[0].ID <= first.Jobs[9].ID {
		t.Fatalf("jobs are not newest first: %#v", first.Jobs)
	}
	second, err := store.UpdateJobsPage(2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if second.Page != 2 || len(second.Jobs) != 2 {
		t.Fatalf("second page = %#v", second)
	}
	if second.Jobs[0].ID <= second.Jobs[1].ID {
		t.Fatalf("second page jobs are not newest first: %#v", second.Jobs)
	}
}

func TestUpdateJobsUseCleanCreatorDisplayName(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	withUsername, err := insertLegacyUpdateCreator(t, store, dir.ID, "待更新-longname", "real_long_username")
	if err != nil {
		t.Fatal(err)
	}
	withoutUsername, err := insertLegacyUpdateCreator(t, store, dir.ID, "待更新-mat2kx", "")
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := store.AddUpdateJobs([]int64{withUsername, withoutUsername}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %#v", jobs)
	}
	page, err := store.UpdateJobsPage(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	names := map[int64]string{}
	for _, job := range page.Jobs {
		names[job.CreatorID] = job.CreatorName
	}
	if names[withUsername] != "real_long_username" {
		t.Fatalf("creator with username display name = %q", names[withUsername])
	}
	if names[withoutUsername] != "mat2kx" {
		t.Fatalf("creator without username display name = %q", names[withoutUsername])
	}
}

func TestUpdateJobsTrackSourceAttemptRunningAndDelete(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	creator, _, err := store.UpsertCreatorWithWorks(dir.ID, "scheduled_user", []WorkInput{{
		AwemeID: "1001",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(t.TempDir(), "clip.mp4"), FileName: "clip.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := store.AddUpdateJobs([]int64{creator.ID}, 0, 0, "scheduled")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Source != "scheduled" || jobs[0].Attempt != 1 {
		t.Fatalf("scheduled job = %#v", jobs)
	}
	if err := store.MarkUpdateJobRunning(jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	running, err := store.RunningUpdateJobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 1 || running[0].ID != jobs[0].ID {
		t.Fatalf("running jobs = %#v", running)
	}
	if err := store.DeleteUpdateJob(jobs[0].ID); err == nil || !strings.Contains(err.Error(), "running update job") {
		t.Fatalf("delete running error = %v", err)
	}
	if err := store.FinishUpdateJob(jobs[0].ID, "failed", 0, "bad auth"); err != nil {
		t.Fatal(err)
	}
	retried, retriedOK, err := store.RetryUpdateJob(jobs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !retriedOK || retried.ID != jobs[0].ID || retried.Attempt != 2 || retried.Source != "manual" || retried.Status != "pending" {
		t.Fatalf("retried job = %#v ok=%v", retried, retriedOK)
	}
	if err := store.FinishUpdateJob(jobs[0].ID, "succeeded", 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUpdateJob(jobs[0].ID); err != nil {
		t.Fatalf("delete finished job: %v", err)
	}
	page, err := store.UpdateJobsPage(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 {
		t.Fatalf("jobs after delete = %#v", page)
	}
}

func insertLegacyUpdateCreator(t *testing.T, store *Store, directoryID int64, name string, username string) (int64, error) {
	t.Helper()
	result, err := store.db.Exec(`INSERT INTO creators(directory_id, name, twitter_username, twitter_profile_url, online_identity_status, work_count)
VALUES(?, ?, ?, ?, 'ready', 0)`, directoryID, name, username, "")
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func TestUpdateManagerCountsOnlyNewWorksAfterScan(t *testing.T) {
	store := newTestStore(t)
	root := t.TempDir()
	dir, err := store.SaveDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	videoPath := filepath.Join(root, "twuser", "video", "2026-04-08_mockvideo_更新作品.mp4")
	creator, _, err := store.UpsertCreatorWithWorksWithIdentity(dir.ID, "twuser", CreatorIdentity{TwitterUsername: "twuser", ProfileURL: "https://x.com/twuser"}, []WorkInput{{
		AwemeID: "mockvideo",
		Title:   "更新作品",
		Media:   []MediaInput{{Type: "video", FilePath: videoPath, FileName: filepath.Base(videoPath)}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "twuser", "twuser", "video"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUpdateAuth(UpdateAuthInput{AuthToken: stringPtr("auth-one"), CT0: stringPtr("ct0-one"), MediaOnly: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	manager := NewUpdateManager(store, NewEventHub(), fakeDownloader{addedWorks: 1})
	if _, err := manager.EnqueueImmediate([]int64{creator.ID}, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool {
		status := manager.Status()
		return !status.Running && status.PendingCreators == 0
	})
	status := manager.Status()
	if status.SucceededCreators != 1 || status.AddedWorks != 0 {
		t.Fatalf("status = %#v", status)
	}
	after, err := store.CreatorWorkCount(creator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after != 1 {
		t.Fatalf("work count = %d, want 1", after)
	}
	jobs, err := store.UpdateJobs(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].AddedWorks != 0 {
		t.Fatalf("jobs = %#v", jobs)
	}
}

func TestUpdateManagerUsesCreatorNameAsTwitterUsername(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ensureCreatorVideoDir(t, dir.Path, "Khazix0918")
	creator, _, err := store.UpsertCreatorWithWorks(dir.ID, "Khazix0918", []WorkInput{{
		AwemeID: "1001",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(t.TempDir(), "clip.mp4"), FileName: "clip.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUpdateAuth(UpdateAuthInput{AuthToken: stringPtr("auth-one"), CT0: stringPtr("ct0-one"), MediaOnly: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	usernames := make(chan string, 1)
	manager := NewUpdateManager(store, NewEventHub(), fakeDownloader{addedWorks: 1, usernames: usernames})
	if _, err := manager.EnqueueImmediate([]int64{creator.ID}, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool {
		status := manager.Status()
		return !status.Running && status.PendingCreators == 0
	})
	select {
	case username := <-usernames:
		if username != "Khazix0918" {
			t.Fatalf("username = %q", username)
		}
	default:
		t.Fatal("downloader was not called")
	}
}

func TestUpdateManagerPersistsTweetProgress(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ensureCreatorVideoDir(t, dir.Path, "twuser")
	creator, _, err := store.UpsertCreatorWithWorks(dir.ID, "twuser", []WorkInput{{
		AwemeID: "1001",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(t.TempDir(), "clip.mp4"), FileName: "clip.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	progressDone := make(chan struct{}, 1)
	release := make(chan struct{})
	manager := NewUpdateManager(store, NewEventHub(), fakeDownloader{
		addedWorks: 1,
		progress: []DownloadProgress{{
			TotalTweets:      10,
			ProcessedTweets:  4,
			DownloadedVideos: 2,
			SkippedTweets:    1,
			DownloadSpeedBps: 12 * 1024 * 1024,
			LastMessage:      "正在处理推文 4/10",
		}},
		progressDone: progressDone,
		release:      release,
	})
	if _, err := manager.EnqueueImmediate([]int64{creator.ID}, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case <-progressDone:
	case <-time.After(2 * time.Second):
		t.Fatal("progress was not reported")
	}
	status := manager.Status()
	if len(status.Jobs) == 0 {
		t.Fatalf("status has no jobs: %#v", status)
	}
	job := status.Jobs[0]
	if job.TotalTweets != 10 || job.ProcessedTweets != 4 || job.DownloadedVideos != 2 || job.SkippedTweets != 1 || job.DownloadSpeedBps != 12*1024*1024 || job.ProgressMessage != "正在处理推文 4/10" {
		t.Fatalf("job progress = %#v", job)
	}
	close(release)
	waitFor(t, 2*time.Second, func() bool {
		status := manager.Status()
		return !status.Running && status.PendingCreators == 0
	})
}

func TestUpdateManagerRunsJobsUpToDownloaderConcurrency(t *testing.T) {
	store := newTestStore(t)
	root := t.TempDir()
	dir, err := store.SaveDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	creatorIDs := make([]int64, 0, 3)
	for _, name := range []string{"one", "two", "three"} {
		ensureCreatorVideoDir(t, root, name)
		creator, _, err := store.UpsertCreatorWithWorksWithIdentity(dir.ID, name, CreatorIdentity{TwitterUsername: name, ProfileURL: "https://x.com/" + name}, []WorkInput{{
			AwemeID: "seed-" + name,
			Title:   "seed",
			Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(root, name, "video", "seed.mp4"), FileName: "seed.mp4"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		creatorIDs = append(creatorIDs, creator.ID)
	}
	if err := store.SaveUpdateAuth(UpdateAuthInput{AuthToken: stringPtr("auth-one"), CT0: stringPtr("ct0-one"), DownloaderConcurrency: intPtr(2)}); err != nil {
		t.Fatal(err)
	}
	started := make(chan string, 3)
	release := make(chan struct{})
	manager := NewUpdateManager(store, NewEventHub(), fakeDownloader{addedWorks: 1, started: started, release: release})
	if _, err := manager.EnqueueImmediate(creatorIDs, 0); err != nil {
		t.Fatal(err)
	}
	waitForStarted(t, started, 2)
	jobs, err := store.UpdateJobs(10)
	if err != nil {
		t.Fatal(err)
	}
	running := 0
	pending := 0
	for _, job := range jobs {
		if job.Status == "running" {
			running++
		}
		if job.Status == "pending" {
			pending++
		}
	}
	if running != 2 || pending != 1 {
		t.Fatalf("running=%d pending=%d jobs=%#v", running, pending, jobs)
	}
	close(release)
	waitFor(t, 2*time.Second, func() bool {
		status := manager.Status()
		return !status.Running && status.PendingCreators == 0
	})
}

func TestUpdateManagerRetriesOnlyFailedJobs(t *testing.T) {
	store := newTestStore(t)
	root := t.TempDir()
	dir, err := store.SaveDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	ensureCreatorVideoDir(t, root, "twuser")
	creator, _, err := store.UpsertCreatorWithWorksWithIdentity(dir.ID, "twuser", CreatorIdentity{TwitterUsername: "twuser", ProfileURL: "https://x.com/twuser"}, []WorkInput{{
		AwemeID: "1001",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(root, "twuser", "video", "clip.mp4"), FileName: "clip.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUpdateAuth(UpdateAuthInput{AuthToken: stringPtr("auth-one"), CT0: stringPtr("ct0-one")}); err != nil {
		t.Fatal(err)
	}
	jobs, err := store.AddUpdateJobs([]int64{creator.ID}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishUpdateJob(jobs[0].ID, "failed", 0, "bad auth"); err != nil {
		t.Fatal(err)
	}
	started := make(chan string, 1)
	release := make(chan struct{})
	manager := NewUpdateManager(store, NewEventHub(), fakeDownloader{addedWorks: 1, started: started, release: release})
	_, retriedJob, retried, err := manager.RetryJob(jobs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !retried || retriedJob.ID != jobs[0].ID || retriedJob.Attempt != 2 || retriedJob.Source != "manual" {
		t.Fatalf("retry result retried=%v job=%#v original=%#v", retried, retriedJob, jobs[0])
	}
	waitForStarted(t, started, 1)
	_, sameJob, retriedAgain, err := manager.RetryJob(retriedJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retriedAgain || sameJob.ID != retriedJob.ID {
		t.Fatalf("running retry should be no-op: retried=%v job=%#v", retriedAgain, sameJob)
	}
	close(release)
	waitFor(t, 2*time.Second, func() bool {
		status := manager.Status()
		return !status.Running && status.PendingCreators == 0
	})
	_, succeededJob, retriedSucceeded, err := manager.RetryJob(retriedJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retriedSucceeded || succeededJob.ID != retriedJob.ID {
		t.Fatalf("succeeded retry should be no-op: retried=%v job=%#v", retriedSucceeded, succeededJob)
	}
}

func TestUpdateManagerRetryUsesCurrentUserAuthOwner(t *testing.T) {
	store := newTestStore(t)
	root := t.TempDir()
	dir, err := store.SaveDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	ensureCreatorVideoDir(t, root, "twuser")
	creator, _, err := store.UpsertCreatorWithWorksWithIdentity(dir.ID, "twuser", CreatorIdentity{TwitterUsername: "twuser", ProfileURL: "https://x.com/twuser"}, []WorkInput{{
		AwemeID: "1001",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(root, "twuser", "video", "clip.mp4"), FileName: "clip.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	superID, err := store.superAdminUserID()
	if err != nil {
		t.Fatal(err)
	}
	alice, err := store.CreateUser("alice", "alice123")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUpdateAuthForUser(alice.ID, UpdateAuthInput{CookieHeader: stringPtr("auth_token=alice; ct0=alice")}); err != nil {
		t.Fatal(err)
	}
	jobs, err := store.AddUpdateJobsForUser([]int64{creator.ID}, 0, 0, superID)
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].CookieOwnerUserID != superID {
		t.Fatalf("seed job owner = %#v, want %d", jobs[0], superID)
	}
	if err := store.FinishUpdateJob(jobs[0].ID, "failed", 0, "bad auth"); err != nil {
		t.Fatal(err)
	}
	started := make(chan string, 1)
	release := make(chan struct{})
	manager := NewUpdateManager(store, NewEventHub(), fakeDownloader{addedWorks: 1, started: started, release: release})
	_, retriedJob, retried, err := manager.RetryJobForUser(jobs[0].ID, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !retried {
		t.Fatal("expected failed job to be retried")
	}
	if retriedJob.ActorUserID != alice.ID || retriedJob.CookieOwnerUserID != alice.ID {
		t.Fatalf("retried job should use current user auth owner, got %#v want user %d", retriedJob, alice.ID)
	}
	waitForStarted(t, started, 1)
	close(release)
	waitFor(t, 2*time.Second, func() bool {
		status := manager.Status()
		return !status.Running && status.PendingCreators == 0
	})
}

func TestUpdateManagerClassifiesNoDownloadableVideo(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ensureCreatorVideoDir(t, dir.Path, "twuser")
	creator, _, err := store.UpsertCreatorWithWorks(dir.ID, "twuser", []WorkInput{{
		AwemeID: "1001",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(t.TempDir(), "clip.mp4"), FileName: "clip.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewUpdateManager(store, NewEventHub(), fakeDownloader{progress: []DownloadProgress{{TotalTweets: 3, ProcessedTweets: 3, SkippedTweets: 3, LastMessage: "全部跳过"}}})
	if _, err := manager.EnqueueImmediate([]int64{creator.ID}, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool {
		status := manager.Status()
		return !status.Running && status.PendingCreators == 0
	})
	status := manager.Status()
	if len(status.Jobs) == 0 {
		t.Fatalf("status has no jobs: %#v", status)
	}
	job := status.Jobs[0]
	if job.Error != "未发现可下载视频或全部已跳过" {
		t.Fatalf("friendly error = %q, detail=%q", job.Error, job.ErrorDetail)
	}
	if job.ErrorDetail == "" || job.ErrorDetail == job.Error {
		t.Fatalf("error detail should preserve diagnostic context: %#v", job)
	}
}

func TestClassifyUpdateError(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want string
	}{
		{name: "auth", err: errors.New(`response status 401 Unauthorized: {"errors":[{"message":"Could not authenticate you","code":32}]}`), want: "Twitter 登录配置不可用，请更新完整 Cookie 或重新登录 X"},
		{name: "proxy", err: errors.New("proxyconnect tcp: dial tcp 127.0.0.1:1080: i/o timeout"), want: "网络或代理不可用，请检查代理地址"},
		{name: "proxy auth preflight", err: errors.New("Twitter 登录配置不可用或代理不可用，请检查完整 Cookie 和代理地址"), want: "网络或代理不可用，请检查代理地址"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, detail := classifyUpdateError(tt.err)
			if got != tt.want || detail == "" {
				t.Fatalf("classifyUpdateError() = %q, %q", got, detail)
			}
		})
	}
}

func TestTWMDNamingAndScannerContract(t *testing.T) {
	name := TWMDMediaFileName(time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC), "video.twimg.com/ext_tw_video/abc123/pu/vid/720x1280/Foo-Bar.mp4?tag=12", `放个增管素材 / 顺便:清理*库?存`, ".mp4")
	want := "2026-04-08_Foo-Bar_放个增管素材  顺便清理库存.mp4"
	if name != want {
		t.Fatalf("TWMDMediaFileName() = %q, want %q", name, want)
	}
	dir := t.TempDir()
	for _, item := range []struct {
		name string
		data string
	}{
		{name, "video"},
		{"2026-04-08_Foo-Bar_放个增管素材  顺便清理库存.jpg", "cover"},
		{"2026-04-08_Foo-Bar_放个增管素材  顺便清理库存.ass", "ass"},
		{"2026-04-08_Foo-Bar_放个增管素材  顺便清理库存.nfo", "nfo"},
		{"2026-04-08_Foo-Bar_放个增管素材  顺便清理库存.json", `{"ID":"2048","Text":"放个增管素材","Username":"twuser","Name":"TW User","Timestamp":1775642400,"PermanentURL":"https://x.com/twuser/status/2048"}`},
	} {
		if err := writeTestFile(filepath.Join(dir, item.name), []byte(item.data)); err != nil {
			t.Fatal(err)
		}
	}
	works, err := scanCreator(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 1 || works[0].AwemeID != "2048" || len(works[0].Media) != 1 || works[0].Media[0].Type != "video" {
		t.Fatalf("works = %#v", works)
	}
}

type fakeDownloader struct {
	addedWorks   int
	err          error
	usernames    chan<- string
	outputDirs   chan<- string
	progress     []DownloadProgress
	progressDone chan<- struct{}
	started      chan<- string
	release      <-chan struct{}
}

func (f fakeDownloader) Ready(context.Context) error { return f.err }

func (f fakeDownloader) UpdateCreator(ctx context.Context, task DownloadTask) (DownloadResult, error) {
	if f.err != nil {
		return DownloadResult{}, f.err
	}
	if f.usernames != nil {
		f.usernames <- task.Username
	}
	if f.outputDirs != nil {
		f.outputDirs <- task.OutputDir
	}
	if f.started != nil {
		f.started <- task.Username
	}
	for _, progress := range f.progress {
		if task.ReportProgress != nil {
			task.ReportProgress(progress)
		}
		if f.progressDone != nil {
			f.progressDone <- struct{}{}
		}
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return DownloadResult{}, ctx.Err()
		}
	}
	if f.addedWorks > 0 {
		videoDir := task.VideoDir
		if videoDir == "" {
			videoDir = filepath.Join(task.OutputDir, task.Username, "video")
		}
		if err := os.MkdirAll(videoDir, 0o755); err == nil {
			for i := 0; i < f.addedWorks; i++ {
				id := "mockvideo"
				if f.addedWorks > 1 {
					id = id + strconv.Itoa(i+1)
				}
				prefix := "2026-04-08_" + id + "_更新作品"
				_ = os.WriteFile(filepath.Join(videoDir, prefix+".mp4"), []byte("video"), 0o644)
				_ = os.WriteFile(filepath.Join(videoDir, prefix+".json"), []byte(`{"ID":"`+id+`","Text":"更新作品","Username":"`+task.Username+`","Timestamp":1775642400,"PermanentURL":"https://x.com/`+task.Username+`/status/`+id+`"}`), 0o644)
			}
		}
	}
	return DownloadResult{AddedWorks: f.addedWorks}, nil
}

func (f fakeDownloader) ValidateAuth(context.Context, UpdateAuthSecret) error { return f.err }

func ensureCreatorVideoDir(t *testing.T, root string, username string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, username, "video"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func waitForStarted(t *testing.T, started <-chan string, count int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for i := 0; i < count; i++ {
		select {
		case <-started:
		case <-deadline:
			t.Fatalf("only observed %d started downloads, want %d", i, count)
		}
	}
}

func writeTestFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

func stringPtr(value string) *string { return &value }

func intPtr(value int) *int { return &value }

func boolPtr(value bool) *bool { return &value }
