package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestUpdateJobLifecycleMigrationIsAdditive(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE directories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  path TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'idle',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE creators (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  directory_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  work_count INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE update_jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  creator_id INTEGER NOT NULL,
  actor_user_id INTEGER NOT NULL DEFAULT 0,
  cookie_owner_user_id INTEGER NOT NULL DEFAULT 0,
  queue_id INTEGER NOT NULL DEFAULT 0,
  run_id INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',
  added_works INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT ''
);
INSERT INTO directories(id, path, name) VALUES(1, '/tmp/twitter', 'twitter');
INSERT INTO creators(id, directory_id, name, work_count) VALUES(1, 1, 'legacy_user', 0);
INSERT INTO update_jobs(id, creator_id, status) VALUES(1, 1, 'failed');`)
	if err != nil {
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
	second, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("migration should be repeatable: %v", err)
	}
	_ = second.Close()
	job, err := store.UpdateJob(1)
	if err != nil {
		t.Fatal(err)
	}
	if job.Attempt != 1 || job.Source != "manual" || job.RetryOfJobID != 0 {
		t.Fatalf("migrated job lifecycle fields = %#v", job)
	}
}

func TestSaveDirectoryDeduplicatesAndDeleteKeepsFiles(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()
	realFile := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(realFile, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := store.SaveDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.SaveDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected dedupe, got %d and %d", first.ID, second.ID)
	}
	if err := store.DeleteDirectory(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(realFile); err != nil {
		t.Fatalf("delete directory should not delete real files: %v", err)
	}
}

func TestUpsertCreatorWithWorksDeduplicates(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "a.mp4")
	input := []WorkInput{{
		AwemeID:     "1",
		Title:       "title",
		Description: "desc",
		Tags:        []string{"tag"},
		Media:       []MediaInput{{Type: "video", FilePath: mediaPath, FileName: "a.mp4"}},
	}}
	if _, _, err := store.UpsertCreatorWithWorks(dir.ID, "creator", input); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpsertCreatorWithWorks(dir.ID, "creator", input); err != nil {
		t.Fatal(err)
	}
	creators, err := store.Creators()
	if err != nil {
		t.Fatal(err)
	}
	if len(creators) != 1 {
		t.Fatalf("creators len = %d", len(creators))
	}
	works, err := store.Feed(0, 0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 1 || len(works[0].Media) != 1 {
		t.Fatalf("unexpected works: %#v", works)
	}
}

func TestRecordWorkViewAggregatesCreatorViews(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mediaDir := t.TempDir()
	firstCreator, _, err := store.UpsertCreatorWithWorks(dir.ID, "alpha", []WorkInput{
		{AwemeID: "a1", Title: "alpha one", Media: []MediaInput{{Type: "video", FilePath: filepath.Join(mediaDir, "a1.mp4"), FileName: "a1.mp4"}}},
		{AwemeID: "a2", Title: "alpha two", Media: []MediaInput{{Type: "video", FilePath: filepath.Join(mediaDir, "a2.mp4"), FileName: "a2.mp4"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpsertCreatorWithWorks(dir.ID, "beta", []WorkInput{
		{AwemeID: "b1", Title: "beta one", Media: []MediaInput{{Type: "video", FilePath: filepath.Join(mediaDir, "b1.mp4"), FileName: "b1.mp4"}}},
	}); err != nil {
		t.Fatal(err)
	}
	alphaWorks, err := store.Feed(firstCreator.ID, 0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(alphaWorks) != 2 {
		t.Fatalf("alpha works = %#v", alphaWorks)
	}
	if _, err := store.db.Exec(`INSERT INTO users(id, username, password_salt, password_hash, password_iterations, is_admin, role) VALUES(2, 'u2', 'salt', 'hash', 1, 1, 'admin')`); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordWorkView(1, alphaWorks[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordWorkView(1, alphaWorks[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordWorkView(2, alphaWorks[1].ID); err != nil {
		t.Fatal(err)
	}
	creators, err := store.Creators()
	if err != nil {
		t.Fatal(err)
	}
	var alpha, beta Creator
	for _, creator := range creators {
		if creator.Name == "alpha" {
			alpha = creator
		}
		if creator.Name == "beta" {
			beta = creator
		}
	}
	if alpha.ViewCount != 3 {
		t.Fatalf("alpha view count = %d, want 3", alpha.ViewCount)
	}
	if alpha.LastWorkCreatedAt == "" {
		t.Fatal("alpha last work created at should be set")
	}
	if beta.ViewCount != 0 {
		t.Fatalf("beta view count = %d, want 0", beta.ViewCount)
	}
	if err := store.RecordWorkView(1, 999999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing work error = %v, want sql.ErrNoRows", err)
	}
}

func TestCreatorsForUserPageFiltersAndSorts(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mediaDir := t.TempDir()
	alpha, _, err := store.UpsertCreatorWithWorks(dir.ID, "alpha", []WorkInput{
		{AwemeID: "a1", Title: "alpha one", Media: []MediaInput{{Type: "video", FilePath: filepath.Join(mediaDir, "a1.mp4"), FileName: "a1.mp4"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	beta, _, err := store.UpsertCreatorWithWorks(dir.ID, "beta", []WorkInput{
		{AwemeID: "b1", Title: "beta one", Media: []MediaInput{{Type: "video", FilePath: filepath.Join(mediaDir, "b1.mp4"), FileName: "b1.mp4"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	gamma, _, err := store.UpsertCreatorWithWorks(dir.ID, "gamma", []WorkInput{
		{AwemeID: "g1", Title: "gamma one", Media: []MediaInput{{Type: "video", FilePath: filepath.Join(mediaDir, "g1.mp4"), FileName: "g1.mp4"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO creators(directory_id, name, work_count, online_identity_status) VALUES(?, 'empty', 0, 'ready')`, dir.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE works SET created_at=CASE creator_id WHEN ? THEN '2026-05-01T00:00:00Z' WHEN ? THEN '2026-06-01T00:00:00Z' WHEN ? THEN '2026-04-01T00:00:00Z' ELSE created_at END`, alpha.ID, beta.ID, gamma.ID); err != nil {
		t.Fatal(err)
	}
	alphaWorks, err := store.Feed(alpha.ID, 0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	betaWorks, err := store.Feed(beta.ID, 0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := store.RecordWorkView(1, alphaWorks[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RecordWorkView(1, betaWorks[0].ID); err != nil {
		t.Fatal(err)
	}

	creators, total, _, err := store.CreatorsForUserPage(0, "", 1, "updated", "desc", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || creatorNames(creators) != "beta,alpha,gamma" {
		t.Fatalf("updated desc creators = %s total=%d, want beta,alpha,gamma total=3", creatorNames(creators), total)
	}

	creators, _, _, err = store.CreatorsForUserPage(0, "", 0, "updated", "asc", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if creatorNames(creators) != "gamma,alpha,beta,empty" {
		t.Fatalf("updated asc creators = %s, want gamma,alpha,beta,empty", creatorNames(creators))
	}

	creators, _, _, err = store.CreatorsForUserPage(0, "", 0, "views", "desc", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if creatorNames(creators[:3]) != "alpha,beta,empty" {
		t.Fatalf("views desc creators = %s, want alpha,beta,empty first", creatorNames(creators))
	}

	creators, _, _, err = store.CreatorsForUserPage(0, "a", 1, "name", "asc", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if creatorNames(creators) != "alpha,beta,gamma" {
		t.Fatalf("name asc search creators = %s, want alpha,beta,gamma", creatorNames(creators))
	}
}

func creatorNames(creators []Creator) string {
	names := make([]string, 0, len(creators))
	for _, creator := range creators {
		names = append(names, creator.Name)
	}
	return strings.Join(names, ",")
}

func TestEmptyCreatorWorksAreNotStoredOrCounted(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, count, err := store.UpsertCreatorWithWorks(dir.ID, "empty", nil); err != nil {
		t.Fatal(err)
	} else if count != 0 {
		t.Fatalf("empty upsert count = %d", count)
	}
	mediaPath := filepath.Join(t.TempDir(), "a.mp4")
	if _, _, err := store.UpsertCreatorWithWorks(dir.ID, "filled", []WorkInput{{
		AwemeID: "1",
		Title:   "title",
		Media:   []MediaInput{{Type: "video", FilePath: mediaPath, FileName: "a.mp4"}},
	}}); err != nil {
		t.Fatal(err)
	}
	creators, err := store.Creators()
	if err != nil {
		t.Fatal(err)
	}
	if len(creators) != 1 || creators[0].Name != "filled" {
		t.Fatalf("creators should exclude empty creator: %#v", creators)
	}
	counts, err := store.DirectoryCounts(dir.ID)
	if err != nil {
		t.Fatal(err)
	}
	if counts.creators != 1 || counts.works != 1 {
		t.Fatalf("counts = %#v, want one filled creator and one work", counts)
	}
}

func TestFeedSearchMatchesMetadataAndRespectsCreator(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	firstMedia := filepath.Join(t.TempDir(), "first.mp4")
	secondMedia := filepath.Join(t.TempDir(), "second.mp4")
	creatorA, _, err := store.UpsertCreatorWithWorks(dir.ID, "阿言", []WorkInput{{
		AwemeID:     "1001",
		Title:       "夏天训练",
		Description: "健身日常",
		MusicTitle:  "海边音乐",
		SourceURL:   "https://twitter.com/ayan/status/1001",
		Tags:        []string{"夏日"},
		Media:       []MediaInput{{Type: "video", FilePath: firstMedia, FileName: "first.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpsertCreatorWithWorks(dir.ID, "另一位", []WorkInput{{
		AwemeID:     "1002",
		Title:       "夏天旅行",
		Description: "旅游",
		MusicTitle:  "城市音乐",
		Tags:        []string{"旅行"},
		Media:       []MediaInput{{Type: "video", FilePath: secondMedia, FileName: "second.mp4"}},
	}}); err != nil {
		t.Fatal(err)
	}
	cases := []string{"训练", "健身", "阿言", "夏日", "海边", "ayan/status"}
	for _, keyword := range cases {
		works, err := store.Feed(0, 0, keyword, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(works) != 1 || works[0].AwemeID != "1001" {
			t.Fatalf("search %q returned %#v", keyword, works)
		}
	}
	works, err := store.Feed(creatorA.ID, 0, "旅行", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 0 {
		t.Fatalf("creator-scoped search leaked results: %#v", works)
	}
}

func TestCreatorProfileAndLinksAreStoredAndFeedUsesLinkedCreators(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mediaDir := t.TempDir()
	first, _, err := store.UpsertCreatorWithWorks(dir.ID, "oldname", []WorkInput{{
		AwemeID: "1001",
		Title:   "old clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(mediaDir, "old.mp4"), FileName: "old.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.UpsertCreatorWithWorks(dir.ID, "newname", []WorkInput{{
		AwemeID: "1002",
		Title:   "new clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(mediaDir, "new.mp4"), FileName: "new.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	third, _, err := store.UpsertCreatorWithWorks(dir.ID, "newername", []WorkInput{{
		AwemeID: "1003",
		Title:   "newer clip",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(mediaDir, "newer.mp4"), FileName: "newer.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	avatarFile := filepath.Join(t.TempDir(), "avatar.jpg")
	if err := os.WriteFile(avatarFile, []byte("avatar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateCreatorProfile(first.ID, CreatorProfileInput{
		TwitterUserID: "42",
		Username:      "newname",
		ProfileURL:    "https://x.com/newname",
		AvatarURL:     "https://pbs.twimg.com/profile_images/a.jpg",
		AvatarFile:    avatarFile,
		Bio:           "新的签名",
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Creator(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Bio != "新的签名" || updated.AvatarURL == "" || updated.AvatarFile == "" || updated.ProfileUpdatedAt == "" {
		t.Fatalf("creator profile = %#v", updated)
	}
	if err := store.SetCreatorLinks(second.ID, []int64{third.ID}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCreatorLinks(first.ID, []int64{second.ID}); err != nil {
		t.Fatal(err)
	}
	links, err := store.CreatorLinks(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links.Linked) != 2 {
		t.Fatalf("links = %#v", links)
	}
	works, err := store.Feed(first.ID, 0, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 3 {
		t.Fatalf("linked feed works = %#v", works)
	}
	names := works[0].CreatorName + "," + works[1].CreatorName + "," + works[2].CreatorName
	if !strings.Contains(names, "oldname") || !strings.Contains(names, "newname") || !strings.Contains(names, "newername") {
		t.Fatalf("linked feed creator names = %q", names)
	}
	for _, work := range works {
		if work.CreatorID == first.ID && len(work.LinkedCreators) != 2 {
			t.Fatalf("work should expose linked creators: %#v", work)
		}
	}
	if err := store.SetCreatorLinks(first.ID, nil); err != nil {
		t.Fatal(err)
	}
	links, err = store.CreatorLinks(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links.Linked) != 0 {
		t.Fatalf("links after clear = %#v", links)
	}
}

func TestRefreshCreatorProfileStoresBioAndLocalAvatar(t *testing.T) {
	avatarServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("avatar"))
	}))
	t.Cleanup(avatarServer.Close)
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	creator, _, err := store.UpsertCreatorWithWorks(dir.ID, "oldname", []WorkInput{{
		AwemeID: "old",
		Title:   "old",
		Media:   []MediaInput{{Type: "video", FilePath: filepath.Join(t.TempDir(), "old.mp4"), FileName: "old.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	fetcher := fakeProfileFetcher{profile: CreatorProfileInput{
		TwitterUserID: "user-1",
		Username:      "newname",
		ProfileURL:    "https://x.com/newname",
		AvatarURL:     avatarServer.URL + "/avatar.jpg",
		Bio:           "新的签名",
	}}
	if err := RefreshCreatorProfile(context.Background(), store, creator, UpdateAuthSecret{}, fetcher); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Creator(creator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Bio != "新的签名" || updated.TwitterUsername != "newname" || updated.TwitterUserID != "user-1" || updated.AvatarFile == "" || updated.AvatarURL != "/api/creators/"+strconvID(updated.ID)+"/avatar" {
		t.Fatalf("updated creator = %#v", updated)
	}
	data, err := os.ReadFile(updated.AvatarFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "avatar" {
		t.Fatalf("avatar data = %q", data)
	}
}

type fakeProfileFetcher struct {
	profile CreatorProfileInput
	err     error
}

func (f fakeProfileFetcher) FetchCreatorProfile(context.Context, string, UpdateAuthSecret) (CreatorProfileInput, error) {
	return f.profile, f.err
}

func TestTwitterProfileFetcherFallsBackFromRejectedCookie(t *testing.T) {
	calls := make([]bool, 0, 2)
	fetcher := TwitterProfileFetcher{fetch: func(_ context.Context, username string, auth UpdateAuthSecret, useCookie bool) (CreatorProfileInput, error) {
		calls = append(calls, useCookie)
		if useCookie {
			return CreatorProfileInput{}, errors.New("profile request failed: 403")
		}
		if auth.AuthToken != "auth-token" || auth.CT0 != "ct0" {
			t.Fatalf("fallback auth = %#v", auth)
		}
		return CreatorProfileInput{Username: username, Bio: "refreshed"}, nil
	}}

	profile, err := fetcher.FetchCreatorProfile(context.Background(), "creator", UpdateAuthSecret{
		CookieHeader: "auth_token=expired; ct0=expired",
		AuthToken:    "auth-token",
		CT0:          "ct0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []bool{true, false}) || profile.Bio != "refreshed" {
		t.Fatalf("calls=%v profile=%#v", calls, profile)
	}
}

func TestFeedAndMediaForWorkReturnOnlyVideos(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	imageOnly := filepath.Join(t.TempDir(), "only.jpg")
	mixedImage := filepath.Join(t.TempDir(), "cover.jpg")
	mixedVideo := filepath.Join(t.TempDir(), "clip.mp4")
	secondVideo := filepath.Join(t.TempDir(), "clip-2.mp4")
	if _, _, err := store.UpsertCreatorWithWorks(dir.ID, "creator", []WorkInput{
		{
			AwemeID: "image-only",
			Title:   "image only",
			Media:   []MediaInput{{Type: "image", FilePath: imageOnly, FileName: "only.jpg"}},
		},
		{
			AwemeID: "mixed",
			Title:   "mixed",
			Media: []MediaInput{
				{Type: "image", FilePath: mixedImage, FileName: "cover.jpg"},
				{Type: "video", FilePath: mixedVideo, FileName: "clip.mp4"},
				{Type: "video", FilePath: secondVideo, FileName: "clip-2.mp4", Ordinal: 1},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	works, err := store.Feed(0, 0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 1 || works[0].AwemeID != "mixed" {
		t.Fatalf("feed should only include video works: %#v", works)
	}
	if len(works[0].Media) != 2 || works[0].Media[0].Type != "video" || works[0].Media[0].FileName != "clip.mp4" || works[0].Media[1].Type != "video" || works[0].Media[1].FileName != "clip-2.mp4" {
		t.Fatalf("feed media should only include video: %#v", works[0].Media)
	}
	if works[0].CoverURL != "/api/works/"+strconvID(works[0].ID)+"/cover" {
		t.Fatalf("feed should expose local cover url, got %#v", works[0])
	}
	media, err := store.MediaForWork(works[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(media) != 2 || media[0].Type != "video" || media[0].FileName != "clip.mp4" || media[1].Type != "video" || media[1].FileName != "clip-2.mp4" {
		t.Fatalf("media should only include video: %#v", media)
	}
}

func TestHomeFeedRandomizesAcrossCreators(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mediaDir := t.TempDir()
	for creatorIndex, name := range []string{"alpha", "beta", "gamma"} {
		works := make([]WorkInput, 0, 8)
		for workIndex := 0; workIndex < 8; workIndex++ {
			fileName := fmt.Sprintf("%s-%02d.mp4", name, workIndex)
			mediaPath := filepath.Join(mediaDir, fileName)
			if err := os.WriteFile(mediaPath, []byte("video"), 0o644); err != nil {
				t.Fatal(err)
			}
			works = append(works, WorkInput{
				AwemeID: fmt.Sprintf("%d-%02d", creatorIndex, workIndex),
				Title:   fileName,
				Media:   []MediaInput{{Type: "video", FilePath: mediaPath, FileName: fileName}},
			})
		}
		if _, _, err := store.UpsertCreatorWithWorks(dir.ID, name, works); err != nil {
			t.Fatal(err)
		}
	}

	works, err := store.Feed(0, 0, "", 6)
	if err != nil {
		t.Fatal(err)
	}
	seenCreators := map[string]bool{}
	for _, work := range works {
		seenCreators[work.CreatorName] = true
	}
	if len(seenCreators) < 3 {
		t.Fatalf("home feed should cover multiple creators, got %d creators in %#v", len(seenCreators), works)
	}
}

func TestCreatorsForUserPagePaginationAndSorting(t *testing.T) {
	store := newTestStore(t)
	dir, err := store.SaveDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Create 10 creators with various names, work counts, and work timestamps
	for i := 0; i < 10; i++ {
		mediaFile := filepath.Join(t.TempDir(), "test.mp4")
		if err := os.WriteFile(mediaFile, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("博主%02d", i)
		workAweID := fmt.Sprintf("aweme_%02d", i)
		workTitle := fmt.Sprintf("作品_%02d", i)
		store.UpsertCreatorWithWorks(dir.ID, name, []WorkInput{{
			AwemeID: workAweID,
			Title:   workTitle,
			Media:   []MediaInput{{Type: "video", FilePath: mediaFile, FileName: "test.mp4"}},
		}})
	}

	// Test default sort (name asc)
	creators, total, pages, err := store.CreatorsForUserPage(0, "", 0, "name", "asc", 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	if total != 10 {
		t.Fatalf("expected total=10, got %d", total)
	}
	if pages != 2 {
		t.Fatalf("expected pages=2, got %d", pages)
	}
	if len(creators) != 5 {
		t.Fatalf("expected len=5, got %d", len(creators))
	}
	if creators[0].Name != "博主00" || creators[4].Name != "博主04" {
		t.Fatalf("name asc wrong order: %#v", creators)
	}

	// Test page 2
	creators, _, _, err = store.CreatorsForUserPage(0, "", 0, "name", "asc", 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(creators) != 5 {
		t.Fatalf("page2 expected len=5, got %d", len(creators))
	}
	if creators[0].Name != "博主05" {
		t.Fatalf("page2 first should be 博主05, got %s", creators[0].Name)
	}

	// Test name desc
	creators, _, _, err = store.CreatorsForUserPage(0, "", 0, "name", "desc", 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	if creators[0].Name != "博主09" {
		t.Fatalf("name desc first should be 博主09, got %s", creators[0].Name)
	}

	// Test search filter
	creators, total, _, err = store.CreatorsForUserPage(0, "博主0", 0, "name", "asc", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 10 {
		t.Fatalf("search filter total=10, got %d", total)
	}

	creators, total, _, err = store.CreatorsForUserPage(0, "博主01", 0, "name", "asc", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || creators[0].Name != "博主01" {
		t.Fatalf("search 博主01 failed: total=%d, name=%s", total, creators[0].Name)
	}

	// Test page out of range should clamp
	creators, total, pages, err = store.CreatorsForUserPage(0, "", 0, "name", "asc", 99, 5)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || total != 10 {
		t.Fatalf("page 99 clamp: pages=%d total=%d", pages, total)
	}
}

func TestStoreCreatesFeedAndCreatorPerformanceIndexes(t *testing.T) {
	store := newTestStore(t)
	required := []string{
		"works_creator_created_idx",
		"works_directory_created_idx",
		"media_work_type_idx",
		"work_view_history_work_idx",
	}
	for _, name := range required {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("missing performance index %s", name)
		}
	}
}
