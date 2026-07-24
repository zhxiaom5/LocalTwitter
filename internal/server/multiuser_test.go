package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestMultiUserMigrationPreservesLegacyStateForSuperAdmin(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_salt TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  password_iterations INTEGER NOT NULL,
  is_admin INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE favorite_folders (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,
  name TEXT NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE update_auth_settings (
  id INTEGER PRIMARY KEY CHECK(id=1),
  auth_token TEXT NOT NULL DEFAULT '',
  ct0 TEXT NOT NULL DEFAULT '',
  proxy TEXT NOT NULL DEFAULT '',
  fetch_limit INTEGER NOT NULL DEFAULT 300,
  media_only INTEGER NOT NULL DEFAULT 1,
  include_retweets INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT '',
  validation_status TEXT NOT NULL DEFAULT 'unknown',
  validation_message TEXT NOT NULL DEFAULT '',
  cookie_header TEXT NOT NULL DEFAULT '',
  download_mode TEXT NOT NULL DEFAULT 'original',
  download_directory TEXT NOT NULL DEFAULT '',
  downloader_concurrency INTEGER NOT NULL DEFAULT 1
);
INSERT INTO users(username,password_salt,password_hash,password_iterations,is_admin) VALUES('ted','salt','hash',1,1);
INSERT INTO favorite_folders(type,name,is_default) VALUES('work','旧作品收藏夹',1),('creator','旧作者收藏夹',1);
INSERT INTO update_auth_settings(id,auth_token,ct0,proxy,fetch_limit,media_only,cookie_header,download_mode,download_directory,downloader_concurrency)
VALUES(1,'auth-one','ct0-one','socks5://127.0.0.1:1080',88,1,'auth_token=legacy; ct0=legacy','custom','/legacy/twitter',3);
`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ted, err := store.UserByUsername("ted")
	if err != nil {
		t.Fatal(err)
	}
	if ted.Role != userRoleSuperAdmin || !ted.IsSuperAdmin || !ted.IsAdmin {
		t.Fatalf("ted role = %#v", ted)
	}
	folders, err := store.FavoriteFoldersForUser(ted.ID, "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || folders[0].Name != "旧作品收藏夹" || folders[0].UserID != ted.ID {
		t.Fatalf("migrated folders = %#v", folders)
	}
	auth, err := store.UpdateAuthForUser(ted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if auth.AuthToken != "auth-one" || auth.CT0 != "ct0-one" || !auth.CookieHeaderSaved || auth.DownloadDirectory != "/legacy/twitter" || auth.DownloaderConcurrency != 3 {
		t.Fatalf("migrated auth = %#v", auth)
	}

	alice, err := store.CreateUser("alice", "alice123")
	if err != nil {
		t.Fatal(err)
	}
	if alice.Role != userRoleAdmin || alice.IsSuperAdmin || alice.IsAdmin || !alice.CanUpdate {
		t.Fatalf("created user role = %#v", alice)
	}
	aliceFolders, err := store.FavoriteFoldersForUser(alice.ID, "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceFolders) != 1 || aliceFolders[0].UserID != alice.ID {
		t.Fatalf("alice default folders = %#v", aliceFolders)
	}
}

func TestUserScopedFavoritesUpdateAuthVisibilityAndLogs(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	creator, _, err := store.UpsertCreatorWithWorks(dir.ID, "creator-a", []WorkInput{{
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
	ted, err := store.UserByUsername("ted")
	if err != nil {
		t.Fatal(err)
	}
	alice, err := store.CreateUser("alice", "alice123")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.AddWorkFavoriteForUser(ted.ID, works[0].ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := store.AddCreatorFavoriteForUser(alice.ID, creator.ID, 0); err != nil {
		t.Fatal(err)
	}
	tedStatus, err := store.FavoriteStatusForUser(ted.ID, works[0].ID, creator.ID)
	if err != nil {
		t.Fatal(err)
	}
	aliceStatus, err := store.FavoriteStatusForUser(alice.ID, works[0].ID, creator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !tedStatus.WorkFavorited || tedStatus.CreatorFavorited || aliceStatus.WorkFavorited || !aliceStatus.CreatorFavorited {
		t.Fatalf("scoped favorite status ted=%#v alice=%#v", tedStatus, aliceStatus)
	}

	if err := store.SaveUpdateAuthForUser(ted.ID, UpdateAuthInput{AuthToken: stringPtr("ted-auth"), CT0: stringPtr("ted-ct0")}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUpdateAuthForUser(alice.ID, UpdateAuthInput{AuthToken: stringPtr("alice-auth"), CT0: stringPtr("alice-ct0")}); err != nil {
		t.Fatal(err)
	}
	tedAuth, _ := store.UpdateAuthForUser(ted.ID)
	aliceAuth, _ := store.UpdateAuthForUser(alice.ID)
	if tedAuth.AuthToken != "ted-auth" || aliceAuth.AuthToken != "alice-auth" {
		t.Fatalf("scoped auth ted=%#v alice=%#v", tedAuth, aliceAuth)
	}

	if err := store.SetHiddenCreatorsForUser(alice.ID, []int64{creator.ID}); err != nil {
		t.Fatal(err)
	}
	visible, err := store.CreatorsForUser(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("hidden creator should not be visible: %#v", visible)
	}
	if err := store.RecordAuditEvent(AuditEventInput{ActorUserID: ted.ID, EventType: "hide_creator", TargetType: "creator", TargetID: creator.ID, Message: "隐藏推主"}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordSystemEvent(SystemEventInput{EventType: "scan_completed", Severity: "info", Message: "扫描完成"}); err != nil {
		t.Fatal(err)
	}
	audit, err := store.AuditEvents(LogQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	system, err := store.SystemEvents(LogQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if audit.Total != 1 || system.Total != 1 {
		t.Fatalf("logs audit=%#v system=%#v", audit, system)
	}
}

func TestSuperAdminViewContextAndLogAPIs(t *testing.T) {
	app := mustTestApp(t, newTestStore(t))
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	superClient := cookieClient()
	adminClient := cookieClient()

	login(t, superClient, server.URL, "ted", "ted")
	alice := createUser(t, superClient, server.URL, "alice", "alice123")
	logout(t, superClient, server.URL)

	login(t, adminClient, server.URL, "alice", "alice123")
	assertStatus(t, adminClient, http.MethodGet, server.URL+"/api/logs/audit", nil, http.StatusForbidden)
	assertStatus(t, adminClient, http.MethodPut, server.URL+"/api/users/view-context", bytes.NewReader([]byte(`{"user_id":1}`)), http.StatusForbidden)

	login(t, superClient, server.URL, "ted", "ted")
	body, _ := json.Marshal(map[string]int64{"user_id": alice.ID})
	var contextResponse struct {
		Actor User `json:"actor_user"`
		View  User `json:"view_user"`
	}
	decodeJSONResponse(t, superClient, http.MethodPut, server.URL+"/api/users/view-context", bytes.NewReader(body), http.StatusOK, &contextResponse)
	if contextResponse.Actor.Username != "ted" || contextResponse.View.Username != "alice" {
		t.Fatalf("view context = %#v", contextResponse)
	}
	assertStatus(t, superClient, http.MethodGet, server.URL+"/api/logs/audit", nil, http.StatusOK)
}

func TestSuperAdminViewContextWritesUserScopedFavorites(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	creator, _, err := store.UpsertCreatorWithWorks(dir.ID, "creator-a", []WorkInput{{
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
	app := mustTestApp(t, store)
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()

	login(t, client, server.URL, "ted", "ted")
	alice := createUser(t, client, server.URL, "alice", "alice123")
	body, _ := json.Marshal(map[string]int64{"user_id": alice.ID})
	assertStatus(t, client, http.MethodPut, server.URL+"/api/users/view-context", bytes.NewReader(body), http.StatusOK)

	favoriteBody, _ := json.Marshal(map[string]int64{"work_id": works[0].ID})
	assertStatus(t, client, http.MethodPost, server.URL+"/api/favorites/works", bytes.NewReader(favoriteBody), http.StatusOK)
	creatorBody, _ := json.Marshal(map[string]int64{"creator_id": creator.ID})
	assertStatus(t, client, http.MethodPost, server.URL+"/api/favorites/creators", bytes.NewReader(creatorBody), http.StatusOK)

	ted, err := store.UserByUsername("ted")
	if err != nil {
		t.Fatal(err)
	}
	tedStatus, err := store.FavoriteStatusForUser(ted.ID, works[0].ID, creator.ID)
	if err != nil {
		t.Fatal(err)
	}
	aliceStatus, err := store.FavoriteStatusForUser(alice.ID, works[0].ID, creator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tedStatus.WorkFavorited || tedStatus.CreatorFavorited {
		t.Fatalf("actor favorites should stay untouched: %#v", tedStatus)
	}
	if !aliceStatus.WorkFavorited || !aliceStatus.CreatorFavorited {
		t.Fatalf("view user favorites should be written: %#v", aliceStatus)
	}
}

func TestAboutAPIReportsBuildInformation(t *testing.T) {
	app := mustTestApp(t, newTestStore(t))
	req := httptest.NewRequest(http.MethodGet, "/api/about", nil)
	setTestSessionCookie(t, app.store, req)
	rec := httptest.NewRecorder()
	app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		About AboutInfo `json:"about"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.About.Name != "LocalTwitter" || response.About.Version == "" {
		t.Fatalf("about = %#v", response.About)
	}
}

func waitTiny() {
	time.Sleep(10 * time.Millisecond)
}
