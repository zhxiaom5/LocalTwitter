package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestAPIDirectoryScanAndImmediateFeed(t *testing.T) {
	root := t.TempDir()
	creator := filepath.Join(root, "creator-a")
	if err := os.Mkdir(creator, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(creator, "2026-04-08_WHToaPYvsw8d0-eZ_作品.json"), []byte(`{"ID":"2041001","Text":"作品","Username":"creator-a","TimeParsed":"2026-04-08T10:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(creator, "2026-04-08_WHToaPYvsw8d0-eZ_作品.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(filepath.Join(t.TempDir(), "app.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()
	login(t, client, server.URL, "ted", "ted")

	body, _ := json.Marshal(map[string]string{"path": root})
	resp, err := client.Post(server.URL+"/api/directories", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var created struct {
		Directory Directory `json:"directory`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/directories/"+strconvID(created.Directory.ID)+"/scan", nil)
	if _, err := client.Do(req); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		feedResp, err := client.Get(server.URL + "/api/feed")
		if err != nil {
			t.Fatal(err)
		}
		var feed struct {
			Works []Work `json:"works`
		}
		_ = json.NewDecoder(feedResp.Body).Decode(&feed)
		_ = feedResp.Body.Close()
		if len(feed.Works) == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("feed did not become visible after scan")
}

func TestUnifiedEventsStreamsScanAndUpdate(t *testing.T) {
	app, err := NewApp(filepath.Join(t.TempDir(), "app.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()
	login(t, client, server.URL, "ted", "ted")

	resp := openEventStream(t, client, server.URL+"/api/events")
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	app.hub.Publish(ScanEvent{Type: "completed", Status: ScanStatus{State: "completed"}})
	name, data := readSSEEvent(t, reader)
	if name != "scan" || !strings.Contains(data, `"type":"completed"`) {
		t.Fatalf("first event = %q %q, want scan completed", name, data)
	}

	app.hub.PublishUpdate(UpdateEvent{Type: "queued", Status: UpdateStatus{State: "queued"}})
	name, data = readSSEEvent(t, reader)
	if name != "update" || !strings.Contains(data, `"type":"queued"`) {
		t.Fatalf("second event = %q %q, want update queued", name, data)
	}
}

func TestLegacyEventStreamsOnlyReceiveTheirOwnEventType(t *testing.T) {
	app, err := NewApp(filepath.Join(t.TempDir(), "app.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()
	login(t, client, server.URL, "ted", "ted")

	scanResp := openEventStream(t, client, server.URL+"/api/scan/events")
	defer scanResp.Body.Close()
	scanReader := bufio.NewReader(scanResp.Body)
	app.hub.PublishUpdate(UpdateEvent{Type: "queued", Status: UpdateStatus{State: "queued"}})
	app.hub.Publish(ScanEvent{Type: "completed", Status: ScanStatus{State: "completed"}})
	name, data := readSSEEvent(t, scanReader)
	if name != "scan" || strings.Contains(data, `"queued"`) {
		t.Fatalf("legacy scan event = %q %q, want scan only", name, data)
	}

	updateResp := openEventStream(t, client, server.URL+"/api/update/events")
	defer updateResp.Body.Close()
	updateReader := bufio.NewReader(updateResp.Body)
	app.hub.Publish(ScanEvent{Type: "completed", Status: ScanStatus{State: "completed"}})
	app.hub.PublishUpdate(UpdateEvent{Type: "running", Status: UpdateStatus{State: "running"}})
	name, data = readSSEEvent(t, updateReader)
	if name != "update" || strings.Contains(data, `"completed"`) {
		t.Fatalf("legacy update event = %q %q, want update only", name, data)
	}
}

func openEventStream(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		t.Fatalf("event stream status = %d, body=%s", resp.StatusCode, readBody(t, resp))
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		resp.Body.Close()
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	return resp
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) (string, string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var eventName string
	var eventData string
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			eventName = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			eventData = strings.TrimPrefix(line, "data: ")
		case line == "" && eventName != "":
			return eventName, eventData
		}
	}
	t.Fatal("timed out waiting for SSE event")
	return "", ""
}

func TestScanSkipsCreatorWithoutVideoWorks(t *testing.T) {
	root := t.TempDir()
	emptyCreator := filepath.Join(root, "empty-creator")
	filledCreator := filepath.Join(root, "filled-creator")
	if err := os.Mkdir(emptyCreator, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filledCreator, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptyCreator, "2026-04-08_empty.json"), []byte(`{"ID":"empty","Text":"empty"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptyCreator, "2026-04-08_empty.jpg"), []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filledCreator, "2026-04-08_video.json"), []byte(`{"ID":"video","Text":"video"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filledCreator, "2026-04-08_video.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(filepath.Join(t.TempDir(), "app.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	dir, err := app.store.SaveDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	app.scanner.Enqueue(dir.ID)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := app.scanner.Status()
		if !status.Running && status.State == "idle" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	creators, err := app.store.Creators()
	if err != nil {
		t.Fatal(err)
	}
	if len(creators) != 1 || creators[0].Name != "filled-creator" {
		t.Fatalf("empty creator should be skipped, got %#v", creators)
	}
}

func TestRecordWorkViewAPIRequiresLoginAndAggregates(t *testing.T) {
	app, err := NewApp(filepath.Join(t.TempDir(), "app.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	dir, err := app.store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	creator, _, err := app.store.UpsertCreatorWithWorks(dir.ID, "creator-a", []WorkInput{{
		AwemeID: "1001",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(t.TempDir(), "a.mp4"), FileName: "a.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	works, err := app.store.Feed(creator.ID, 0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 1 {
		t.Fatalf("works = %#v", works)
	}
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/works/"+strconvID(works[0].ID)+"/view", nil)
	unauthenticated, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthenticated.Body.Close()
	if unauthenticated.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", unauthenticated.StatusCode)
	}

	client := cookieClient()
	login(t, client, server.URL, "ted", "ted")
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/works/"+strconvID(works[0].ID)+"/view", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("record view status = %d, want 200", resp.StatusCode)
		}
	}
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/api/works/999999/view", nil)
	missing, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing work status = %d, want 404", missing.StatusCode)
	}
	creators, err := app.store.Creators()
	if err != nil {
		t.Fatal(err)
	}
	if len(creators) != 1 || creators[0].ViewCount != 2 {
		t.Fatalf("creators = %#v, want view count 2", creators)
	}
}

func TestDirectoryScanIncludesNestedAndFlatCreatorVideoDirs(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "dahuoluowan", "dahuoluowan", "video")
	flat := filepath.Join(root, "dahuoluowan", "video")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(flat, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "2026-04-08_nested_旧作品.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flat, "2026-04-09_flat_新作品.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(filepath.Join(t.TempDir(), "app.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	dir, err := app.store.SaveDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	app.scanner.Enqueue(dir.ID)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := app.scanner.Status()
		if !status.Running && status.State == "idle" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	works, err := app.store.Feed(0, 0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 2 {
		t.Fatalf("works = %#v, want two works from nested and flat dirs", works)
	}
}

func TestAPIScanCreatorOnlyScansTargetCreator(t *testing.T) {
	root := t.TempDir()
	targetNested := filepath.Join(root, "target", "target", "video")
	targetFlat := filepath.Join(root, "target", "video")
	other := filepath.Join(root, "other")
	for _, dir := range []string{targetNested, targetFlat, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(targetNested, "2026-04-08_nested_嵌套作品.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetFlat, "2026-04-09_flat_扁平作品.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "2026-04-10_other_其他作品.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(filepath.Join(t.TempDir(), "app.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	dir, err := app.store.SaveDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	seedPath := filepath.Join(t.TempDir(), "seed.mp4")
	if err := os.WriteFile(seedPath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	creator, _, err := app.store.UpsertCreatorWithWorksWithIdentity(dir.ID, "target", CreatorIdentity{TwitterUsername: "target", ProfileURL: "https://x.com/target"}, []WorkInput{{
		AwemeID: "seed",
		Title:   "seed",
		Media:   []MediaInput{{Type: "video", FilePath: seedPath, FileName: "seed.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/creators/"+strconvID(creator.ID)+"/scan", nil)
	setTestSessionCookie(t, app.store, req)
	rec := httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := app.scanner.Status()
		if !status.Running && status.State == "idle" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	count, err := app.store.CreatorWorkCount(creator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("target work count = %d, want 3", count)
	}
	creators, err := app.store.Creators()
	if err != nil {
		t.Fatal(err)
	}
	if len(creators) != 1 || creators[0].Name != "target" {
		t.Fatalf("single creator scan should not import other creators, got %#v", creators)
	}
}

func TestMediaSupportsRange(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(mediaPath, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = store.UpsertCreatorWithWorks(dir.ID, "creator", []WorkInput{{
		AwemeID: "1",
		Title:   "clip",
		Media:   []MediaInput{{Type: "video", FilePath: mediaPath, FileName: "clip.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: store, scanner: NewScanner(store, NewEventHub()), hub: NewEventHub()}
	works, err := store.Feed(0, 0, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/media/"+strconvID(works[0].Media[0].ID), nil)
	setTestSessionCookie(t, store, req)
	req.Header.Set("Range", "bytes=0-3")
	rec := httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges = %q, want bytes", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("Content-Type = %q, want video/mp4", got)
	}
}

func TestAPICreatorLinksAndAvatar(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mediaDir := t.TempDir()
	first, _, err := store.UpsertCreatorWithWorks(dir.ID, "oldname", []WorkInput{{
		AwemeID: "old",
		Title:   "old",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(mediaDir, "old.mp4"), FileName: "old.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.UpsertCreatorWithWorks(dir.ID, "newname", []WorkInput{{
		AwemeID: "new",
		Title:   "new",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(mediaDir, "new.mp4"), FileName: "new.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	avatarPath := filepath.Join(t.TempDir(), "avatar.jpg")
	if err := os.WriteFile(avatarPath, []byte("avatar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateCreatorProfile(first.ID, CreatorProfileInput{AvatarURL: "https://pbs.twimg.com/avatar.jpg", AvatarFile: avatarPath, Bio: "签名"}); err != nil {
		t.Fatal(err)
	}
	app := &App{store: store, scanner: NewScanner(store, NewEventHub()), hub: NewEventHub(), updater: NewUpdateManager(store, NewEventHub(), fakeDownloader{})}

	body, _ := json.Marshal(map[string][]int64{"creator_ids": []int64{second.ID}})
	req := httptest.NewRequest(http.MethodPut, "/api/creators/"+strconvID(first.ID)+"/links", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setTestSessionCookie(t, store, req)
	rec := httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save links status = %d, body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/creators/"+strconvID(first.ID)+"/links", nil)
	setTestSessionCookie(t, store, req)
	rec = httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("links status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var links CreatorLinksResult
	if err := json.NewDecoder(rec.Body).Decode(&links); err != nil {
		t.Fatal(err)
	}
	if len(links.Linked) != 1 || links.Linked[0].ID != second.ID {
		t.Fatalf("links = %#v", links)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/creators/"+strconvID(first.ID)+"/avatar", nil)
	setTestSessionCookie(t, store, req)
	rec = httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "avatar" {
		t.Fatalf("avatar status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestEmbeddedWebServesSPAAndKeepsAPIJSON(t *testing.T) {
	app, err := NewAppWithAssets(filepath.Join(t.TempDir(), "app.db"), WebAssets{FS: testWebFS()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })

	for _, path := range []string{"/", "/creators", "/settings"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		app.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body=%s", path, rec.Code, rec.Body.String())
		}
		if rec.Body.String() != "<html>embedded</html>" {
			t.Fatalf("%s body = %q", path, rec.Body.String())
		}
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/creators", nil)
	setTestSessionCookie(t, app.store, apiReq)
	apiRec := httptest.NewRecorder()
	app.Routes().ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("api status = %d, body=%s", apiRec.Code, apiRec.Body.String())
	}
	if contentType := apiRec.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("api content type = %q", contentType)
	}
}

func TestCreateDirectoryRejectsUnauthorizedPath(t *testing.T) {
	allowed := t.TempDir()
	blocked := t.TempDir()
	app, err := NewAppWithAssets(filepath.Join(t.TempDir(), "app.db"), WebAssets{FS: testWebFS()})
	if err != nil {
		t.Fatal(err)
	}
	app.browser = FileBrowser{allowedPaths: []string{allowed}}
	t.Cleanup(func() { _ = app.Close() })

	body, _ := json.Marshal(map[string]string{"path": blocked})
	req := httptest.NewRequest(http.MethodPost, "/api/directories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setTestSessionCookie(t, app.store, req)
	rec := httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("blocked status = %d, body=%s", rec.Code, rec.Body.String())
	}

	body, _ = json.Marshal(map[string]string{"path": allowed})
	req = httptest.NewRequest(http.MethodPost, "/api/directories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setTestSessionCookie(t, app.store, req)
	rec = httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("allowed status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestEmbeddedWebServesAssetsAndMissingAssets404(t *testing.T) {
	app, err := NewAppWithAssets(filepath.Join(t.TempDir(), "app.db"), WebAssets{FS: testWebFS()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("asset status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "console.log('embedded')" {
		t.Fatalf("asset body = %q", rec.Body.String())
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	missingRec := httptest.NewRecorder()
	app.Routes().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d, body=%s", missingRec.Code, missingRec.Body.String())
	}
}

func TestExternalWebDirOverridesEmbeddedAssets(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<html>external</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := NewAppWithAssets(filepath.Join(t.TempDir(), "app.db"), WebAssets{Dir: webDir, FS: testWebFS()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "<html>external</html>" {
		t.Fatalf("body = %q", string(body))
	}
}

func testWebFS() fs.FS {
	return fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>embedded</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('embedded')")},
	}
}

func setTestSessionCookie(t *testing.T, store *Store, req *http.Request) {
	t.Helper()
	user, err := store.Authenticate("ted", "ted")
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.CreateSession(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(store.SessionCookie(token))
}

func strconvID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func TestPlayerLogEndpointWritesSystemEvent(t *testing.T) {
	app, err := NewApp(filepath.Join(t.TempDir(), "app.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()
	login(t, client, server.URL, "ted", "ted")

	body, _ := json.Marshal(map[string]any{
		"action":           "can_play",
		"play_session_id":  "play-test-1",
		"work_id":          123,
		"media_id":         456,
		"load_time_ms":     800,
		"metadata_load_ms": 300,
		"loaded_data_ms":   350,
		"preloaded":        false,
	})
	resp, err := client.Post(server.URL+"/api/logs/player", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result struct {
		Ok bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Ok {
		t.Fatal("expected ok=true")
	}

	// Verify system event was written
	systemResp, err := client.Get(server.URL + "/api/logs/system?event_type=player_load")
	if err != nil {
		t.Fatal(err)
	}
	defer systemResp.Body.Close()
	var page struct {
		Events []LogEvent `json:"events"`
	}
	if err := json.NewDecoder(systemResp.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Events) < 1 {
		t.Fatal("expected at least 1 player_load event")
	}
	found := false
	for _, ev := range page.Events {
		if ev.EventType == "player_load" {
			if !strings.Contains(ev.DetailJSON, `"play_session_id":"play-test-1"`) || !strings.Contains(ev.DetailJSON, `"loaded_data_ms":350`) {
				t.Fatalf("missing player timing detail: %s", ev.DetailJSON)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected player_load event in system logs")
	}
}

func TestMediaRequestWritesPlayerTimingAuditEvent(t *testing.T) {
	app, err := NewApp(filepath.Join(t.TempDir(), "app.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	store := app.currentStore()
	mediaPath := filepath.Join(t.TempDir(), "slow-file-name-with-many-characters.mp4")
	if err := os.WriteFile(mediaPath, []byte("0123456789abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpsertCreatorWithWorks(dir.ID, "creator", []WorkInput{{
		AwemeID: "timing-1",
		Title:   "timing",
		Media:   []MediaInput{{Type: "video", FilePath: mediaPath, FileName: filepath.Base(mediaPath)}},
	}}); err != nil {
		t.Fatal(err)
	}
	works, err := store.Feed(0, 0, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	mediaID := works[0].Media[0].ID

	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()
	login(t, client, server.URL, "ted", "ted")

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/media/"+strconvID(mediaID)+"?play_session_id=session-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-3")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	page, err := store.AuditEvents(LogQuery{EventType: "video_stream", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("events = %#v", page.Events)
	}
	event := page.Events[0]
	if event.TargetType != "media" || event.TargetID != mediaID {
		t.Fatalf("target = %s #%d", event.TargetType, event.TargetID)
	}
	if !strings.Contains(event.Message, filepath.Base(mediaPath)) ||
		!strings.Contains(event.DetailJSON, `"range_header":"bytes=0-3"`) ||
		!strings.Contains(event.DetailJSON, `"play_session_id":"session-1"`) ||
		!strings.Contains(event.DetailJSON, `"range_start":0`) ||
		!strings.Contains(event.DetailJSON, `"write_ms"`) ||
		!strings.Contains(event.DetailJSON, `"write_wait_max_ms"`) ||
		!strings.Contains(event.DetailJSON, `"read_wait_max_ms"`) ||
		!strings.Contains(event.DetailJSON, `"client_aborted"`) ||
		!strings.Contains(event.DetailJSON, `"total_ms"`) {
		t.Fatalf("unexpected log message=%q detail=%s", event.Message, event.DetailJSON)
	}
}

func TestFeedRequestWritesTimingLog(t *testing.T) {
	app, err := NewApp(filepath.Join(t.TempDir(), "app.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	store := app.currentStore()
	mediaPath := filepath.Join(t.TempDir(), "feed.mp4")
	if err := os.WriteFile(mediaPath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpsertCreatorWithWorks(dir.ID, "creator", []WorkInput{{
		AwemeID: "feed-1",
		Title:   "feed",
		Media:   []MediaInput{{Type: "video", FilePath: mediaPath, FileName: filepath.Base(mediaPath)}},
	}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()
	login(t, client, server.URL, "ted", "ted")

	resp, err := client.Get(server.URL + "/api/feed?limit=1")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	page, err := store.SystemEvents(LogQuery{EventType: "feed_load", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("events = %#v", page.Events)
	}
	if !strings.Contains(page.Events[0].DetailJSON, `"query_ms"`) ||
		!strings.Contains(page.Events[0].DetailJSON, `"json_ms"`) ||
		!strings.Contains(page.Events[0].DetailJSON, `"works_count":1`) {
		t.Fatalf("missing feed timing detail: %s", page.Events[0].DetailJSON)
	}
}
