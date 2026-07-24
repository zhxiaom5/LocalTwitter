package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSeedsDefaultAdminUser(t *testing.T) {
	store := newTestStore(t)
	users, err := store.Users()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("users len = %d, want 1", len(users))
	}
	if users[0].Username != "ted" || !users[0].IsAdmin {
		t.Fatalf("default user = %#v", users[0])
	}
	if _, err := store.Authenticate("ted", "ted"); err != nil {
		t.Fatalf("default admin should authenticate: %v", err)
	}
}

func TestAuthLifecycleAndProtectedRoutes(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(mediaPath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpsertCreatorWithWorks(dir.ID, "creator", []WorkInput{{
		AwemeID: "1001",
		Title:   "作品",
		Media:   []MediaInput{{Type: "video", FilePath: mediaPath, FileName: "video.mp4"}},
	}}); err != nil {
		t.Fatal(err)
	}
	app := mustTestApp(t, store)
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()

	assertStatus(t, client, http.MethodGet, server.URL+"/api/feed", nil, http.StatusUnauthorized)
	assertStatus(t, client, http.MethodGet, server.URL+"/api/media/1", nil, http.StatusUnauthorized)
	assertStatus(t, client, http.MethodGet, server.URL+"/api/events", nil, http.StatusUnauthorized)
	assertStatus(t, client, http.MethodGet, server.URL+"/api/scan/events", nil, http.StatusUnauthorized)

	login(t, client, server.URL, "ted", "ted")
	me := getCurrentUser(t, client, server.URL)
	if me.Username != "ted" || !me.IsAdmin {
		t.Fatalf("current user = %#v", me)
	}
	assertStatus(t, client, http.MethodGet, server.URL+"/api/feed", nil, http.StatusOK)

	changePassword(t, client, server.URL, "ted", "new-ted")
	logout(t, client, server.URL)
	assertStatus(t, client, http.MethodGet, server.URL+"/api/feed", nil, http.StatusUnauthorized)
	login(t, client, server.URL, "ted", "new-ted")
}

func TestAdminUserManagement(t *testing.T) {
	app := mustTestApp(t, newTestStore(t))
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()

	login(t, client, server.URL, "ted", "ted")
	created := createUser(t, client, server.URL, "alice", "alice123")
	users := listUsers(t, client, server.URL)
	if len(users) != 2 || !containsUser(users, "alice") {
		t.Fatalf("users = %#v", users)
	}
	resetUserPassword(t, client, server.URL, created.ID, "alice456")
	logout(t, client, server.URL)
	login(t, client, server.URL, "alice", "alice456")

	assertStatus(t, client, http.MethodGet, server.URL+"/api/users", nil, http.StatusForbidden)

	logout(t, client, server.URL)
	login(t, client, server.URL, "ted", "ted")
	assertStatus(t, client, http.MethodDelete, server.URL+"/api/users/"+strconvID(created.ID), nil, http.StatusOK)
}

func TestUpdateJobDeleteAPIRejectsRunningJob(t *testing.T) {
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
	jobs, err := store.AddUpdateJobs([]int64{creator.ID}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkUpdateJobRunning(jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	app := mustTestApp(t, store)
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()
	login(t, client, server.URL, "ted", "ted")

	assertStatus(t, client, http.MethodDelete, server.URL+"/api/update/jobs/"+strconvID(jobs[0].ID), nil, http.StatusConflict)
	if err := store.FinishUpdateJob(jobs[0].ID, "failed", 0, "bad auth"); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, client, http.MethodDelete, server.URL+"/api/update/jobs/"+strconvID(jobs[0].ID), nil, http.StatusOK)
}

func TestAdminCannotDeleteSelf(t *testing.T) {
	app := mustTestApp(t, newTestStore(t))
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()
	login(t, client, server.URL, "ted", "ted")
	self := getCurrentUser(t, client, server.URL)
	assertStatus(t, client, http.MethodDelete, server.URL+"/api/users/"+strconvID(self.ID), nil, http.StatusBadRequest)
}

func mustTestApp(t *testing.T, store *Store) *App {
	t.Helper()
	hub := NewEventHub()
	return &App{store: store, scanner: NewScanner(store, hub), hub: hub, updater: NewUpdateManager(store, hub, fakeDownloader{})}
}

func cookieClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func login(t *testing.T, client *http.Client, baseURL, username, password string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	assertStatus(t, client, http.MethodPost, baseURL+"/api/auth/login", bytes.NewReader(body), http.StatusOK)
}

func logout(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	assertStatus(t, client, http.MethodPost, baseURL+"/api/auth/logout", nil, http.StatusOK)
}

func changePassword(t *testing.T, client *http.Client, baseURL, oldPassword, newPassword string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"old_password": oldPassword, "new_password": newPassword})
	assertStatus(t, client, http.MethodPost, baseURL+"/api/auth/password", bytes.NewReader(body), http.StatusOK)
}

func createUser(t *testing.T, client *http.Client, baseURL, username, password string) User {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	var response struct {
		User User `json:"user"`
	}
	decodeJSONResponse(t, client, http.MethodPost, baseURL+"/api/users", bytes.NewReader(body), http.StatusCreated, &response)
	return response.User
}

func resetUserPassword(t *testing.T, client *http.Client, baseURL string, id int64, password string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password})
	assertStatus(t, client, http.MethodPost, baseURL+"/api/users/"+strconvID(id)+"/reset-password", bytes.NewReader(body), http.StatusOK)
}

func listUsers(t *testing.T, client *http.Client, baseURL string) []User {
	t.Helper()
	var response struct {
		Users []User `json:"users"`
	}
	decodeJSONResponse(t, client, http.MethodGet, baseURL+"/api/users", nil, http.StatusOK, &response)
	return response.Users
}

func getCurrentUser(t *testing.T, client *http.Client, baseURL string) User {
	t.Helper()
	var response struct {
		User User `json:"user"`
	}
	decodeJSONResponse(t, client, http.MethodGet, baseURL+"/api/auth/me", nil, http.StatusOK, &response)
	return response.User
}

func decodeJSONResponse(t *testing.T, client *http.Client, method, url string, body io.Reader, wantStatus int, out any) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, body=%s", method, url, resp.StatusCode, readBody(t, resp))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
}

func assertStatus(t *testing.T, client *http.Client, method, url string, body io.Reader, wantStatus int) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, body=%s", method, url, resp.StatusCode, readBody(t, resp))
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func containsUser(users []User, username string) bool {
	for _, user := range users {
		if user.Username == username {
			return true
		}
	}
	return false
}
