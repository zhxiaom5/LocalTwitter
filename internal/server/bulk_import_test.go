package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPreviewBulkImportCreatorsNormalizesTwitterInputs(t *testing.T) {
	store := newTestStore(t)
	directory, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	existing, err := store.ImportCreatorPlaceholder(directory.ID, "https://x.com/alice")
	if err != nil {
		t.Fatal(err)
	}

	items := PreviewBulkImportCreators(t.Context(), store, directory.ID, `
@alice
bob
看看这个 https://twitter.com/cat/status/123?s=20
https://x.com/bob
not a user!
`, nil)

	ready := map[string]BulkImportItem{}
	errors := 0
	for _, item := range items {
		if item.Status == "ready" {
			ready[item.ProfileURL] = item
		} else {
			errors++
		}
	}
	if len(ready) != 3 {
		t.Fatalf("ready items = %#v, want 3 unique profiles", ready)
	}
	if !ready["https://x.com/alice"].Existing || ready["https://x.com/alice"].ExistingCreatorID != existing.ID {
		t.Fatalf("existing alice item = %#v", ready["https://x.com/alice"])
	}
	if ready["https://x.com/bob"].Username != "bob" {
		t.Fatalf("bob item = %#v", ready["https://x.com/bob"])
	}
	if ready["https://x.com/cat"].Username != "cat" {
		t.Fatalf("cat item = %#v", ready["https://x.com/cat"])
	}
	if errors == 0 {
		t.Fatalf("expected at least one invalid input error, got %#v", items)
	}
}

func TestBulkImportCreatorsCreatesPlaceholderAndReusesExisting(t *testing.T) {
	store := newTestStore(t)
	directory, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	creators, err := store.ImportCreatorPlaceholders(directory.ID, []string{"https://x.com/alice", "@bob", "twitter.com/alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(creators) != 2 {
		t.Fatalf("creators len = %d, want 2: %#v", len(creators), creators)
	}
	for _, creator := range creators {
		if creator.OnlineIdentityStatus != "ready" || creator.TwitterUsername == "" || creator.TwitterProfileURL == "" {
			t.Fatalf("creator should be ready placeholder: %#v", creator)
		}
		if strings.HasPrefix(creator.Name, "待更新-") {
			t.Fatalf("creator name should not include pending prefix: %#v", creator)
		}
		if creator.WorkCount != 0 {
			t.Fatalf("placeholder work count = %d, want 0", creator.WorkCount)
		}
	}
	if creators[0].Name != "alice" || creators[1].Name != "bob" {
		t.Fatalf("placeholder names = %q, %q; want account names alice, bob", creators[0].Name, creators[1].Name)
	}
}

func TestBulkImportAPIsAllowAdminRole(t *testing.T) {
	store := newTestStore(t)
	directory, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := mustTestApp(t, store)
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()

	login(t, client, server.URL, "ted", "ted")
	created := createUser(t, client, server.URL, "alice", "alice123")
	if created.ID == 0 {
		t.Fatal("expected created user")
	}
	logout(t, client, server.URL)
	login(t, client, server.URL, "alice", "alice123")

	body, _ := json.Marshal(map[string]any{"directory_id": directory.ID, "text": "@alice"})
	assertStatus(t, client, http.MethodPost, server.URL+"/api/creators/bulk-import/preview", bytes.NewReader(body), http.StatusOK)
	body, _ = json.Marshal(map[string]any{"directory_id": directory.ID, "profile_urls": []string{"https://x.com/alice"}})
	assertStatus(t, client, http.MethodPost, server.URL+"/api/creators/bulk-import", bytes.NewReader(body), http.StatusCreated)
}

func TestBulkImportAPICreatesReadyPlaceholders(t *testing.T) {
	store := newTestStore(t)
	directory, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := mustTestApp(t, store)
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	client := cookieClient()
	login(t, client, server.URL, "ted", "ted")

	body, _ := json.Marshal(map[string]any{"directory_id": directory.ID, "text": "@alice\nhttps://twitter.com/bob/status/1"})
	var preview struct {
		Items []BulkImportItem `json:"items"`
	}
	decodeJSONResponse(t, client, http.MethodPost, server.URL+"/api/creators/bulk-import/preview", bytes.NewReader(body), http.StatusOK, &preview)
	if len(preview.Items) != 2 {
		t.Fatalf("preview items = %#v", preview.Items)
	}

	body, _ = json.Marshal(map[string]any{"directory_id": directory.ID, "profile_urls": []string{"https://x.com/alice", "https://x.com/bob"}})
	var imported struct {
		Creators []Creator `json:"creators"`
	}
	decodeJSONResponse(t, client, http.MethodPost, server.URL+"/api/creators/bulk-import", bytes.NewReader(body), http.StatusCreated, &imported)
	if len(imported.Creators) != 2 {
		t.Fatalf("imported creators = %#v", imported.Creators)
	}
}
