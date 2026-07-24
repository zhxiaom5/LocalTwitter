package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAccessLogWritesCombinedFormatForHTTPRequests(t *testing.T) {
	accessLog := filepath.Join(t.TempDir(), "logs", "access.log")
	t.Setenv("LOCALTWITTER_ACCESS_LOG", accessLog)

	app, err := NewAppWithAssets(filepath.Join(t.TempDir(), "app.db"), WebAssets{FS: testWebFS()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	store := app.currentStore()
	mediaPath := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(mediaPath, []byte("0123456789abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpsertCreatorWithWorks(dir.ID, "creator", []WorkInput{{
		AwemeID: "access-log-1",
		Title:   "access",
		Media:   []MediaInput{{Type: "video", FilePath: mediaPath, FileName: filepath.Base(mediaPath)}},
	}}); err != nil {
		t.Fatal(err)
	}
	works, err := store.Feed(0, 0, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	mediaID := works[0].Media[0].ID

	routes := app.Routes()
	requests := []*http.Request{
		accessLogRequest(http.MethodGet, "/api/about", nil),
		accessLogRequest(http.MethodGet, "/assets/app.js", nil),
		accessLogRequest(http.MethodGet, "/assets/missing.js", nil),
		accessLogRequest(http.MethodGet, "/api/media/"+strconvID(mediaID), func(req *http.Request) {
			setTestSessionCookie(t, store, req)
			req.Header.Set("Range", "bytes=0-3")
		}),
	}
	for _, req := range requests {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
	}

	lines := readAccessLogLines(t, accessLog)
	if len(lines) != len(requests) {
		t.Fatalf("access log lines = %d, want %d: %#v", len(lines), len(requests), lines)
	}
	patterns := []string{
		`^192\.0\.2\.1 - - \[\d{2}/[A-Z][a-z]{2}/\d{4}:\d{2}:\d{2}:\d{2} [+-]\d{4}\] "GET /api/about HTTP/1\.1" 401 \d+ "https://example\.test/from" "access-test" "203\.0\.113\.9" rt=\d+\.\d{3} request_time=\d+\.\d{3} request_length=\d+ host="example\.com" range="-"$`,
		`"GET /assets/app\.js HTTP/1\.1" 200 \d+ "https://example\.test/from" "access-test" "203\.0\.113\.9" rt=\d+\.\d{3} request_time=\d+\.\d{3} request_length=\d+ host="example\.com" range="-"$`,
		`"GET /assets/missing\.js HTTP/1\.1" 404 \d+ "https://example\.test/from" "access-test" "203\.0\.113\.9" rt=\d+\.\d{3} request_time=\d+\.\d{3} request_length=\d+ host="example\.com" range="-"$`,
		`"GET /api/media/\d+ HTTP/1\.1" 206 \d+ "https://example\.test/from" "access-test" "203\.0\.113\.9" rt=\d+\.\d{3} request_time=\d+\.\d{3} request_length=\d+ host="example\.com" range="bytes=0-3"$`,
	}
	for i, pattern := range patterns {
		if !regexp.MustCompile(pattern).MatchString(lines[i]) {
			t.Fatalf("line %d = %q, want pattern %s", i, lines[i], pattern)
		}
	}
}

func TestAccessLogDoesNotWriteSQLiteEvents(t *testing.T) {
	accessLog := filepath.Join(t.TempDir(), "access.log")
	t.Setenv("LOCALTWITTER_ACCESS_LOG", accessLog)

	app, err := NewAppWithAssets(filepath.Join(t.TempDir(), "app.db"), WebAssets{FS: testWebFS()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })

	routes := app.Routes()
	for _, path := range []string{"/assets/app.js", "/assets/missing.js"} {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, accessLogRequest(http.MethodGet, path, nil))
	}
	if len(readAccessLogLines(t, accessLog)) != 2 {
		t.Fatal("expected access log file entries")
	}
	audit, err := app.currentStore().AuditEvents(LogQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	system, err := app.currentStore().SystemEvents(LogQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.Events) != 0 || len(system.Events) != 0 {
		t.Fatalf("access log should not write sqlite events, audit=%#v system=%#v", audit.Events, system.Events)
	}
}

func TestAccessLogRotatesAndKeepsThreeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	logger := newAccessLogger(path, 60, 3)
	for i := 0; i < 4; i++ {
		if err := logger.write(strings.Repeat(string(rune('a'+i)), 40) + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{path, path + ".1", path + ".2"} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("expected rotated log %s: %v", name, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("unexpected fourth log file err=%v", err)
	}
}

func accessLogRequest(method, target string, configure func(*http.Request)) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = "192.0.2.1:12345"
	req.Header.Set("Referer", "https://example.test/from")
	req.Header.Set("User-Agent", "access-test")
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	if configure != nil {
		configure(req)
	}
	return req
}

func readAccessLogLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}
