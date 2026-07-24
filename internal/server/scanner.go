package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var twitterFilePattern = regexp.MustCompile(`(?s)^\d{4}-\d{2}-\d{2}_(.+)$`)
var ErrPathNotAllowed = errors.New("path is not in authorized directories")

type FileBrowser struct {
	allowedPaths []string
}

type FSEntry struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func ValidateDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("path is not a directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	_ = entries
	return nil
}

func NewFileBrowserFromEnv() FileBrowser {
	return FileBrowser{allowedPaths: ParseAccessiblePaths(os.Getenv("TRIM_DATA_ACCESSIBLE_PATHS"))}
}

func ParseAccessiblePaths(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, string(os.PathListSeparator))
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := filepath.Clean(strings.TrimSpace(part))
		if clean == "" || clean == "." {
			continue
		}
		paths = append(paths, clean)
	}
	return paths
}

func PathAllowed(path string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	clean := filepath.Clean(path)
	for _, root := range allowed {
		root = filepath.Clean(root)
		if clean == root {
			return true
		}
		rel, err := filepath.Rel(root, clean)
		if err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func ValidateAccessibleDirectory(path string, allowed []string) error {
	if err := ValidateDirectory(path); err != nil {
		return err
	}
	if !PathAllowed(path, allowed) {
		return ErrPathNotAllowed
	}
	return nil
}

func (browser FileBrowser) Roots() []FSEntry {
	if len(browser.allowedPaths) > 0 {
		roots := make([]FSEntry, 0, len(browser.allowedPaths))
		for _, path := range browser.allowedPaths {
			if err := ValidateDirectory(path); err != nil {
				continue
			}
			roots = append(roots, FSEntry{Path: path, Name: filepath.Base(path)})
		}
		return roots
	}
	roots := []FSEntry{}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, FSEntry{Path: home, Name: "Home"})
	}
	wd, err := os.Getwd()
	if err == nil {
		roots = append(roots, FSEntry{Path: wd, Name: "Project"})
	}
	roots = append(roots, FSEntry{Path: string(filepath.Separator), Name: "Root"})
	return roots
}

func (browser FileBrowser) List(path string) ([]FSEntry, error) {
	if err := ValidateAccessibleDirectory(path, browser.allowedPaths); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var dirs []FSEntry
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(path, name)
		if err := ValidateAccessibleDirectory(full, browser.allowedPaths); err != nil {
			continue
		}
		dirs = append(dirs, FSEntry{Path: full, Name: name})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	return dirs, nil
}

func (browser FileBrowser) Validate(path string) error {
	return ValidateAccessibleDirectory(path, browser.allowedPaths)
}

type Scanner struct {
	store          *Store
	hub            *EventHub
	profileFetcher CreatorProfileFetcher

	mu      sync.Mutex
	queue   []scanTask
	running bool
	status  ScanStatus
}

type scanTask struct {
	directoryID int64
	creatorID   int64
}

func NewScanner(store *Store, hub *EventHub) *Scanner {
	return &Scanner{store: store, hub: hub, profileFetcher: TwitterProfileFetcher{}, status: ScanStatus{State: "idle"}}
}

func (s *Scanner) Enqueue(directoryIDs ...int64) ScanStatus {
	s.mu.Lock()
	for _, id := range directoryIDs {
		if id > 0 {
			s.queue = append(s.queue, scanTask{directoryID: id})
		}
	}
	if !s.running {
		s.running = true
		go s.run()
	}
	status := s.snapshotLocked()
	s.mu.Unlock()
	s.hub.Publish(ScanEvent{Type: "queued", Status: status})
	return status
}

func (s *Scanner) EnqueueCreator(creatorID int64) ScanStatus {
	s.mu.Lock()
	if creatorID > 0 {
		s.queue = append(s.queue, scanTask{creatorID: creatorID})
	}
	if !s.running {
		s.running = true
		go s.run()
	}
	status := s.snapshotLocked()
	s.mu.Unlock()
	s.hub.Publish(ScanEvent{Type: "queued", Status: status})
	return status
}

func (s *Scanner) Status() ScanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *Scanner) run() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.running = false
			s.status.Running = false
			s.status.State = "idle"
			status := s.snapshotLocked()
			s.mu.Unlock()
			s.hub.Publish(ScanEvent{Type: "idle", Status: status})
			return
		}
		task := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()
		if task.creatorID > 0 {
			s.scanSingleCreator(task.creatorID)
			continue
		}
		s.scanDirectory(task.directoryID)
	}
}

func (s *Scanner) scanDirectory(directoryID int64) {
	dir, err := s.store.Directory(directoryID)
	if err != nil {
		s.recordError("load directory failed: " + err.Error())
		return
	}
	_ = s.store.MarkDirectoryStatus(directoryID, "scanning")
	creatorDirs, err := readCreatorDirs(dir.Path)
	if err != nil {
		s.recordError("read directory failed: " + err.Error())
		_ = s.store.MarkDirectoryStatus(directoryID, "error")
		return
	}
	s.mu.Lock()
	s.status = ScanStatus{
		Running:            true,
		QueueLength:        len(s.queue),
		CurrentTaskID:      time.Now().UTC().Format("20060102150405.000000000"),
		CurrentDirectoryID: dir.ID,
		CurrentDirectory:   dir.Path,
		TotalCreators:      len(creatorDirs),
		State:              "scanning",
	}
	status := s.snapshotLocked()
	s.mu.Unlock()
	s.hub.Publish(ScanEvent{Type: "started", Status: status})

	for _, creatorDir := range creatorDirs {
		s.mu.Lock()
		s.status.CurrentCreator = creatorDir.Name
		status = s.snapshotLocked()
		s.mu.Unlock()
		s.hub.Publish(ScanEvent{Type: "creator_started", Status: status})

		works, warnings, err := scanCreatorWithWarnings(creatorDir.Path)
		if err != nil {
			s.recordError(creatorDir.Name + ": " + err.Error())
			continue
		}
		for _, warning := range warnings {
			s.recordError(creatorDir.Name + ": " + warning)
		}
		if len(works) == 0 {
			s.mu.Lock()
			s.status.ScannedCreators++
			if s.status.TotalCreators > 0 {
				s.status.Progress = int(float64(s.status.ScannedCreators) / float64(s.status.TotalCreators) * 100)
			}
			status = s.snapshotLocked()
			s.mu.Unlock()
			s.hub.Publish(ScanEvent{Type: "creator_skipped", Status: status})
			continue
		}
		creator, count, err := s.store.UpsertCreatorWithWorks(dir.ID, creatorDir.Name, works)
		if err != nil {
			s.recordError(creatorDir.Name + ": " + err.Error())
			continue
		}
		s.refreshProfileAsync(creator)
		s.mu.Lock()
		s.status.ScannedCreators++
		s.status.ScannedWorks += count
		if s.status.TotalCreators > 0 {
			s.status.Progress = int(float64(s.status.ScannedCreators) / float64(s.status.TotalCreators) * 100)
		}
		status = s.snapshotLocked()
		s.mu.Unlock()
		s.hub.Publish(ScanEvent{Type: "creator_completed", Status: status, Creator: &creator})
	}
	_ = s.store.MarkDirectoryScanned(dir.ID)
	s.mu.Lock()
	s.status.State = "completed"
	s.status.Progress = 100
	status = s.snapshotLocked()
	s.mu.Unlock()
	s.hub.Publish(ScanEvent{Type: "completed", Status: status})
}

func (s *Scanner) scanSingleCreator(creatorID int64) {
	creator, err := s.store.Creator(creatorID)
	if err != nil {
		s.recordError("load creator failed: " + err.Error())
		return
	}
	dir, err := s.store.Directory(creator.DirectoryID)
	if err != nil {
		s.recordError("load directory failed: " + err.Error())
		return
	}
	creatorRoot := filepath.Join(dir.Path, creator.Name)
	mediaDirs := resolveCreatorMediaDirs(creatorRoot, creator.Name)
	s.mu.Lock()
	s.status = ScanStatus{
		Running:            true,
		QueueLength:        len(s.queue),
		CurrentTaskID:      time.Now().UTC().Format("20060102150405.000000000"),
		CurrentDirectoryID: dir.ID,
		CurrentDirectory:   dir.Path,
		CurrentCreator:     creator.Name,
		TotalCreators:      1,
		State:              "scanning",
	}
	status := s.snapshotLocked()
	s.mu.Unlock()
	s.hub.Publish(ScanEvent{Type: "started", Status: status})
	s.hub.Publish(ScanEvent{Type: "creator_started", Status: status})

	identity := CreatorIdentity{TwitterUserID: creator.TwitterUserID, TwitterUsername: creator.TwitterUsername, ProfileURL: creator.TwitterProfileURL}
	totalScannedWorks := 0
	updatedCreator := creator
	for _, mediaDir := range mediaDirs {
		works, warnings, err := scanCreatorWithWarnings(mediaDir)
		if err != nil {
			s.recordError(creator.Name + ": " + err.Error())
			return
		}
		for _, warning := range warnings {
			s.recordError(creator.Name + ": " + warning)
		}
		if len(works) == 0 {
			continue
		}
		scannedWorks := 0
		updatedCreator, scannedWorks, err = s.store.UpsertCreatorWithWorksWithIdentity(dir.ID, creator.Name, identity, works)
		if err != nil {
			s.recordError(creator.Name + ": " + err.Error())
			return
		}
		s.refreshProfileAsync(updatedCreator)
		totalScannedWorks += scannedWorks
		s.mu.Lock()
		s.status.ScannedWorks += scannedWorks
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.status.ScannedCreators = 1
	s.status.Progress = 100
	s.status.State = "completed"
	status = s.snapshotLocked()
	s.mu.Unlock()
	if totalScannedWorks == 0 {
		s.hub.Publish(ScanEvent{Type: "creator_skipped", Status: status})
	} else {
		s.hub.Publish(ScanEvent{Type: "creator_completed", Status: status, Creator: &updatedCreator})
	}
	s.hub.Publish(ScanEvent{Type: "completed", Status: status})
}

func (s *Scanner) refreshProfile(ctx context.Context, creator Creator) error {
	auth, err := s.store.updateAuthSecret()
	if err != nil {
		auth = UpdateAuthSecret{}
	}
	return RefreshCreatorProfile(ctx, s.store, creator, auth, s.profileFetcher)
}

func (s *Scanner) refreshProfileAsync(creator Creator) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = s.refreshProfile(ctx, creator)
	}()
}

func (s *Scanner) recordError(message string) {
	s.mu.Lock()
	s.status.Errors = append(s.status.Errors, message)
	s.status.State = "error"
	status := s.snapshotLocked()
	s.mu.Unlock()
	s.hub.Publish(ScanEvent{Type: "error", Status: status})
}

func (s *Scanner) snapshotLocked() ScanStatus {
	status := s.status
	status.Running = s.running
	status.QueueLength = len(s.queue)
	status.Errors = append([]string(nil), s.status.Errors...)
	return status
}

type creatorScanDir struct {
	Name string
	Path string
}

func readCreatorDirs(path string) ([]creatorScanDir, error) {
	if err := ValidateDirectory(path); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var creators []creatorScanDir
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			creatorPath := filepath.Join(path, entry.Name())
			for _, mediaPath := range resolveCreatorMediaDirs(creatorPath, entry.Name()) {
				creators = append(creators, creatorScanDir{Name: entry.Name(), Path: mediaPath})
			}
		}
	}
	sort.Slice(creators, func(i, j int) bool {
		left := strings.ToLower(creators[i].Name)
		right := strings.ToLower(creators[j].Name)
		if left == right {
			return strings.ToLower(creators[i].Path) < strings.ToLower(creators[j].Path)
		}
		return left < right
	})
	return creators, nil
}

func resolveCreatorMediaDir(path string, creatorName string) string {
	return resolveCreatorMediaDirs(path, creatorName)[0]
}

func resolveCreatorMediaDirs(path string, creatorName string) []string {
	var paths []string
	nestedVideo := filepath.Join(path, creatorName, "video")
	if info, err := os.Stat(nestedVideo); err == nil && info.IsDir() {
		paths = append(paths, nestedVideo)
	}
	video := filepath.Join(path, "video")
	if info, err := os.Stat(video); err == nil && info.IsDir() {
		paths = append(paths, video)
	}
	if len(paths) > 0 {
		return paths
	}
	return []string{path}
}

func scanCreator(path string) ([]WorkInput, error) {
	works, _, err := scanCreatorWithWarnings(path)
	return works, err
}

func scanCreatorWithWarnings(path string) ([]WorkInput, []string, error) {
	return scanCreatorWithOptions(path, true)
}

func scanCreatorWithoutVideoRepair(path string) ([]WorkInput, error) {
	works, _, err := scanCreatorWithOptions(path, false)
	return works, err
}

func scanCreatorWithOptions(path string, repairVideos bool) ([]WorkInput, []string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, nil, err
	}
	grouped := map[string][]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || strings.EqualFold(name, "Thumbs.db") {
			continue
		}
		tweetID := ParseAwemeID(name)
		if tweetID == "" {
			continue
		}
		grouped[tweetID] = append(grouped[tweetID], filepath.Join(path, name))
	}
	var works []WorkInput
	var warnings []string
	for tweetID, files := range grouped {
		sort.Strings(files)
		work := WorkInput{AwemeID: tweetID, Title: ParseTitle(filepath.Base(files[0])), Description: "", Tags: []string{}}
		var media []MediaInput
		for _, file := range files {
			name := filepath.Base(file)
			if strings.EqualFold(filepath.Ext(name), ".json") {
				meta := parseMetadata(file)
				if meta.AwemeID != "" {
					work.AwemeID = meta.AwemeID
				}
				if meta.Title != "" {
					work.Title = meta.Title
				}
				work.Description = meta.Description
				work.MusicTitle = meta.MusicTitle
				work.SourceURL = meta.SourceURL
				work.Tags = meta.Tags
				work.PublishedAt = meta.PublishedAt
				continue
			}
			mediaType := MediaType(name)
			if mediaType == "" {
				if CoverType(name) != "" && work.CoverFile == "" {
					work.CoverFile = file
				}
				continue
			}
			if mediaType == "video" && repairVideos {
				if _, err := ensureVideoSeekable(file); err != nil {
					warnings = append(warnings, "视频兼容处理失败 "+name+": "+err.Error())
				}
			}
			media = append(media, MediaInput{Type: mediaType, FilePath: file, FileName: name, Ordinal: len(media)})
			if work.Title == "" || work.Title == tweetID {
				work.Title = ParseTitle(name)
			}
		}
		if len(media) == 0 {
			continue
		}
		work.Media = media
		works = append(works, work)
	}
	sort.Slice(works, func(i, j int) bool {
		return works[i].PublishedAt > works[j].PublishedAt
	})
	return works, warnings, nil
}

func ParseAwemeID(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	matches := twitterFilePattern.FindStringSubmatch(base)
	if len(matches) < 2 {
		return ""
	}
	rest := matches[1]
	if rest == "" {
		return ""
	}
	if strings.HasPrefix(rest, "_") {
		if idx := strings.Index(rest[1:], "_"); idx >= 0 {
			return rest[:idx+1]
		}
		return rest
	}
	if idx := strings.Index(rest, "_"); idx >= 0 {
		return rest[:idx]
	}
	return rest
}

func ParseTitle(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	matches := twitterFilePattern.FindStringSubmatch(base)
	if len(matches) < 2 {
		return base
	}
	rest := matches[1]
	if strings.HasPrefix(rest, "_") {
		if idx := strings.Index(rest[1:], "_"); idx >= 0 {
			return strings.TrimSpace(rest[idx+2:])
		}
		return rest
	}
	if idx := strings.Index(rest, "_"); idx >= 0 {
		return strings.TrimSpace(rest[idx+1:])
	}
	return rest
}

func MediaType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mov", ".m4v", ".webm":
		return "video"
	default:
		return ""
	}
}

func CoverType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return "image"
	default:
		return ""
	}
}

type metadata struct {
	AwemeID     string
	Title       string
	Description string
	MusicTitle  string
	SourceURL   string
	Tags        []string
	PublishedAt string
}

func parseMetadata(path string) metadata {
	var raw struct {
		ID           string          `json:"ID"`
		Text         string          `json:"Text"`
		HTML         string          `json:"HTML"`
		Name         string          `json:"Name"`
		Username     string          `json:"Username"`
		TimeParsed   string          `json:"TimeParsed"`
		Timestamp    int64           `json:"Timestamp"`
		PermanentURL string          `json:"PermanentURL"`
		Hashtags     json.RawMessage `json:"Hashtags"`
		Mentions     []struct {
			Name     string `json:"Name"`
			Username string `json:"Username"`
		} `json:"Mentions"`
		URLs []string `json:"URLs"`
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &raw) != nil {
		return metadata{}
	}
	description := strings.TrimSpace(raw.Text)
	if description == "" {
		description = stripHTML(raw.HTML)
	}
	meta := metadata{
		AwemeID:     raw.ID,
		Title:       firstNonEmpty(description, raw.PermanentURL, raw.ID),
		Description: description,
		MusicTitle:  "Twitter",
		SourceURL:   raw.PermanentURL,
	}
	if raw.TimeParsed != "" {
		if parsed, err := time.Parse(time.RFC3339, raw.TimeParsed); err == nil {
			meta.PublishedAt = parsed.Format(time.RFC3339)
		} else if parsed, err := time.Parse("2006-01-02T15:04:05Z07:00", raw.TimeParsed); err == nil {
			meta.PublishedAt = parsed.Format(time.RFC3339)
		} else {
			meta.PublishedAt = raw.TimeParsed
		}
	}
	if meta.PublishedAt == "" && raw.Timestamp > 0 {
		meta.PublishedAt = time.Unix(raw.Timestamp, 0).Format(time.RFC3339)
	}
	meta.Tags = append(meta.Tags, raw.Username, raw.Name)
	meta.Tags = append(meta.Tags, decodeStringList(raw.Hashtags)...)
	meta.Tags = append(meta.Tags, hashtags(description)...)
	for _, mention := range raw.Mentions {
		meta.Tags = append(meta.Tags, mention.Username, mention.Name)
	}
	meta.Tags = append(meta.Tags, raw.URLs...)
	meta.Tags = compactStrings(meta.Tags)
	if len(meta.Tags) == 0 {
		meta.Tags = nil
	}
	return meta
}

func hashtags(desc string) []string {
	parts := strings.Split(desc, "#")
	if len(parts) < 2 {
		return nil
	}
	var tags []string
	for _, part := range parts[1:] {
		tag := strings.Fields(part)
		if len(tag) > 0 {
			tags = append(tags, tag[0])
		}
	}
	return tags
}

func stripHTML(value string) string {
	replacer := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&#39;", "'", "&quot;", `"`)
	value = replacer.Replace(value)
	var out strings.Builder
	inTag := false
	for _, r := range value {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				out.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(out.String())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func decodeStringList(data json.RawMessage) []string {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	var stringsOnly []string
	if json.Unmarshal(data, &stringsOnly) == nil {
		return stringsOnly
	}
	var objects []struct {
		Text string `json:"Text"`
		Name string `json:"Name"`
	}
	if json.Unmarshal(data, &objects) == nil {
		var values []string
		for _, object := range objects {
			values = append(values, object.Text, object.Name)
		}
		return values
	}
	return nil
}

func compactStrings(values []string) []string {
	seen := map[string]bool{}
	var compact []string
	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(value, "@"))
		if value == "" || seen[strings.ToLower(value)] {
			continue
		}
		seen[strings.ToLower(value)] = true
		compact = append(compact, value)
	}
	return compact
}
