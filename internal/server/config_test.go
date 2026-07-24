package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredAppCreatesDatabaseInDirectoryAndSwitches(t *testing.T) {
	firstDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "localtwitter.config.json")
	app, err := NewConfiguredApp(configPath, firstDir, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if _, err := os.Stat(filepath.Join(firstDir, DatabaseFileName)); err != nil {
		t.Fatalf("database was not created: %v", err)
	}
	dir, err := app.store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if dir.ID == 0 {
		t.Fatal("expected directory in first database")
	}

	secondDir := t.TempDir()
	body, _ := json.Marshal(map[string]string{"directory": secondDir})
	req := httptest.NewRequest(http.MethodPut, "/api/config/database", bytes.NewReader(body))
	setTestSessionCookie(t, app.store, req)
	rec := httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(secondDir, DatabaseFileName)); err != nil {
		t.Fatalf("switched database was not created: %v", err)
	}
	dirs, err := app.store.Directories()
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 0 {
		t.Fatalf("switched database should be empty, got %#v", dirs)
	}
}

func TestSwitchDatabaseRejectsRunningScan(t *testing.T) {
	app, err := NewApp(filepath.Join(t.TempDir(), "app.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	app.scanner.mu.Lock()
	app.scanner.running = true
	app.scanner.status.Running = true
	app.scanner.mu.Unlock()

	body, _ := json.Marshal(map[string]string{"directory": t.TempDir()})
	req := httptest.NewRequest(http.MethodPut, "/api/config/database", bytes.NewReader(body))
	setTestSessionCookie(t, app.store, req)
	rec := httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
