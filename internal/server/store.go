package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, path: path}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS directories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  path TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'idle',
  last_scanned_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS creators (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  directory_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  work_count INTEGER NOT NULL DEFAULT 0,
  twitter_user_id TEXT NOT NULL DEFAULT '',
  twitter_username TEXT NOT NULL DEFAULT '',
  twitter_profile_url TEXT NOT NULL DEFAULT '',
  online_identity_status TEXT NOT NULL DEFAULT 'missing',
  avatar_url TEXT NOT NULL DEFAULT '',
  avatar_file TEXT NOT NULL DEFAULT '',
  bio TEXT NOT NULL DEFAULT '',
  profile_updated_at TEXT NOT NULL DEFAULT '',
  UNIQUE(directory_id, name),
  FOREIGN KEY(directory_id) REFERENCES directories(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS creator_link_groups (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS creator_link_members (
  group_id INTEGER NOT NULL,
  creator_id INTEGER NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(group_id) REFERENCES creator_link_groups(id) ON DELETE CASCADE,
  FOREIGN KEY(creator_id) REFERENCES creators(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS creator_link_members_group_idx ON creator_link_members(group_id);
CREATE TABLE IF NOT EXISTS works (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  directory_id INTEGER NOT NULL,
  creator_id INTEGER NOT NULL,
  aweme_id TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  music_title TEXT NOT NULL DEFAULT '',
  source_url TEXT NOT NULL DEFAULT '',
  cover_url TEXT NOT NULL DEFAULT '',
  cover_file TEXT NOT NULL DEFAULT '',
  tags_json TEXT NOT NULL DEFAULT '[]',
  published_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(directory_id, creator_id, aweme_id),
  FOREIGN KEY(directory_id) REFERENCES directories(id) ON DELETE CASCADE,
  FOREIGN KEY(creator_id) REFERENCES creators(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS media (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  work_id INTEGER NOT NULL,
  type TEXT NOT NULL,
  file_path TEXT NOT NULL UNIQUE,
  file_name TEXT NOT NULL,
  ordinal INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY(work_id) REFERENCES works(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS works_creator_created_idx ON works(creator_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS works_directory_created_idx ON works(directory_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS media_work_type_idx ON media(work_id, type);
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_salt TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  password_iterations INTEGER NOT NULL,
  is_admin INTEGER NOT NULL DEFAULT 0,
  role TEXT NOT NULL DEFAULT 'admin',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at TEXT NOT NULL,
  last_used_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);
CREATE TABLE IF NOT EXISTS favorite_folders (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL DEFAULT 0,
  type TEXT NOT NULL CHECK(type IN ('work', 'creator')),
  name TEXT NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS work_favorites (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  folder_id INTEGER NOT NULL,
  work_id INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(folder_id, work_id),
  FOREIGN KEY(folder_id) REFERENCES favorite_folders(id) ON DELETE CASCADE,
  FOREIGN KEY(work_id) REFERENCES works(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS creator_favorites (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  folder_id INTEGER NOT NULL,
  creator_id INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(folder_id, creator_id),
  FOREIGN KEY(folder_id) REFERENCES favorite_folders(id) ON DELETE CASCADE,
  FOREIGN KEY(creator_id) REFERENCES creators(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS update_queues (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  cron TEXT NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  next_run_at TEXT NOT NULL DEFAULT '',
  last_run_summary TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS update_queues_one_default ON update_queues(is_default) WHERE is_default=1;
CREATE TABLE IF NOT EXISTS update_queue_creators (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  queue_id INTEGER NOT NULL,
  creator_id INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(queue_id, creator_id),
  FOREIGN KEY(queue_id) REFERENCES update_queues(id) ON DELETE CASCADE,
  FOREIGN KEY(creator_id) REFERENCES creators(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS update_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  queue_id INTEGER NOT NULL,
  queue_name TEXT NOT NULL,
  actor_user_id INTEGER NOT NULL DEFAULT 0,
  enqueued_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  total_creators INTEGER NOT NULL DEFAULT 0,
  succeeded_creators INTEGER NOT NULL DEFAULT 0,
  failed_creators INTEGER NOT NULL DEFAULT 0,
  added_works INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'queued',
  summary TEXT NOT NULL DEFAULT '',
  FOREIGN KEY(queue_id) REFERENCES update_queues(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS update_jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  creator_id INTEGER NOT NULL,
  actor_user_id INTEGER NOT NULL DEFAULT 0,
  cookie_owner_user_id INTEGER NOT NULL DEFAULT 0,
  queue_id INTEGER NOT NULL DEFAULT 0,
  run_id INTEGER NOT NULL DEFAULT 0,
  retry_of_job_id INTEGER NOT NULL DEFAULT 0,
  attempt INTEGER NOT NULL DEFAULT 1,
  source TEXT NOT NULL DEFAULT 'manual',
  status TEXT NOT NULL DEFAULT 'pending',
  added_works INTEGER NOT NULL DEFAULT 0,
  total_tweets INTEGER NOT NULL DEFAULT 0,
  processed_tweets INTEGER NOT NULL DEFAULT 0,
  downloaded_videos INTEGER NOT NULL DEFAULT 0,
  skipped_tweets INTEGER NOT NULL DEFAULT 0,
  download_speed_bytes_per_second INTEGER NOT NULL DEFAULT 0,
  progress_message TEXT NOT NULL DEFAULT '',
  download_directory TEXT NOT NULL DEFAULT '',
  download_notice TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  error_detail TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  FOREIGN KEY(creator_id) REFERENCES creators(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS update_jobs_status_idx ON update_jobs(status, id);
CREATE TABLE IF NOT EXISTS update_auth_settings (
  id INTEGER PRIMARY KEY CHECK(id=1),
  auth_token TEXT NOT NULL DEFAULT '',
  ct0 TEXT NOT NULL DEFAULT '',
  proxy TEXT NOT NULL DEFAULT '',
  fetch_limit INTEGER NOT NULL DEFAULT 300,
  media_only INTEGER NOT NULL DEFAULT 1,
  include_retweets INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT '',
  validation_status TEXT NOT NULL DEFAULT 'unknown',
  validation_message TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS user_update_auth_settings (
  user_id INTEGER PRIMARY KEY,
  auth_token TEXT NOT NULL DEFAULT '',
  ct0 TEXT NOT NULL DEFAULT '',
  cookie_header TEXT NOT NULL DEFAULT '',
  proxy TEXT NOT NULL DEFAULT '',
  download_mode TEXT NOT NULL DEFAULT 'original',
  download_directory TEXT NOT NULL DEFAULT '',
  downloader_concurrency INTEGER NOT NULL DEFAULT 1,
  fetch_limit INTEGER NOT NULL DEFAULT 300,
  media_only INTEGER NOT NULL DEFAULT 1,
  include_retweets INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT '',
  validation_status TEXT NOT NULL DEFAULT 'unknown',
  validation_message TEXT NOT NULL DEFAULT '',
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS creator_visibility_overrides (
  user_id INTEGER NOT NULL,
  creator_id INTEGER NOT NULL,
  hidden INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(user_id, creator_id),
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY(creator_id) REFERENCES creators(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS work_view_history (
  user_id INTEGER NOT NULL,
  work_id INTEGER NOT NULL,
  position_seconds REAL NOT NULL DEFAULT 0,
  duration_seconds REAL NOT NULL DEFAULT 0,
  view_count INTEGER NOT NULL DEFAULT 0,
  last_viewed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(user_id, work_id),
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY(work_id) REFERENCES works(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS work_view_history_work_idx ON work_view_history(work_id);
CREATE TABLE IF NOT EXISTS audit_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_user_id INTEGER NOT NULL DEFAULT 0,
  event_type TEXT NOT NULL,
  target_type TEXT NOT NULL DEFAULT '',
  target_id INTEGER NOT NULL DEFAULT 0,
  message TEXT NOT NULL DEFAULT '',
  detail_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS system_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_type TEXT NOT NULL,
  severity TEXT NOT NULL DEFAULT 'info',
  message TEXT NOT NULL DEFAULT '',
  detail_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO update_queues(name, cron, is_default, enabled, next_run_at, created_at, updated_at)
SELECT '默认更新队列', '0 3 * * *', 1, 1, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM update_queues);
INSERT INTO update_auth_settings(id, validation_status)
SELECT 1, 'unknown' WHERE NOT EXISTS (SELECT 1 FROM update_auth_settings WHERE id=1);
`)
	if err != nil {
		return err
	}
	for _, column := range []struct {
		name string
		def  string
	}{
		{"twitter_user_id", "TEXT NOT NULL DEFAULT ''"},
		{"twitter_username", "TEXT NOT NULL DEFAULT ''"},
		{"twitter_profile_url", "TEXT NOT NULL DEFAULT ''"},
		{"online_identity_status", "TEXT NOT NULL DEFAULT 'missing'"},
		{"avatar_url", "TEXT NOT NULL DEFAULT ''"},
		{"avatar_file", "TEXT NOT NULL DEFAULT ''"},
		{"bio", "TEXT NOT NULL DEFAULT ''"},
		{"profile_updated_at", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := s.addColumnIfMissing("creators", column.name, column.def); err != nil {
			return err
		}
	}
	if err := s.addColumnIfMissing("works", "source_url", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("works", "cover_url", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("works", "cover_file", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("update_auth_settings", "cookie_header", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("update_auth_settings", "download_mode", "TEXT NOT NULL DEFAULT 'original'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("update_auth_settings", "download_directory", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("update_auth_settings", "downloader_concurrency", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("users", "role", "TEXT NOT NULL DEFAULT 'admin'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("favorite_folders", "user_id", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	for _, column := range []struct {
		name string
		def  string
	}{
		{"created_by_user_id", "INTEGER NOT NULL DEFAULT 0"},
		{"last_updated_by_user_id", "INTEGER NOT NULL DEFAULT 0"},
		{"last_updated_at", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := s.addColumnIfMissing("creators", column.name, column.def); err != nil {
			return err
		}
	}
	for _, table := range []string{"update_jobs", "update_runs"} {
		if err := s.addColumnIfMissing(table, "actor_user_id", "INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	if err := s.addColumnIfMissing("update_jobs", "cookie_owner_user_id", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DROP INDEX IF EXISTS favorite_folders_one_default;
CREATE UNIQUE INDEX IF NOT EXISTS favorite_folders_one_default_by_user ON favorite_folders(user_id, type) WHERE is_default=1;`); err != nil {
		return err
	}
	for _, column := range []struct {
		name string
		def  string
	}{
		{"total_tweets", "INTEGER NOT NULL DEFAULT 0"},
		{"processed_tweets", "INTEGER NOT NULL DEFAULT 0"},
		{"downloaded_videos", "INTEGER NOT NULL DEFAULT 0"},
		{"skipped_tweets", "INTEGER NOT NULL DEFAULT 0"},
		{"download_speed_bytes_per_second", "INTEGER NOT NULL DEFAULT 0"},
		{"progress_message", "TEXT NOT NULL DEFAULT ''"},
		{"download_directory", "TEXT NOT NULL DEFAULT ''"},
		{"download_notice", "TEXT NOT NULL DEFAULT ''"},
		{"error_detail", "TEXT NOT NULL DEFAULT ''"},
		{"retry_of_job_id", "INTEGER NOT NULL DEFAULT 0"},
		{"attempt", "INTEGER NOT NULL DEFAULT 1"},
		{"source", "TEXT NOT NULL DEFAULT 'manual'"},
	} {
		if err := s.addColumnIfMissing("update_jobs", column.name, column.def); err != nil {
			return err
		}
	}
	if err := s.backfillTwitterCreatorIdentity(); err != nil {
		return err
	}
	if err := s.ensureBootstrapAdmin(); err != nil {
		return err
	}
	return s.migrateMultiUserOwnership()
}

func (s *Store) addColumnIfMissing(table, column, definition string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}

func (s *Store) migrateMultiUserOwnership() error {
	superID, err := s.superAdminUserID()
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE users SET role=? WHERE username=?`, userRoleSuperAdmin, defaultAdminUsername); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE users SET role=? WHERE role='' OR role IS NULL`, userRoleAdmin); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE users SET is_admin=CASE WHEN role=? THEN 1 ELSE 0 END`, userRoleSuperAdmin); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE favorite_folders SET user_id=? WHERE user_id=0`, superID); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE creators SET created_by_user_id=? WHERE created_by_user_id=0`, superID); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE creators SET last_updated_by_user_id=? WHERE last_updated_by_user_id=0`, superID); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE update_jobs SET actor_user_id=? WHERE actor_user_id=0`, superID); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE update_jobs SET cookie_owner_user_id=actor_user_id WHERE cookie_owner_user_id=0`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE update_runs SET actor_user_id=? WHERE actor_user_id=0`, superID); err != nil {
		return err
	}
	if err := s.migrateLegacyUpdateAuth(superID); err != nil {
		return err
	}
	users, err := s.Users()
	if err != nil {
		return err
	}
	for _, user := range users {
		if err := s.ensureDefaultFavoriteFoldersForUser(user.ID); err != nil {
			return err
		}
		if err := s.ensureUpdateAuthForUser(user.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateLegacyUpdateAuth(superID int64) error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM user_update_auth_settings WHERE user_id=?`, superID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO user_update_auth_settings(user_id, auth_token, ct0, cookie_header, proxy, download_mode, download_directory, downloader_concurrency, fetch_limit, media_only, include_retweets, updated_at, validation_status, validation_message)
SELECT ?, auth_token, ct0, COALESCE(cookie_header,''), proxy, COALESCE(download_mode,'original'), COALESCE(download_directory,''), COALESCE(downloader_concurrency,1), fetch_limit, media_only, include_retweets, updated_at, validation_status, validation_message
FROM update_auth_settings WHERE id=1`, superID)
	return err
}

func (s *Store) superAdminUserID() (int64, error) {
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM users WHERE username=? LIMIT 1`, defaultAdminUsername).Scan(&id); err == nil {
		return id, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if err := s.db.QueryRow(`SELECT id FROM users WHERE is_admin=1 ORDER BY id LIMIT 1`).Scan(&id); err == nil {
		return id, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return 0, errors.New("super admin user is missing")
}

func (s *Store) ensureDefaultFavoriteFoldersForUser(userID int64) error {
	if userID <= 0 {
		return errors.New("user id is required")
	}
	for _, item := range []struct {
		kind string
		name string
	}{
		{kind: "work", name: "默认作品收藏夹"},
		{kind: "creator", name: "默认作者收藏夹"},
	} {
		if _, err := s.db.Exec(`INSERT INTO favorite_folders(user_id, type, name, is_default, created_at, updated_at)
SELECT ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM favorite_folders WHERE user_id=? AND type=?)`,
			userID, item.kind, item.name, userID, item.kind); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureUpdateAuthForUser(userID int64) error {
	if userID <= 0 {
		return errors.New("user id is required")
	}
	_, err := s.db.Exec(`INSERT INTO user_update_auth_settings(user_id, validation_status)
SELECT ?, 'unknown' WHERE NOT EXISTS (SELECT 1 FROM user_update_auth_settings WHERE user_id=?)`, userID, userID)
	return err
}

func (s *Store) SaveDirectory(path string) (Directory, error) {
	clean := filepath.Clean(path)
	name := filepath.Base(clean)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO directories(path, name, created_at) VALUES(?, ?, ?)
ON CONFLICT(path) DO UPDATE SET name=excluded.name`, clean, name, now)
	if err != nil {
		return Directory{}, err
	}
	return s.DirectoryByPath(clean)
}

func (s *Store) DirectoryByPath(path string) (Directory, error) {
	row := s.db.QueryRow(`SELECT id, path, name, status, last_scanned_at, created_at FROM directories WHERE path=?`, filepath.Clean(path))
	return scanDirectory(row)
}

func (s *Store) Directory(id int64) (Directory, error) {
	row := s.db.QueryRow(`SELECT id, path, name, status, last_scanned_at, created_at FROM directories WHERE id=?`, id)
	return scanDirectory(row)
}

func (s *Store) Directories() ([]Directory, error) {
	rows, err := s.db.Query(`SELECT id, path, name, status, last_scanned_at, created_at FROM directories ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dirs := make([]Directory, 0)
	for rows.Next() {
		dir, err := scanDirectory(rows)
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dir)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range dirs {
		counts, err := s.DirectoryCounts(dirs[i].ID)
		if err != nil {
			return nil, err
		}
		dirs[i].CreatorCount = counts.creators
		dirs[i].WorkCount = counts.works
	}
	return dirs, nil
}

type dirCounts struct {
	creators int64
	works    int64
}

func (s *Store) DirectoryCounts(id int64) (dirCounts, error) {
	var counts dirCounts
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM creators WHERE directory_id=?`, id).Scan(&counts.creators); err != nil {
		return counts, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM works WHERE directory_id=?`, id).Scan(&counts.works); err != nil {
		return counts, err
	}
	return counts, nil
}

func (s *Store) CreatorWorkCount(id int64) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM works WHERE creator_id=?`, id).Scan(&count)
	return count, err
}

func (s *Store) DeleteDirectory(id int64) error {
	_, err := s.db.Exec(`DELETE FROM directories WHERE id=?`, id)
	return err
}

func (s *Store) MarkDirectoryStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE directories SET status=? WHERE id=?`, status, id)
	return err
}

func (s *Store) MarkDirectoryScanned(id int64) error {
	_, err := s.db.Exec(`UPDATE directories SET status='idle', last_scanned_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

type WorkInput struct {
	AwemeID     string
	Title       string
	Description string
	MusicTitle  string
	SourceURL   string
	CoverURL    string
	CoverFile   string
	Tags        []string
	PublishedAt string
	Media       []MediaInput
}

type MediaInput struct {
	Type     string
	FilePath string
	FileName string
	Ordinal  int
}

type CreatorIdentity struct {
	TwitterUserID   string
	TwitterUsername string
	ProfileURL      string
}

type CreatorProfileInput struct {
	TwitterUserID string
	Username      string
	ProfileURL    string
	AvatarURL     string
	AvatarFile    string
	Bio           string
}

type CreatorLinksResult struct {
	Linked     []Creator `json:"linked"`
	Candidates []Creator `json:"candidates"`
}

const creatorSelectColumns = `id, directory_id, name, work_count, twitter_user_id, twitter_username, twitter_profile_url, online_identity_status, avatar_url, avatar_file, bio, profile_updated_at,
COALESCE((SELECT SUM(wvh.view_count) FROM works cw LEFT JOIN work_view_history wvh ON wvh.work_id=cw.id WHERE cw.creator_id=creators.id), 0),
COALESCE((SELECT MAX(cw.created_at) FROM works cw WHERE cw.creator_id=creators.id), '')`

const creatorStatsSelectColumnsForAlias = `COALESCE((SELECT SUM(wvh.view_count) FROM works cw LEFT JOIN work_view_history wvh ON wvh.work_id=cw.id WHERE cw.creator_id=c.id), 0),
COALESCE((SELECT MAX(cw.created_at) FROM works cw WHERE cw.creator_id=c.id), '')`

func (s *Store) UpsertCreatorWithWorks(directoryID int64, name string, works []WorkInput) (Creator, int, error) {
	if len(works) == 0 {
		return Creator{}, 0, nil
	}
	return s.UpsertCreatorWithWorksWithIdentity(directoryID, name, CreatorIdentity{}, works)
}

func (s *Store) UpsertCreatorWithWorksWithIdentity(directoryID int64, name string, identity CreatorIdentity, works []WorkInput) (Creator, int, error) {
	if len(works) == 0 {
		return Creator{}, 0, nil
	}
	identity = enrichCreatorIdentity(name, identity)
	tx, err := s.db.Begin()
	if err != nil {
		return Creator{}, 0, err
	}
	defer tx.Rollback()

	status := "missing"
	if strings.TrimSpace(identity.ProfileURL) != "" || strings.TrimSpace(identity.TwitterUsername) != "" || strings.TrimSpace(identity.TwitterUserID) != "" {
		status = "ready"
	}
	_, err = tx.Exec(`INSERT INTO creators(directory_id, name, twitter_user_id, twitter_username, twitter_profile_url, online_identity_status) VALUES(?, ?, ?, ?, ?, ?)
ON CONFLICT(directory_id, name) DO NOTHING`, directoryID, name, identity.TwitterUserID, identity.TwitterUsername, identity.ProfileURL, status)
	if err != nil {
		return Creator{}, 0, err
	}
	if status == "ready" {
		if _, err := tx.Exec(`UPDATE creators SET
  twitter_user_id=CASE WHEN ?<>'' THEN ? ELSE twitter_user_id END,
  twitter_username=CASE WHEN ?<>'' THEN ? ELSE twitter_username END,
  twitter_profile_url=CASE WHEN ?<>'' THEN ? ELSE twitter_profile_url END,
  online_identity_status='ready'
WHERE directory_id=? AND name=?`, identity.TwitterUserID, identity.TwitterUserID, identity.TwitterUsername, identity.TwitterUsername, identity.ProfileURL, identity.ProfileURL, directoryID, name); err != nil {
			return Creator{}, 0, err
		}
	}
	creator, err := scanCreatorRow(tx.QueryRow(`SELECT `+creatorSelectColumns+` FROM creators WHERE directory_id=? AND name=?`, directoryID, name))
	if err != nil {
		return Creator{}, 0, err
	}

	insertedWorks := 0
	for _, work := range works {
		tagsJSON, _ := json.Marshal(work.Tags)
		coverFile := strings.TrimSpace(work.CoverFile)
		coverURL := strings.TrimSpace(work.CoverURL)
		for _, media := range work.Media {
			if media.Type == "image" && coverFile == "" {
				coverFile = media.FilePath
			}
		}
		_, err = tx.Exec(`INSERT INTO works(directory_id, creator_id, aweme_id, title, description, music_title, source_url, cover_url, cover_file, tags_json, published_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(directory_id, creator_id, aweme_id) DO UPDATE SET
  title=excluded.title,
  description=excluded.description,
  music_title=excluded.music_title,
  source_url=excluded.source_url,
  cover_url=CASE WHEN excluded.cover_url<>'' THEN excluded.cover_url ELSE works.cover_url END,
  cover_file=CASE WHEN excluded.cover_file<>'' THEN excluded.cover_file ELSE works.cover_file END,
  tags_json=excluded.tags_json,
  published_at=excluded.published_at`, directoryID, creator.ID, work.AwemeID, work.Title, work.Description, work.MusicTitle, work.SourceURL, coverURL, coverFile, string(tagsJSON), work.PublishedAt)
		if err != nil {
			return Creator{}, 0, err
		}
		var workID int64
		err = tx.QueryRow(`SELECT id FROM works WHERE directory_id=? AND creator_id=? AND aweme_id=?`, directoryID, creator.ID, work.AwemeID).Scan(&workID)
		if err != nil {
			return Creator{}, 0, err
		}
		if _, err := tx.Exec(`DELETE FROM media WHERE work_id=? AND type<>'video'`, workID); err != nil {
			return Creator{}, 0, err
		}
		for _, media := range work.Media {
			if media.Type != "video" {
				continue
			}
			_, err = tx.Exec(`INSERT INTO media(work_id, type, file_path, file_name, ordinal) VALUES(?, ?, ?, ?, ?)
ON CONFLICT(file_path) DO UPDATE SET work_id=excluded.work_id, type=excluded.type, file_name=excluded.file_name, ordinal=excluded.ordinal`,
				workID, media.Type, media.FilePath, media.FileName, media.Ordinal)
			if err != nil {
				return Creator{}, 0, err
			}
		}
		insertedWorks++
	}
	_, err = tx.Exec(`UPDATE creators SET work_count=(SELECT COUNT(*) FROM works WHERE creator_id=?) WHERE id=?`, creator.ID, creator.ID)
	if err != nil {
		return Creator{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return Creator{}, 0, err
	}
	creator.WorkCount = int64(len(works))
	return creator, insertedWorks, nil
}

func (s *Store) backfillTwitterCreatorIdentity() error {
	rows, err := s.db.Query(`SELECT id, name, twitter_user_id, twitter_username, twitter_profile_url FROM creators`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type update struct {
		id       int64
		username string
		profile  string
	}
	updates := []update{}
	for rows.Next() {
		var id int64
		var name, userID, username, profileURL string
		if err := rows.Scan(&id, &name, &userID, &username, &profileURL); err != nil {
			return err
		}
		enriched := enrichCreatorIdentity(name, CreatorIdentity{TwitterUserID: userID, TwitterUsername: username, ProfileURL: profileURL})
		if strings.TrimSpace(enriched.TwitterUsername) == "" || strings.TrimSpace(enriched.ProfileURL) == "" {
			continue
		}
		updates = append(updates, update{id: id, username: enriched.TwitterUsername, profile: enriched.ProfileURL})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range updates {
		if _, err := s.db.Exec(`UPDATE creators SET twitter_username=?, twitter_profile_url=?, online_identity_status='ready' WHERE id=?`, item.username, item.profile, item.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreatorByOnlineIdentity(directoryID int64, secTwitterUserID string, profileURL string) (Creator, bool, error) {
	secTwitterUserID = strings.TrimSpace(secTwitterUserID)
	profileURL = strings.TrimSpace(profileURL)
	if secTwitterUserID == "" && profileURL == "" {
		return Creator{}, false, nil
	}
	where := []string{}
	args := []any{}
	if secTwitterUserID != "" {
		where = append(where, "twitter_username=?")
		args = append(args, secTwitterUserID)
	}
	if profileURL != "" {
		where = append(where, "twitter_profile_url=?")
		args = append(args, profileURL)
	}
	query := `SELECT ` + creatorSelectColumns + ` FROM creators WHERE ` + strings.Join(where, " OR ") + ` ORDER BY CASE WHEN directory_id=? THEN 0 ELSE 1 END, id LIMIT 1`
	args = append(args, directoryID)
	creator, err := scanCreatorRow(s.db.QueryRow(query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Creator{}, false, nil
	}
	if err != nil {
		return Creator{}, false, err
	}
	return creator, true, nil
}

func (s *Store) ImportCreatorPlaceholder(directoryID int64, profileURL string) (Creator, error) {
	normalized, secTwitterUserID, err := normalizeTwitterProfileURL(context.Background(), profileURL, nil)
	if err != nil {
		return Creator{}, err
	}
	if creator, ok, err := s.CreatorByOnlineIdentity(directoryID, secTwitterUserID, normalized); err != nil || ok {
		return creator, err
	}
	if _, err := s.Directory(directoryID); err != nil {
		return Creator{}, err
	}
	name := secTwitterUserID
	_, err = s.db.Exec(`INSERT INTO creators(directory_id, name, twitter_username, twitter_profile_url, online_identity_status)
VALUES(?, ?, ?, ?, 'ready')
ON CONFLICT(directory_id, name) DO UPDATE SET
  twitter_username=excluded.twitter_username,
  twitter_profile_url=excluded.twitter_profile_url,
  online_identity_status='ready'`, directoryID, name, secTwitterUserID, normalized)
	if err != nil {
		return Creator{}, err
	}
	creator, ok, err := s.CreatorByOnlineIdentity(directoryID, secTwitterUserID, normalized)
	if err != nil {
		return Creator{}, err
	}
	if !ok {
		return Creator{}, sql.ErrNoRows
	}
	return creator, nil
}

func (s *Store) ImportCreatorPlaceholders(directoryID int64, profileURLs []string) ([]Creator, error) {
	seen := map[string]bool{}
	creators := []Creator{}
	for _, raw := range profileURLs {
		normalized, _, err := normalizeTwitterProfileURL(context.Background(), raw, nil)
		if err != nil {
			return nil, err
		}
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		creator, err := s.ImportCreatorPlaceholder(directoryID, normalized)
		if err != nil {
			return nil, err
		}
		creators = append(creators, creator)
	}
	return creators, nil
}
func (s *Store) CreatorsForUserPage(userID int64, search string, minWorkCount int, sortField string, sortDirection string, page, pageSize int) ([]Creator, int, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50
	}
	sortField = strings.TrimSpace(sortField)
	if sortField == "" {
		sortField = "updated"
	}
	dir := strings.ToUpper(strings.TrimSpace(sortDirection))
	if dir != "ASC" && dir != "DESC" {
		dir = "DESC"
	}
	search = strings.TrimSpace(search)

	where := []string{"(c.work_count > 0 OR c.online_identity_status='ready')"}
	args := []any{}

	if userID > 0 {
		where = append(where, "c.id NOT IN (SELECT creator_id FROM creator_visibility_overrides WHERE user_id=? AND hidden=1)")
		args = append(args, userID)
	}
	if search != "" {
		where = append(where, "LOWER(c.name) LIKE ?")
		args = append(args, "%"+strings.ToLower(search)+"%")
	}
	if minWorkCount > 0 {
		where = append(where, "c.work_count >= ?")
		args = append(args, minWorkCount)
	}

	whereClause := strings.Join(where, " AND ")

	viewCountCol := `COALESCE(cs.view_count, 0)`
	lastWorkCol := `COALESCE(cs.last_work_created_at, '')`

	var orderClause string
	switch sortField {
	case "updated":
		orderClause = "ORDER BY (" + lastWorkCol + "='') ASC, " + lastWorkCol + " " + dir + ", c.name COLLATE NOCASE"
	case "views":
		orderClause = "ORDER BY " + viewCountCol + " " + dir + ", c.name COLLATE NOCASE"
	default:
		orderClause = "ORDER BY c.name COLLATE NOCASE " + dir
	}

	columns := "c.id, c.directory_id, c.name, c.work_count, c.twitter_user_id, c.twitter_username, c.twitter_profile_url, c.online_identity_status, c.avatar_url, c.avatar_file, c.bio, c.profile_updated_at, " + viewCountCol + ", " + lastWorkCol
	statsCTE := `WITH creator_stats AS (
  SELECT w.creator_id, COALESCE(SUM(wvh.view_count), 0) AS view_count, MAX(w.created_at) AS last_work_created_at
  FROM works w LEFT JOIN work_view_history wvh ON wvh.work_id=w.id
  GROUP BY w.creator_id
) `
	fromClause := `FROM creators c LEFT JOIN creator_stats cs ON cs.creator_id=c.id`

	var total int
	countArgs := append([]any{}, args...)
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM creators c WHERE `+whereClause, countArgs...).Scan(&total); err != nil {
		return nil, 0, 0, err
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	if page > totalPages && totalPages > 0 {
		page = totalPages
	}

	offset := (page - 1) * pageSize
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, offset)
	rows, err := s.db.Query(statsCTE+`SELECT `+columns+` `+fromClause+` WHERE `+whereClause+` `+orderClause+` LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	creators := make([]Creator, 0)
	for rows.Next() {
		creator, err := scanCreatorRow(rows)
		if err != nil {
			return nil, 0, 0, err
		}
		creators = append(creators, creator)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	for i := range creators {
		linked, err := s.CreatorLinkedSummaries(creators[i].ID)
		if err != nil {
			return nil, 0, 0, err
		}
		creators[i].LinkedCreators = linked
	}

	return creators, total, totalPages, nil
}

func (s *Store) nextPlaceholderCreatorName(directoryID int64, secTwitterUserID string) (string, error) {
	suffix := secTwitterUserID
	if len([]rune(secTwitterUserID)) > 8 {
		runes := []rune(secTwitterUserID)
		suffix = string(runes[len(runes)-8:])
	}
	base := "待更新-" + suffix
	name := base
	for i := 2; ; i++ {
		var existing int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM creators WHERE directory_id=? AND name=?`, directoryID, name).Scan(&existing)
		if err != nil {
			return "", err
		}
		if existing == 0 {
			return name, nil
		}
		name = base + "-" + strconv.Itoa(i)
	}
}

func (s *Store) Creators(search ...string) ([]Creator, error) {
	return s.CreatorsForUser(0, search...)
}

func (s *Store) CreatorsForUser(userID int64, search ...string) ([]Creator, error) {
	query := `SELECT ` + creatorSelectColumns + ` FROM creators WHERE (work_count > 0 OR online_identity_status='ready')`
	var args []any
	if userID > 0 {
		query += ` AND id NOT IN (SELECT creator_id FROM creator_visibility_overrides WHERE user_id=? AND hidden=1)`
		args = append(args, userID)
	}
	if len(search) > 0 && strings.TrimSpace(search[0]) != "" {
		query += ` AND LOWER(name) LIKE ?`
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(search[0]))+"%")
	}
	query += ` ORDER BY name COLLATE NOCASE`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	creators := make([]Creator, 0)
	for rows.Next() {
		creator, err := scanCreatorRow(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		creators = append(creators, creator)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range creators {
		linked, err := s.CreatorLinkedSummaries(creators[i].ID)
		if err != nil {
			return nil, err
		}
		creators[i].LinkedCreators = linked
	}
	return creators, nil
}

func (s *Store) RecordWorkView(userID, workID int64) error {
	if userID <= 0 {
		return errors.New("user id is required")
	}
	if workID <= 0 {
		return errors.New("work id is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM works WHERE id=?`, workID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`INSERT INTO work_view_history(user_id, work_id, view_count, last_viewed_at) VALUES(?, ?, 1, ?)
ON CONFLICT(user_id, work_id) DO UPDATE SET view_count=view_count+1, last_viewed_at=excluded.last_viewed_at`, userID, workID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateCreatorProfile(id int64, input CreatorProfileInput) error {
	input.TwitterUserID = strings.TrimSpace(input.TwitterUserID)
	input.Username = strings.Trim(strings.TrimSpace(input.Username), "@")
	input.ProfileURL = strings.TrimSpace(input.ProfileURL)
	input.AvatarURL = strings.TrimSpace(input.AvatarURL)
	input.AvatarFile = strings.TrimSpace(input.AvatarFile)
	input.Bio = strings.TrimSpace(input.Bio)
	identity := enrichCreatorIdentity(input.Username, CreatorIdentity{
		TwitterUserID:   input.TwitterUserID,
		TwitterUsername: input.Username,
		ProfileURL:      input.ProfileURL,
	})
	status := ""
	if identity.TwitterUserID != "" || identity.TwitterUsername != "" || identity.ProfileURL != "" {
		status = "ready"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(`UPDATE creators SET
  twitter_user_id=CASE WHEN ?<>'' THEN ? ELSE twitter_user_id END,
  twitter_username=CASE WHEN ?<>'' THEN ? ELSE twitter_username END,
  twitter_profile_url=CASE WHEN ?<>'' THEN ? ELSE twitter_profile_url END,
  online_identity_status=CASE WHEN ?<>'' THEN ? ELSE online_identity_status END,
  avatar_url=CASE WHEN ?<>'' THEN ? ELSE avatar_url END,
  avatar_file=CASE WHEN ?<>'' THEN ? ELSE avatar_file END,
  bio=?,
  profile_updated_at=?
WHERE id=?`,
		identity.TwitterUserID, identity.TwitterUserID,
		identity.TwitterUsername, identity.TwitterUsername,
		identity.ProfileURL, identity.ProfileURL,
		status, status,
		input.AvatarURL, input.AvatarURL,
		input.AvatarFile, input.AvatarFile,
		input.Bio,
		now,
		id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CreatorFeedIDs(id int64) ([]int64, error) {
	groupID, ok, err := s.creatorLinkGroupID(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		if _, err := s.Creator(id); err != nil {
			return nil, err
		}
		return []int64{id}, nil
	}
	rows, err := s.db.Query(`SELECT creator_id FROM creator_link_members WHERE group_id=? ORDER BY creator_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var memberID int64
		if err := rows.Scan(&memberID); err != nil {
			return nil, err
		}
		ids = append(ids, memberID)
	}
	if len(ids) == 0 {
		return []int64{id}, nil
	}
	return ids, rows.Err()
}

func (s *Store) CreatorLinkedSummaries(id int64) ([]CreatorSummary, error) {
	groupID, ok, err := s.creatorLinkGroupID(id)
	if err != nil || !ok {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT c.id, c.name, c.twitter_username, c.avatar_url, c.avatar_file
FROM creator_link_members clm JOIN creators c ON c.id=clm.creator_id
WHERE clm.group_id=? AND c.id<>?
ORDER BY c.name COLLATE NOCASE`, groupID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	linked := []CreatorSummary{}
	for rows.Next() {
		summary, err := scanCreatorSummary(rows)
		if err != nil {
			return nil, err
		}
		linked = append(linked, summary)
	}
	return linked, rows.Err()
}

func (s *Store) CreatorLinks(id int64) (CreatorLinksResult, error) {
	if _, err := s.Creator(id); err != nil {
		return CreatorLinksResult{}, err
	}
	linkedIDs := map[int64]bool{}
	linkedSummaries, err := s.CreatorLinkedSummaries(id)
	if err != nil {
		return CreatorLinksResult{}, err
	}
	for _, item := range linkedSummaries {
		linkedIDs[item.ID] = true
	}
	all, err := s.Creators()
	if err != nil {
		return CreatorLinksResult{}, err
	}
	result := CreatorLinksResult{Linked: []Creator{}, Candidates: []Creator{}}
	for _, creator := range all {
		if creator.ID == id {
			continue
		}
		if linkedIDs[creator.ID] {
			result.Linked = append(result.Linked, creator)
		}
		result.Candidates = append(result.Candidates, creator)
	}
	return result, nil
}

func (s *Store) SetCreatorLinks(id int64, linkedIDs []int64) error {
	if _, err := s.Creator(id); err != nil {
		return err
	}
	unique := map[int64]bool{id: true}
	for _, linkedID := range linkedIDs {
		if linkedID > 0 && linkedID != id {
			unique[linkedID] = true
		}
	}
	if len(unique) == 1 {
		return s.unlinkCreator(id)
	}
	memberIDs := make([]int64, 0, len(unique))
	for memberID := range unique {
		memberIDs = append(memberIDs, memberID)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, memberID := range memberIDs {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM creators WHERE id=?`, memberID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return sql.ErrNoRows
		}
	}
	placeholders := make([]string, 0, len(memberIDs))
	args := make([]any, 0, len(memberIDs))
	for _, memberID := range memberIDs {
		placeholders = append(placeholders, "?")
		args = append(args, memberID)
	}
	rows, err := tx.Query(`SELECT DISTINCT group_id FROM creator_link_members WHERE creator_id IN (`+strings.Join(placeholders, ",")+`) ORDER BY group_id`, args...)
	if err != nil {
		return err
	}
	groupIDs := []int64{}
	for rows.Next() {
		var groupID int64
		if err := rows.Scan(&groupID); err != nil {
			rows.Close()
			return err
		}
		groupIDs = append(groupIDs, groupID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(groupIDs) > 0 {
		groupArgs := make([]any, 0, len(groupIDs))
		groupPlaceholders := make([]string, 0, len(groupIDs))
		for _, oldGroupID := range groupIDs {
			groupArgs = append(groupArgs, oldGroupID)
			groupPlaceholders = append(groupPlaceholders, "?")
		}
		memberRows, err := tx.Query(`SELECT creator_id FROM creator_link_members WHERE group_id IN (`+strings.Join(groupPlaceholders, ",")+`)`, groupArgs...)
		if err != nil {
			return err
		}
		for memberRows.Next() {
			var memberID int64
			if err := memberRows.Scan(&memberID); err != nil {
				memberRows.Close()
				return err
			}
			unique[memberID] = true
		}
		if err := memberRows.Close(); err != nil {
			return err
		}
		memberIDs = memberIDs[:0]
		for memberID := range unique {
			memberIDs = append(memberIDs, memberID)
		}
	}
	var groupID int64
	if len(groupIDs) > 0 {
		groupID = groupIDs[0]
	} else {
		result, err := tx.Exec(`INSERT INTO creator_link_groups(created_at) VALUES(?)`, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return err
		}
		groupID, err = result.LastInsertId()
		if err != nil {
			return err
		}
	}
	if len(groupIDs) > 0 {
		groupArgs := make([]any, 0, len(groupIDs))
		groupPlaceholders := make([]string, 0, len(groupIDs))
		for _, oldGroupID := range groupIDs {
			groupArgs = append(groupArgs, oldGroupID)
			groupPlaceholders = append(groupPlaceholders, "?")
		}
		if _, err := tx.Exec(`DELETE FROM creator_link_members WHERE group_id IN (`+strings.Join(groupPlaceholders, ",")+`)`, groupArgs...); err != nil {
			return err
		}
		for _, oldGroupID := range groupIDs[1:] {
			if _, err := tx.Exec(`DELETE FROM creator_link_groups WHERE id=?`, oldGroupID); err != nil {
				return err
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, memberID := range memberIDs {
		if _, err := tx.Exec(`INSERT INTO creator_link_members(group_id, creator_id, created_at) VALUES(?, ?, ?)
ON CONFLICT(creator_id) DO UPDATE SET group_id=excluded.group_id`, groupID, memberID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) creatorLinkGroupID(id int64) (int64, bool, error) {
	var groupID int64
	err := s.db.QueryRow(`SELECT group_id FROM creator_link_members WHERE creator_id=?`, id).Scan(&groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return groupID, true, nil
}

func (s *Store) unlinkCreator(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var groupID int64
	err = tx.QueryRow(`SELECT group_id FROM creator_link_members WHERE creator_id=?`, id).Scan(&groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM creator_link_members WHERE creator_id=?`, id); err != nil {
		return err
	}
	var remaining int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM creator_link_members WHERE group_id=?`, groupID).Scan(&remaining); err != nil {
		return err
	}
	if remaining < 2 {
		if _, err := tx.Exec(`DELETE FROM creator_link_members WHERE group_id=?`, groupID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM creator_link_groups WHERE id=?`, groupID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type FeedOptions struct {
	CreatorID        int64
	UserID           int64
	DirectoryID      int64
	FavoriteFolderID int64
	Search           string
	Limit            int
}

func (s *Store) Feed(creatorID int64, directoryID int64, search string, limit int) ([]Work, error) {
	return s.FeedWithOptions(FeedOptions{CreatorID: creatorID, DirectoryID: directoryID, Search: search, Limit: limit})
}

func (s *Store) FeedWithOptions(options FeedOptions) ([]Work, error) {
	limit := options.Limit
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	selectClause := `SELECT w.id, w.directory_id, w.creator_id, c.name, c.avatar_url, c.avatar_file, c.bio, w.aweme_id, w.title, w.description, w.music_title, w.source_url, w.cover_url, w.cover_file, w.tags_json, w.published_at, w.created_at`
	fromClause := `
	FROM works w JOIN creators c ON c.id=w.creator_id`
	query := selectClause + fromClause
	var args []any
	where := []string{`EXISTS (SELECT 1 FROM media m WHERE m.work_id=w.id AND m.type='video')`}
	if options.UserID > 0 {
		where = append(where, `w.creator_id NOT IN (SELECT creator_id FROM creator_visibility_overrides WHERE user_id=? AND hidden=1)`)
		args = append(args, options.UserID)
	}
	if options.FavoriteFolderID > 0 {
		query += ` JOIN work_favorites wf ON wf.work_id=w.id`
		where = append(where, "wf.folder_id=?")
		args = append(args, options.FavoriteFolderID)
	}
	if options.CreatorID > 0 {
		creatorIDs, err := s.CreatorFeedIDs(options.CreatorID)
		if err != nil {
			return nil, err
		}
		placeholders := make([]string, 0, len(creatorIDs))
		for _, id := range creatorIDs {
			placeholders = append(placeholders, "?")
			args = append(args, id)
		}
		where = append(where, "w.creator_id IN ("+strings.Join(placeholders, ",")+")")
	}
	if options.DirectoryID > 0 {
		where = append(where, "w.directory_id=?")
		args = append(args, options.DirectoryID)
	}
	if strings.TrimSpace(options.Search) != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(options.Search)) + "%"
		where = append(where, `(LOWER(w.title) LIKE ? OR LOWER(w.description) LIKE ? OR LOWER(c.name) LIKE ? OR LOWER(w.tags_json) LIKE ? OR LOWER(w.music_title) LIKE ? OR LOWER(w.source_url) LIKE ?)`)
		args = append(args, like, like, like, like, like, like)
	}
	whereClause := strings.Join(where, " AND ")
	query += " WHERE " + whereClause
	if options.FavoriteFolderID > 0 {
		query += ` ORDER BY wf.created_at DESC, w.id DESC LIMIT ?`
		args = append(args, limit)
		rows, err := s.db.Query(query, args...)
		if err != nil {
			return nil, err
		}
		return s.scanFeedRows(rows)
	}
	if strings.TrimSpace(options.Search) != "" {
		query += ` ORDER BY RANDOM() LIMIT ?`
		args = append(args, limit)
		rows, err := s.db.Query(query, args...)
		if err != nil {
			return nil, err
		}
		return s.scanFeedRows(rows)
	}

	works, err := s.feedFromRandomCreators(selectClause, fromClause, whereClause, args, limit)
	if err != nil {
		return nil, err
	}
	return s.enrichWorks(works)
}

func (s *Store) feedFromRandomCreators(selectClause string, fromClause string, whereClause string, args []any, limit int) ([]Work, error) {
	creatorIDs, err := s.randomFeedCreatorIDs(fromClause, whereClause, args, limit*3)
	if err != nil {
		return nil, err
	}
	if len(creatorIDs) == 0 {
		return []Work{}, nil
	}
	works := make([]Work, 0, limit)
	seen := make(map[int64]bool, limit)
	maxAttempts := limit * 8
	if maxAttempts < len(creatorIDs)*2 {
		maxAttempts = len(creatorIDs) * 2
	}
	for attempts := 0; len(works) < limit && attempts < maxAttempts; attempts++ {
		creatorID := creatorIDs[attempts%len(creatorIDs)]
		work, ok, err := s.randomFeedWorkForCreator(selectClause, fromClause, whereClause, args, creatorID)
		if err != nil {
			return nil, err
		}
		if !ok || seen[work.ID] {
			continue
		}
		seen[work.ID] = true
		works = append(works, work)
	}
	if len(works) >= limit {
		return works, nil
	}
	fallback, err := s.feedFromSequentialAnchor(selectClause, fromClause, whereClause, args, limit-len(works), seen)
	if err != nil {
		return nil, err
	}
	works = append(works, fallback...)
	return works, nil
}

func (s *Store) randomFeedCreatorIDs(fromClause string, whereClause string, args []any, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 30
	}
	queryArgs := append(append([]any{}, args...), limit)
	rows, err := s.db.Query(`SELECT w.creator_id`+fromClause+` WHERE `+whereClause+` GROUP BY w.creator_id ORDER BY RANDOM() LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) randomFeedWorkForCreator(selectClause string, fromClause string, whereClause string, args []any, creatorID int64) (Work, bool, error) {
	creatorArgs := append(append([]any{}, args...), creatorID)
	creatorWhere := whereClause + ` AND w.creator_id=?`
	var minID, maxID int64
	if err := s.db.QueryRow(`SELECT COALESCE(MIN(w.id), 0), COALESCE(MAX(w.id), 0)`+fromClause+` WHERE `+creatorWhere, creatorArgs...).Scan(&minID, &maxID); err != nil {
		return Work{}, false, err
	}
	if maxID <= 0 || minID <= 0 {
		return Work{}, false, nil
	}
	anchor := minID
	if maxID > minID {
		anchor = minID + rand.Int63n(maxID-minID+1)
	}
	work, ok, err := s.firstFeedWork(selectClause, fromClause, creatorWhere+` AND w.id >= ?`, append(append([]any{}, creatorArgs...), anchor), `ORDER BY w.id`)
	if err != nil {
		return Work{}, false, err
	}
	if ok {
		return work, true, nil
	}
	return s.firstFeedWork(selectClause, fromClause, creatorWhere+` AND w.id < ?`, append(append([]any{}, creatorArgs...), anchor), `ORDER BY w.id`)
}

func (s *Store) firstFeedWork(selectClause string, fromClause string, whereClause string, args []any, orderClause string) (Work, bool, error) {
	queryArgs := append(append([]any{}, args...), 1)
	rows, err := s.db.Query(selectClause+fromClause+` WHERE `+whereClause+` `+orderClause+` LIMIT ?`, queryArgs...)
	if err != nil {
		return Work{}, false, err
	}
	works, err := scanFeedRowsOnly(rows)
	if err != nil {
		return Work{}, false, err
	}
	if len(works) == 0 {
		return Work{}, false, nil
	}
	return works[0], true, nil
}

func (s *Store) feedFromSequentialAnchor(selectClause string, fromClause string, whereClause string, args []any, limit int, seen map[int64]bool) ([]Work, error) {
	var maxID int64
	maxQuery := `SELECT COALESCE(MAX(w.id), 0)` + fromClause + ` WHERE ` + whereClause
	if err := s.db.QueryRow(maxQuery, args...).Scan(&maxID); err != nil {
		return nil, err
	}
	if maxID <= 0 {
		return []Work{}, nil
	}
	anchor := rand.Int63n(maxID) + 1
	collected := make([]Work, 0, limit)
	for _, part := range []struct {
		op    string
		order string
	}{
		{op: ">=", order: "ASC"},
		{op: "<", order: "ASC"},
	} {
		if len(collected) >= limit {
			break
		}
		queryArgs := append(append([]any{}, args...), anchor, limit-len(collected))
		query := selectClause + fromClause + ` WHERE ` + whereClause + ` AND w.id ` + part.op + ` ? ORDER BY w.id ` + part.order + ` LIMIT ?`
		rows, err := s.db.Query(query, queryArgs...)
		if err != nil {
			return nil, err
		}
		works, err := scanFeedRowsOnly(rows)
		if err != nil {
			return nil, err
		}
		for _, work := range works {
			if !seen[work.ID] {
				seen[work.ID] = true
				collected = append(collected, work)
			}
		}
	}
	return collected, nil
}

func (s *Store) scanFeedRows(rows *sql.Rows) ([]Work, error) {
	works, err := scanFeedRowsOnly(rows)
	if err != nil {
		return nil, err
	}
	return s.enrichWorks(works)
}

func scanFeedRowsOnly(rows *sql.Rows) ([]Work, error) {
	defer rows.Close()
	works := make([]Work, 0)
	for rows.Next() {
		work, err := scanWork(rows)
		if err != nil {
			return nil, err
		}
		works = append(works, work)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return works, nil
}

func (s *Store) enrichWorks(works []Work) ([]Work, error) {
	for i := range works {
		media, err := s.MediaForWork(works[i].ID)
		if err != nil {
			return nil, err
		}
		works[i].Media = media
		linked, err := s.CreatorLinkedSummaries(works[i].CreatorID)
		if err != nil {
			return nil, err
		}
		works[i].LinkedCreators = linked
	}
	return works, nil
}

func (s *Store) FavoriteFolders(kind string) ([]FavoriteFolder, error) {
	userID, err := s.superAdminUserID()
	if err != nil {
		return nil, err
	}
	return s.FavoriteFoldersForUser(userID, kind)
}

func (s *Store) FavoriteFoldersForUser(userID int64, kind string) ([]FavoriteFolder, error) {
	if kind != "work" && kind != "creator" {
		return nil, errors.New("invalid favorite folder type")
	}
	if err := s.ensureDefaultFavoriteFoldersForUser(userID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, user_id, type, name, is_default, created_at, updated_at FROM favorite_folders WHERE user_id=? AND type=? ORDER BY is_default DESC, created_at, id`, userID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	folders := make([]FavoriteFolder, 0)
	for rows.Next() {
		folder, err := scanFavoriteFolder(rows)
		if err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}
	return folders, rows.Err()
}

func (s *Store) CreateFavoriteFolder(kind, name string) (FavoriteFolder, error) {
	userID, err := s.superAdminUserID()
	if err != nil {
		return FavoriteFolder{}, err
	}
	return s.CreateFavoriteFolderForUser(userID, kind, name)
}

func (s *Store) CreateFavoriteFolderForUser(userID int64, kind, name string) (FavoriteFolder, error) {
	if kind != "work" && kind != "creator" {
		return FavoriteFolder{}, errors.New("invalid favorite folder type")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return FavoriteFolder{}, errors.New("name is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(`INSERT INTO favorite_folders(user_id, type, name, is_default, created_at, updated_at) VALUES(?, ?, ?, 0, ?, ?)`, userID, kind, name, now, now)
	if err != nil {
		return FavoriteFolder{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return FavoriteFolder{}, err
	}
	return s.FavoriteFolder(id)
}

func (s *Store) FavoriteFolder(id int64) (FavoriteFolder, error) {
	row := s.db.QueryRow(`SELECT id, user_id, type, name, is_default, created_at, updated_at FROM favorite_folders WHERE id=?`, id)
	return scanFavoriteFolder(row)
}

func (s *Store) UpdateFavoriteFolder(id int64, name string, isDefault *bool) (FavoriteFolder, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return FavoriteFolder{}, err
	}
	defer tx.Rollback()
	var kind string
	if err := tx.QueryRow(`SELECT type FROM favorite_folders WHERE id=?`, id).Scan(&kind); err != nil {
		return FavoriteFolder{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(name) != "" {
		if _, err := tx.Exec(`UPDATE favorite_folders SET name=?, updated_at=? WHERE id=?`, strings.TrimSpace(name), now, id); err != nil {
			return FavoriteFolder{}, err
		}
	}
	if isDefault != nil && *isDefault {
		var userID int64
		if err := tx.QueryRow(`SELECT user_id FROM favorite_folders WHERE id=?`, id).Scan(&userID); err != nil {
			return FavoriteFolder{}, err
		}
		if _, err := tx.Exec(`UPDATE favorite_folders SET is_default=0, updated_at=? WHERE user_id=? AND type=?`, now, userID, kind); err != nil {
			return FavoriteFolder{}, err
		}
		if _, err := tx.Exec(`UPDATE favorite_folders SET is_default=1, updated_at=? WHERE id=?`, now, id); err != nil {
			return FavoriteFolder{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return FavoriteFolder{}, err
	}
	return s.FavoriteFolder(id)
}

func (s *Store) AddWorkFavorite(workID, folderID int64) error {
	userID, err := s.superAdminUserID()
	if err != nil {
		return err
	}
	return s.AddWorkFavoriteForUser(userID, workID, folderID)
}

func (s *Store) AddWorkFavoriteForUser(userID, workID, folderID int64) error {
	id, err := s.resolveFavoriteFolderIDForUser(userID, "work", folderID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO work_favorites(folder_id, work_id) VALUES(?, ?) ON CONFLICT(folder_id, work_id) DO NOTHING`, id, workID)
	return err
}

func (s *Store) RemoveWorkFavorite(workID, folderID int64) error {
	userID, err := s.superAdminUserID()
	if err != nil {
		return err
	}
	return s.RemoveWorkFavoriteForUser(userID, workID, folderID)
}

func (s *Store) RemoveWorkFavoriteForUser(userID, workID, folderID int64) error {
	id, err := s.resolveFavoriteFolderIDForUser(userID, "work", folderID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM work_favorites WHERE folder_id=? AND work_id=?`, id, workID)
	return err
}

func (s *Store) AddCreatorFavorite(creatorID, folderID int64) error {
	userID, err := s.superAdminUserID()
	if err != nil {
		return err
	}
	return s.AddCreatorFavoriteForUser(userID, creatorID, folderID)
}

func (s *Store) AddCreatorFavoriteForUser(userID, creatorID, folderID int64) error {
	id, err := s.resolveFavoriteFolderIDForUser(userID, "creator", folderID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO creator_favorites(folder_id, creator_id) VALUES(?, ?) ON CONFLICT(folder_id, creator_id) DO NOTHING`, id, creatorID)
	return err
}

func (s *Store) RemoveCreatorFavorite(creatorID, folderID int64) error {
	userID, err := s.superAdminUserID()
	if err != nil {
		return err
	}
	return s.RemoveCreatorFavoriteForUser(userID, creatorID, folderID)
}

func (s *Store) RemoveCreatorFavoriteForUser(userID, creatorID, folderID int64) error {
	id, err := s.resolveFavoriteFolderIDForUser(userID, "creator", folderID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM creator_favorites WHERE folder_id=? AND creator_id=?`, id, creatorID)
	return err
}

func (s *Store) FavoriteCreators(folderID int64) ([]Creator, error) {
	userID, err := s.superAdminUserID()
	if err != nil {
		return nil, err
	}
	return s.FavoriteCreatorsForUser(userID, folderID)
}

func (s *Store) FavoriteCreatorsForUser(userID, folderID int64) ([]Creator, error) {
	if folderID <= 0 {
		folderID, err := s.resolveFavoriteFolderIDForUser(userID, "creator", 0)
		if err != nil {
			return nil, err
		}
		return s.FavoriteCreatorsForUser(userID, folderID)
	}
	rows, err := s.db.Query(`SELECT c.id, c.directory_id, c.name, c.work_count, c.twitter_user_id, c.twitter_username, c.twitter_profile_url, c.online_identity_status, c.avatar_url, c.avatar_file, c.bio, c.profile_updated_at, `+creatorStatsSelectColumnsForAlias+`
FROM creator_favorites cf JOIN creators c ON c.id=cf.creator_id
JOIN favorite_folders ff ON ff.id=cf.folder_id
WHERE cf.folder_id=? AND ff.user_id=? ORDER BY cf.created_at DESC, c.name COLLATE NOCASE`, folderID, userID)
	if err != nil {
		return nil, err
	}
	creators := make([]Creator, 0)
	for rows.Next() {
		creator, err := scanCreatorRow(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		creators = append(creators, creator)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range creators {
		linked, err := s.CreatorLinkedSummaries(creators[i].ID)
		if err != nil {
			return nil, err
		}
		creators[i].LinkedCreators = linked
	}
	return creators, nil
}

func (s *Store) FavoriteWorks(folderID int64) ([]Work, error) {
	userID, err := s.superAdminUserID()
	if err != nil {
		return nil, err
	}
	return s.FavoriteWorksForUser(userID, folderID)
}

func (s *Store) FavoriteWorksForUser(userID, folderID int64) ([]Work, error) {
	resolved, err := s.resolveFavoriteFolderIDForUser(userID, "work", folderID)
	if err != nil {
		return nil, err
	}
	folderID = resolved
	return s.FeedWithOptions(FeedOptions{UserID: userID, FavoriteFolderID: folderID, Limit: 100})
}

func (s *Store) FavoriteStatus(workID, creatorID int64) (FavoriteStatus, error) {
	userID, err := s.superAdminUserID()
	if err != nil {
		return FavoriteStatus{}, err
	}
	return s.FavoriteStatusForUser(userID, workID, creatorID)
}

func (s *Store) FavoriteStatusForUser(userID, workID, creatorID int64) (FavoriteStatus, error) {
	var status FavoriteStatus
	if workID > 0 {
		ids, err := s.favoriteFolderIDsForUser(`SELECT wf.folder_id FROM work_favorites wf JOIN favorite_folders ff ON ff.id=wf.folder_id WHERE wf.work_id=? AND ff.user_id=? ORDER BY wf.folder_id`, workID, userID)
		if err != nil {
			return status, err
		}
		status.WorkFolderIDs = ids
		status.WorkFavorited = len(ids) > 0
	}
	if creatorID > 0 {
		ids, err := s.favoriteFolderIDsForUser(`SELECT cf.folder_id FROM creator_favorites cf JOIN favorite_folders ff ON ff.id=cf.folder_id WHERE cf.creator_id=? AND ff.user_id=? ORDER BY cf.folder_id`, creatorID, userID)
		if err != nil {
			return status, err
		}
		status.CreatorFolderIDs = ids
		status.CreatorFavorited = len(ids) > 0
	}
	return status, nil
}

func (s *Store) favoriteFolderIDs(query string, id int64) ([]int64, error) {
	rows, err := s.db.Query(query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var folderID int64
		if err := rows.Scan(&folderID); err != nil {
			return nil, err
		}
		ids = append(ids, folderID)
	}
	return ids, rows.Err()
}

func (s *Store) favoriteFolderIDsForUser(query string, id, userID int64) ([]int64, error) {
	rows, err := s.db.Query(query, id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var folderID int64
		if err := rows.Scan(&folderID); err != nil {
			return nil, err
		}
		ids = append(ids, folderID)
	}
	return ids, rows.Err()
}

func (s *Store) resolveFavoriteFolderID(kind string, folderID int64) (int64, error) {
	userID, err := s.superAdminUserID()
	if err != nil {
		return 0, err
	}
	return s.resolveFavoriteFolderIDForUser(userID, kind, folderID)
}

func (s *Store) resolveFavoriteFolderIDForUser(userID int64, kind string, folderID int64) (int64, error) {
	if folderID > 0 {
		var actualKind string
		var ownerID int64
		if err := s.db.QueryRow(`SELECT type, user_id FROM favorite_folders WHERE id=?`, folderID).Scan(&actualKind, &ownerID); err != nil {
			return 0, err
		}
		if actualKind != kind {
			return 0, errors.New("favorite folder type mismatch")
		}
		if ownerID != userID {
			return 0, errors.New("favorite folder belongs to another user")
		}
		return folderID, nil
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM favorite_folders WHERE user_id=? AND type=? AND is_default=1`, userID, kind).Scan(&id)
	return id, err
}

func (s *Store) Work(id int64) (Work, error) {
	row := s.db.QueryRow(`SELECT w.id, w.directory_id, w.creator_id, c.name, c.avatar_url, c.avatar_file, c.bio, w.aweme_id, w.title, w.description, w.music_title, w.source_url, w.cover_url, w.cover_file, w.tags_json, w.published_at, w.created_at
FROM works w JOIN creators c ON c.id=w.creator_id WHERE w.id=?`, id)
	work, err := scanWork(row)
	if err != nil {
		return Work{}, err
	}
	work.Media, err = s.MediaForWork(work.ID)
	if err != nil {
		return work, err
	}
	work.LinkedCreators, err = s.CreatorLinkedSummaries(work.CreatorID)
	return work, err
}

func (s *Store) MediaForWork(workID int64) ([]Media, error) {
	rows, err := s.db.Query(`SELECT id, work_id, type, file_name, ordinal FROM media WHERE work_id=? AND type='video' ORDER BY ordinal, id`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	media := make([]Media, 0)
	for rows.Next() {
		var item Media
		if err := rows.Scan(&item.ID, &item.WorkID, &item.Type, &item.FileName, &item.Ordinal); err != nil {
			return nil, err
		}
		item.URL = "/api/media/" + strconv.FormatInt(item.ID, 10)
		media = append(media, item)
	}
	return media, rows.Err()
}

func (s *Store) MediaPath(id int64) (path string, name string, mediaType string, err error) {
	err = s.db.QueryRow(`SELECT file_path, file_name, type FROM media WHERE id=?`, id).Scan(&path, &name, &mediaType)
	return path, name, mediaType, err
}

func (s *Store) WorkCoverPath(id int64) (path string, name string, err error) {
	err = s.db.QueryRow(`SELECT cover_file FROM works WHERE id=?`, id).Scan(&path)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", "", sql.ErrNoRows
	}
	return path, filepath.Base(path), nil
}

func (s *Store) Creator(id int64) (Creator, error) {
	creator, err := scanCreatorRow(s.db.QueryRow(`SELECT `+creatorSelectColumns+` FROM creators WHERE id=?`, id))
	if err != nil {
		return Creator{}, err
	}
	creator.LinkedCreators, err = s.CreatorLinkedSummaries(id)
	return creator, err
}

func (s *Store) CreateUpdateQueue(name, cron string) (UpdateQueue, error) {
	name = strings.TrimSpace(name)
	cron = strings.TrimSpace(cron)
	if name == "" {
		return UpdateQueue{}, errors.New("name is required")
	}
	if !ValidCron5(cron) {
		return UpdateQueue{}, errors.New("invalid cron")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(`INSERT INTO update_queues(name, cron, is_default, enabled, next_run_at, created_at, updated_at) VALUES(?, ?, 0, 1, ?, ?, ?)`, name, cron, NextCronRun(cron, time.Now()).Format(time.RFC3339), now, now)
	if err != nil {
		return UpdateQueue{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return UpdateQueue{}, err
	}
	return s.UpdateQueue(id)
}

func (s *Store) UpdateQueues() ([]UpdateQueue, error) {
	rows, err := s.db.Query(`SELECT q.id, q.name, q.cron, q.is_default, q.enabled, q.next_run_at, q.last_run_summary, q.created_at, q.updated_at, COUNT(uqc.creator_id)
FROM update_queues q LEFT JOIN update_queue_creators uqc ON uqc.queue_id=q.id
GROUP BY q.id ORDER BY q.is_default DESC, q.created_at, q.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	queues := make([]UpdateQueue, 0)
	for rows.Next() {
		queue, err := scanUpdateQueue(rows)
		if err != nil {
			return nil, err
		}
		queues = append(queues, queue)
	}
	return queues, rows.Err()
}

func (s *Store) UpdateQueue(id int64) (UpdateQueue, error) {
	row := s.db.QueryRow(`SELECT q.id, q.name, q.cron, q.is_default, q.enabled, q.next_run_at, q.last_run_summary, q.created_at, q.updated_at, COUNT(uqc.creator_id)
FROM update_queues q LEFT JOIN update_queue_creators uqc ON uqc.queue_id=q.id WHERE q.id=? GROUP BY q.id`, id)
	return scanUpdateQueue(row)
}

func (s *Store) UpdateUpdateQueue(id int64, name, cron string, isDefault, enabled *bool) (UpdateQueue, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return UpdateQueue{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(name) != "" {
		if _, err := tx.Exec(`UPDATE update_queues SET name=?, updated_at=? WHERE id=?`, strings.TrimSpace(name), now, id); err != nil {
			return UpdateQueue{}, err
		}
	}
	if strings.TrimSpace(cron) != "" {
		if !ValidCron5(cron) {
			return UpdateQueue{}, errors.New("invalid cron")
		}
		if _, err := tx.Exec(`UPDATE update_queues SET cron=?, next_run_at=?, updated_at=? WHERE id=?`, strings.TrimSpace(cron), NextCronRun(cron, time.Now()).Format(time.RFC3339), now, id); err != nil {
			return UpdateQueue{}, err
		}
	}
	if enabled != nil {
		value := 0
		if *enabled {
			value = 1
		}
		if _, err := tx.Exec(`UPDATE update_queues SET enabled=?, updated_at=? WHERE id=?`, value, now, id); err != nil {
			return UpdateQueue{}, err
		}
	}
	if isDefault != nil && *isDefault {
		if _, err := tx.Exec(`UPDATE update_queues SET is_default=0, updated_at=?`, now); err != nil {
			return UpdateQueue{}, err
		}
		if _, err := tx.Exec(`UPDATE update_queues SET is_default=1, updated_at=? WHERE id=?`, now, id); err != nil {
			return UpdateQueue{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return UpdateQueue{}, err
	}
	return s.UpdateQueue(id)
}

func (s *Store) DeleteUpdateQueue(id int64) error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM update_queues`).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return errors.New("cannot delete the last update queue")
	}
	_, err := s.db.Exec(`DELETE FROM update_queues WHERE id=?`, id)
	return err
}

func (s *Store) AddCreatorsToUpdateQueue(queueID int64, creatorIDs []int64) error {
	for _, id := range creatorIDs {
		if id <= 0 {
			continue
		}
		if _, err := s.db.Exec(`INSERT INTO update_queue_creators(queue_id, creator_id) VALUES(?, ?) ON CONFLICT(queue_id, creator_id) DO NOTHING`, queueID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RemoveCreatorFromUpdateQueue(queueID, creatorID int64) error {
	_, err := s.db.Exec(`DELETE FROM update_queue_creators WHERE queue_id=? AND creator_id=?`, queueID, creatorID)
	return err
}

func (s *Store) CreatorsForUpdateQueue(queueID int64) ([]Creator, error) {
	rows, err := s.db.Query(`SELECT c.id, c.directory_id, c.name, c.work_count, c.twitter_user_id, c.twitter_username, c.twitter_profile_url, c.online_identity_status, c.avatar_url, c.avatar_file, c.bio, c.profile_updated_at, `+creatorStatsSelectColumnsForAlias+`
FROM update_queue_creators uqc JOIN creators c ON c.id=uqc.creator_id WHERE uqc.queue_id=? ORDER BY c.name COLLATE NOCASE`, queueID)
	if err != nil {
		return nil, err
	}
	creators := make([]Creator, 0)
	for rows.Next() {
		creator, err := scanCreatorRow(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		creators = append(creators, creator)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range creators {
		linked, err := s.CreatorLinkedSummaries(creators[i].ID)
		if err != nil {
			return nil, err
		}
		creators[i].LinkedCreators = linked
	}
	return creators, nil
}

func (s *Store) CreateUpdateRun(queue UpdateQueue, total int) (UpdateRun, error) {
	userID, err := s.superAdminUserID()
	if err != nil {
		return UpdateRun{}, err
	}
	return s.CreateUpdateRunForUser(queue, total, userID)
}

func (s *Store) CreateUpdateRunForUser(queue UpdateQueue, total int, actorUserID int64) (UpdateRun, error) {
	result, err := s.db.Exec(`INSERT INTO update_runs(queue_id, queue_name, actor_user_id, total_creators, status, summary) VALUES(?, ?, ?, ?, 'queued', ?)`, queue.ID, queue.Name, actorUserID, total, "已加入立即执行队列")
	if err != nil {
		return UpdateRun{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return UpdateRun{}, err
	}
	return s.UpdateRun(id)
}

func (s *Store) UpdateRun(id int64) (UpdateRun, error) {
	row := s.db.QueryRow(`SELECT id, queue_id, queue_name, enqueued_at, total_creators, succeeded_creators, failed_creators, added_works, status, summary FROM update_runs WHERE id=?`, id)
	return scanUpdateRun(row)
}

func (s *Store) AddUpdateJobs(creatorIDs []int64, queueID, runID int64, sources ...string) ([]UpdateJob, error) {
	userID, err := s.superAdminUserID()
	if err != nil {
		return nil, err
	}
	return s.AddUpdateJobsForUser(creatorIDs, queueID, runID, userID, sources...)
}

func (s *Store) AddUpdateJobsForUser(creatorIDs []int64, queueID, runID, actorUserID int64, sources ...string) ([]UpdateJob, error) {
	source := normalizeUpdateJobSource(firstUpdateJobSource(sources))
	jobs := make([]UpdateJob, 0, len(creatorIDs))
	for _, creatorID := range creatorIDs {
		if creatorID <= 0 {
			continue
		}
		var existing int64
		err := s.db.QueryRow(`SELECT id FROM update_jobs WHERE creator_id=? AND status IN ('pending','running') LIMIT 1`, creatorID).Scan(&existing)
		if err == nil {
			job, err := s.UpdateJob(existing)
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, job)
			continue
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		result, err := s.db.Exec(`INSERT INTO update_jobs(creator_id, actor_user_id, cookie_owner_user_id, queue_id, run_id, source, status) VALUES(?, ?, ?, ?, ?, ?, 'pending')`, creatorID, actorUserID, actorUserID, queueID, runID, source)
		if err != nil {
			return nil, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return nil, err
		}
		job, err := s.UpdateJob(id)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

const updateJobCreatorNameSelect = `COALESCE(NULLIF(c.twitter_username, ''), CASE WHEN c.name LIKE '待更新-%' THEN substr(c.name, 5) ELSE c.name END)`

const updateJobSelectColumns = `j.id, j.creator_id, ` + updateJobCreatorNameSelect + `, j.actor_user_id, COALESCE(au.username,''), j.cookie_owner_user_id, COALESCE(cu.username,''), j.queue_id, j.run_id, j.retry_of_job_id, j.attempt, j.source, j.status, j.added_works, j.total_tweets, j.processed_tweets, j.downloaded_videos, j.skipped_tweets, j.download_speed_bytes_per_second, j.progress_message, j.download_directory, j.download_notice, j.error, j.error_detail, j.created_at, j.started_at, j.finished_at`

func (s *Store) NextPendingUpdateJob() (UpdateJob, error) {
	row := s.db.QueryRow(`SELECT ` + updateJobSelectColumns + `
FROM update_jobs j JOIN creators c ON c.id=j.creator_id
LEFT JOIN users au ON au.id=j.actor_user_id
LEFT JOIN users cu ON cu.id=j.cookie_owner_user_id
WHERE j.status='pending' ORDER BY j.id LIMIT 1`)
	return scanUpdateJob(row)
}

func (s *Store) ClaimNextPendingUpdateJob() (UpdateJob, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return UpdateJob{}, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRow(`SELECT id FROM update_jobs WHERE status='pending' ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		return UpdateJob{}, err
	}
	result, err := tx.Exec(`UPDATE update_jobs SET status='running', started_at=?, finished_at='', error='', error_detail='' WHERE id=? AND status='pending'`, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return UpdateJob{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return UpdateJob{}, err
	}
	if affected == 0 {
		return UpdateJob{}, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return UpdateJob{}, err
	}
	return s.UpdateJob(id)
}

func (s *Store) UpdateJob(id int64) (UpdateJob, error) {
	row := s.db.QueryRow(`SELECT `+updateJobSelectColumns+`
FROM update_jobs j JOIN creators c ON c.id=j.creator_id
LEFT JOIN users au ON au.id=j.actor_user_id
LEFT JOIN users cu ON cu.id=j.cookie_owner_user_id
WHERE j.id=?`, id)
	return scanUpdateJob(row)
}

func (s *Store) MarkUpdateJobRunning(id int64) error {
	_, err := s.db.Exec(`UPDATE update_jobs SET status='running', started_at=?, finished_at='', error='', error_detail='', download_speed_bytes_per_second=0 WHERE id=?`, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (s *Store) UpdateJobProgress(id int64, progress DownloadProgress) error {
	_, err := s.db.Exec(`UPDATE update_jobs SET total_tweets=?, processed_tweets=?, downloaded_videos=?, skipped_tweets=?, download_speed_bytes_per_second=?, progress_message=? WHERE id=?`,
		progress.TotalTweets,
		progress.ProcessedTweets,
		progress.DownloadedVideos,
		progress.SkippedTweets,
		progress.DownloadSpeedBps,
		progress.LastMessage,
		id,
	)
	return err
}

func (s *Store) UpdateJobDownloadInfo(id int64, directory string, notice string) error {
	_, err := s.db.Exec(`UPDATE update_jobs SET download_directory=?, download_notice=? WHERE id=?`, directory, notice, id)
	return err
}

func (s *Store) FinishUpdateJob(id int64, status string, addedWorks int, message string, detail ...string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var runID, queueID int64
	if err := tx.QueryRow(`SELECT run_id, queue_id FROM update_jobs WHERE id=?`, id).Scan(&runID, &queueID); err != nil {
		return err
	}
	errorDetail := ""
	if len(detail) > 0 {
		errorDetail = detail[0]
	}
	if _, err := tx.Exec(`UPDATE update_jobs SET status=?, added_works=?, download_speed_bytes_per_second=0, error=?, error_detail=?, finished_at=? WHERE id=?`, status, addedWorks, message, errorDetail, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		return err
	}
	if runID > 0 {
		successDelta := 0
		failedDelta := 0
		if status == "succeeded" {
			successDelta = 1
		}
		if status == "failed" {
			failedDelta = 1
		}
		summary := "成功 " + strconv.Itoa(successDelta) + " 个博主，新增 " + strconv.Itoa(addedWorks) + " 个作品"
		if _, err := tx.Exec(`UPDATE update_runs SET
  succeeded_creators=succeeded_creators+?,
  failed_creators=failed_creators+?,
  added_works=added_works+?,
  status=CASE WHEN succeeded_creators + failed_creators + ? + ? >= total_creators THEN 'completed' ELSE status END,
  summary=?
WHERE id=?`, successDelta, failedDelta, addedWorks, successDelta, failedDelta, summary, runID); err != nil {
			return err
		}
		if queueID > 0 {
			_, _ = tx.Exec(`UPDATE update_queues SET last_run_summary=(SELECT summary FROM update_runs WHERE id=?), updated_at=? WHERE id=?`, runID, time.Now().UTC().Format(time.RFC3339), queueID)
		}
	}
	return tx.Commit()
}

func (s *Store) RetryUpdateJob(id int64) (UpdateJob, bool, error) {
	userID, err := s.superAdminUserID()
	if err != nil {
		return UpdateJob{}, false, err
	}
	return s.RetryUpdateJobForUser(id, userID)
}

func (s *Store) RetryUpdateJobForUser(id, actorUserID int64) (UpdateJob, bool, error) {
	if actorUserID <= 0 {
		return UpdateJob{}, false, errors.New("user id is required")
	}
	original, err := s.UpdateJob(id)
	if err != nil {
		return UpdateJob{}, false, err
	}
	if original.Status != "failed" {
		return original, false, nil
	}
	var existing int64
	err = s.db.QueryRow(`SELECT id FROM update_jobs WHERE creator_id=? AND status IN ('pending','running') ORDER BY id LIMIT 1`, original.CreatorID).Scan(&existing)
	if err == nil {
		job, err := s.UpdateJob(existing)
		return job, false, err
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return UpdateJob{}, false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return UpdateJob{}, false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE update_jobs SET
  actor_user_id=?,
  cookie_owner_user_id=?,
  source='manual',
  status='pending',
  attempt=attempt+1,
  added_works=0,
  total_tweets=0,
  processed_tweets=0,
  downloaded_videos=0,
  skipped_tweets=0,
  download_speed_bytes_per_second=0,
  progress_message='等待重试',
  download_directory='',
  download_notice='',
  error='',
  error_detail='',
  started_at='',
  finished_at=''
WHERE id=?`, actorUserID, actorUserID, id); err != nil {
		return UpdateJob{}, false, err
	}
	if original.RunID > 0 {
		if _, err := tx.Exec(`UPDATE update_runs SET
  failed_creators=CASE WHEN failed_creators > 0 THEN failed_creators - 1 ELSE 0 END,
  status='running',
  summary=''
WHERE id=?`, original.RunID); err != nil {
			return UpdateJob{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return UpdateJob{}, false, err
	}
	job, err := s.UpdateJob(id)
	return job, err == nil, err
}

func (s *Store) UpdateJobs(limit int) ([]UpdateJob, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+updateJobSelectColumns+`
FROM update_jobs j JOIN creators c ON c.id=j.creator_id
LEFT JOIN users au ON au.id=j.actor_user_id
LEFT JOIN users cu ON cu.id=j.cookie_owner_user_id
ORDER BY j.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]UpdateJob, 0)
	for rows.Next() {
		job, err := scanUpdateJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) UpdateJobsPage(page, pageSize int) (UpdateJobsPage, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM update_jobs`).Scan(&total); err != nil {
		return UpdateJobsPage{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	offset := (page - 1) * pageSize
	rows, err := s.db.Query(`SELECT `+updateJobSelectColumns+`
FROM update_jobs j JOIN creators c ON c.id=j.creator_id
LEFT JOIN users au ON au.id=j.actor_user_id
LEFT JOIN users cu ON cu.id=j.cookie_owner_user_id
ORDER BY j.id DESC LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return UpdateJobsPage{}, err
	}
	defer rows.Close()
	jobs := make([]UpdateJob, 0)
	for rows.Next() {
		job, err := scanUpdateJob(rows)
		if err != nil {
			return UpdateJobsPage{}, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return UpdateJobsPage{}, err
	}
	return UpdateJobsPage{Jobs: jobs, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

func (s *Store) RunningUpdateJobs() ([]UpdateJob, error) {
	rows, err := s.db.Query(`SELECT ` + updateJobSelectColumns + `
FROM update_jobs j JOIN creators c ON c.id=j.creator_id
LEFT JOIN users au ON au.id=j.actor_user_id
LEFT JOIN users cu ON cu.id=j.cookie_owner_user_id
WHERE j.status='running'
ORDER BY j.started_at DESC, j.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]UpdateJob, 0)
	for rows.Next() {
		job, err := scanUpdateJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) DeleteUpdateJob(id int64) error {
	if id <= 0 {
		return errors.New("job id is required")
	}
	var status string
	if err := s.db.QueryRow(`SELECT status FROM update_jobs WHERE id=?`, id).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status == "running" {
		return errors.New("running update job cannot be deleted")
	}
	_, err := s.db.Exec(`DELETE FROM update_jobs WHERE id=?`, id)
	return err
}

func (s *Store) ClearUpdateJobs() error {
	_, err := s.db.Exec(`DELETE FROM update_jobs`)
	return err
}

func firstUpdateJobSource(values []string) string {
	if len(values) == 0 {
		return "manual"
	}
	return values[0]
}

func normalizeUpdateJobSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "scheduled":
		return "scheduled"
	default:
		return "manual"
	}
}

type UpdateAuthInput struct {
	AuthToken             *string
	CT0                   *string
	CookieHeader          *string
	Proxy                 *string
	DownloadMode          *string
	DownloadDirectory     *string
	DownloaderConcurrency *int
	FetchLimit            *int
	MediaOnly             *bool
	IncludeRetweets       *bool
}

type UpdateAuthSecret struct {
	AuthToken             string
	CT0                   string
	CookieHeader          string
	Proxy                 string
	DownloadMode          string
	DownloadDirectory     string
	DownloaderConcurrency int
	FetchLimit            int
	MediaOnly             bool
	IncludeRetweets       bool
}

type AuditEventInput struct {
	ActorUserID int64
	EventType   string
	TargetType  string
	TargetID    int64
	Message     string
	Detail      any
}

type SystemEventInput struct {
	EventType string
	Severity  string
	Message   string
	Detail    any
}

type LogQuery struct {
	Page      int
	PageSize  int
	UserID    int64
	EventType string
	Keyword   string
}

func (s *Store) SaveUpdateAuth(input UpdateAuthInput) error {
	userID, err := s.superAdminUserID()
	if err != nil {
		return err
	}
	return s.SaveUpdateAuthForUser(userID, input)
}

func (s *Store) SaveUpdateAuthForUser(userID int64, input UpdateAuthInput) error {
	if err := s.ensureUpdateAuthForUser(userID); err != nil {
		return err
	}
	current, err := s.updateAuthSecretForUser(userID)
	if err != nil {
		return err
	}
	authToken := current.AuthToken
	ct0 := current.CT0
	cookieHeader := current.CookieHeader
	proxy := current.Proxy
	downloadMode := normalizeUpdateDownloadMode(current.DownloadMode)
	downloadDirectory := current.DownloadDirectory
	downloaderConcurrency := normalizeDownloaderConcurrency(current.DownloaderConcurrency)
	limit := current.FetchLimit
	mediaOnly := current.MediaOnly
	includeRetweets := current.IncludeRetweets
	if input.AuthToken != nil {
		authToken = *input.AuthToken
	}
	if input.CT0 != nil {
		ct0 = *input.CT0
	}
	if input.CookieHeader != nil {
		cookieHeader = *input.CookieHeader
	}
	if input.Proxy != nil {
		proxy = *input.Proxy
	}
	if input.DownloadMode != nil {
		downloadMode = normalizeUpdateDownloadMode(*input.DownloadMode)
	}
	if input.DownloadDirectory != nil {
		downloadDirectory = strings.TrimSpace(*input.DownloadDirectory)
	}
	if input.DownloaderConcurrency != nil {
		downloaderConcurrency = normalizeDownloaderConcurrency(*input.DownloaderConcurrency)
	}
	if input.FetchLimit != nil {
		limit = *input.FetchLimit
	}
	if input.MediaOnly != nil {
		mediaOnly = *input.MediaOnly
	}
	if input.IncludeRetweets != nil {
		includeRetweets = *input.IncludeRetweets
	}
	if limit <= 0 {
		limit = 300
	}
	_, err = s.db.Exec(`UPDATE user_update_auth_settings SET auth_token=?, ct0=?, cookie_header=?, proxy=?, download_mode=?, download_directory=?, downloader_concurrency=?, fetch_limit=?, media_only=?, include_retweets=?, updated_at=?, validation_status='unknown', validation_message='' WHERE user_id=?`,
		authToken, ct0, cookieHeader, proxy, downloadMode, downloadDirectory, downloaderConcurrency, limit, boolToInt(mediaOnly), boolToInt(includeRetweets), time.Now().UTC().Format(time.RFC3339), userID)
	return err
}

func (s *Store) UpdateAuth() (UpdateAuth, error) {
	userID, err := s.superAdminUserID()
	if err != nil {
		return UpdateAuth{}, err
	}
	return s.UpdateAuthForUser(userID)
}

func (s *Store) UpdateAuthForUser(userID int64) (UpdateAuth, error) {
	if err := s.ensureUpdateAuthForUser(userID); err != nil {
		return UpdateAuth{}, err
	}
	var auth UpdateAuth
	var authToken, ct0, cookieHeader string
	var mediaOnly, includeRetweets int
	err := s.db.QueryRow(`SELECT auth_token, ct0, cookie_header, proxy, download_mode, download_directory, downloader_concurrency, fetch_limit, media_only, include_retweets, updated_at, validation_status, validation_message FROM user_update_auth_settings WHERE user_id=?`, userID).
		Scan(&authToken, &ct0, &cookieHeader, &auth.Proxy, &auth.DownloadMode, &auth.DownloadDirectory, &auth.DownloaderConcurrency, &auth.FetchLimit, &mediaOnly, &includeRetweets, &auth.UpdatedAt, &auth.ValidationStatus, &auth.ValidationMessage)
	if err != nil {
		return auth, err
	}
	auth.Configured = (strings.TrimSpace(authToken) != "" && strings.TrimSpace(ct0) != "") || strings.TrimSpace(cookieHeader) != ""
	auth.AuthToken = authToken
	auth.CT0 = ct0
	auth.CookieHeaderSaved = strings.TrimSpace(cookieHeader) != ""
	auth.DownloadMode = normalizeUpdateDownloadMode(auth.DownloadMode)
	auth.DownloaderConcurrency = normalizeDownloaderConcurrency(auth.DownloaderConcurrency)
	auth.MediaOnly = mediaOnly == 1
	auth.IncludeRetweets = includeRetweets == 1
	return auth, nil
}

func (s *Store) updateAuthSecret() (UpdateAuthSecret, error) {
	userID, err := s.superAdminUserID()
	if err != nil {
		return UpdateAuthSecret{}, err
	}
	return s.updateAuthSecretForUser(userID)
}

func (s *Store) updateAuthSecretForUser(userID int64) (UpdateAuthSecret, error) {
	if err := s.ensureUpdateAuthForUser(userID); err != nil {
		return UpdateAuthSecret{}, err
	}
	var secret UpdateAuthSecret
	var mediaOnly, includeRetweets int
	err := s.db.QueryRow(`SELECT auth_token, ct0, cookie_header, proxy, download_mode, download_directory, downloader_concurrency, fetch_limit, media_only, include_retweets FROM user_update_auth_settings WHERE user_id=?`, userID).
		Scan(&secret.AuthToken, &secret.CT0, &secret.CookieHeader, &secret.Proxy, &secret.DownloadMode, &secret.DownloadDirectory, &secret.DownloaderConcurrency, &secret.FetchLimit, &mediaOnly, &includeRetweets)
	secret.DownloadMode = normalizeUpdateDownloadMode(secret.DownloadMode)
	secret.DownloaderConcurrency = normalizeDownloaderConcurrency(secret.DownloaderConcurrency)
	secret.MediaOnly = mediaOnly == 1
	secret.IncludeRetweets = includeRetweets == 1
	return secret, err
}

func (s *Store) MarkUpdateAuthValidation(status, message string) error {
	userID, err := s.superAdminUserID()
	if err != nil {
		return err
	}
	return s.MarkUpdateAuthValidationForUser(userID, status, message)
}

func (s *Store) MarkUpdateAuthValidationForUser(userID int64, status, message string) error {
	if err := s.ensureUpdateAuthForUser(userID); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE user_update_auth_settings SET validation_status=?, validation_message=?, updated_at=? WHERE user_id=?`, status, message, time.Now().UTC().Format(time.RFC3339), userID)
	return err
}

func (s *Store) SetHiddenCreatorsForUser(userID int64, creatorIDs []int64) error {
	if userID <= 0 {
		return errors.New("user id is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM creator_visibility_overrides WHERE user_id=?`, userID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	seen := map[int64]bool{}
	for _, creatorID := range creatorIDs {
		if creatorID <= 0 || seen[creatorID] {
			continue
		}
		seen[creatorID] = true
		if _, err := tx.Exec(`INSERT INTO creator_visibility_overrides(user_id, creator_id, hidden, created_at, updated_at) VALUES(?, ?, 1, ?, ?)`, userID, creatorID, now, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RecordAuditEvent(input AuditEventInput) error {
	eventType := strings.TrimSpace(input.EventType)
	if eventType == "" {
		return errors.New("event type is required")
	}
	detailJSON, err := encodeLogDetail(input.Detail)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO audit_events(actor_user_id, event_type, target_type, target_id, message, detail_json, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		input.ActorUserID,
		eventType,
		strings.TrimSpace(input.TargetType),
		input.TargetID,
		strings.TrimSpace(input.Message),
		detailJSON,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) RecordSystemEvent(input SystemEventInput) error {
	eventType := strings.TrimSpace(input.EventType)
	if eventType == "" {
		return errors.New("event type is required")
	}
	severity := strings.TrimSpace(input.Severity)
	if severity == "" {
		severity = "info"
	}
	detailJSON, err := encodeLogDetail(input.Detail)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO system_events(event_type, severity, message, detail_json, created_at) VALUES(?, ?, ?, ?, ?)`,
		eventType,
		severity,
		strings.TrimSpace(input.Message),
		detailJSON,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) AuditEvents(query LogQuery) (LogEventsPage, error) {
	query.Page, query.PageSize = normalizeLogPage(query.Page, query.PageSize)
	where := []string{"1=1"}
	args := []any{}
	if query.UserID > 0 {
		where = append(where, "ae.actor_user_id=?")
		args = append(args, query.UserID)
	}
	if strings.TrimSpace(query.EventType) != "" {
		where = append(where, "ae.event_type=?")
		args = append(args, strings.TrimSpace(query.EventType))
	}
	if strings.TrimSpace(query.Keyword) != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(query.Keyword)) + "%"
		where = append(where, "(LOWER(ae.event_type) LIKE ? OR LOWER(ae.message) LIKE ? OR LOWER(ae.detail_json) LIKE ? OR LOWER(COALESCE(u.username,'')) LIKE ?)")
		args = append(args, like, like, like, like)
	}
	return s.logEventsPage(`audit_events ae LEFT JOIN users u ON u.id=ae.actor_user_id`, `ae.id, ae.actor_user_id, COALESCE(u.username,''), ae.event_type, ae.target_type, ae.target_id, '', ae.message, ae.detail_json, ae.created_at`, strings.Join(where, " AND "), args, query.Page, query.PageSize)
}

func (s *Store) SystemEvents(query LogQuery) (LogEventsPage, error) {
	query.Page, query.PageSize = normalizeLogPage(query.Page, query.PageSize)
	where := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(query.EventType) != "" {
		where = append(where, "se.event_type=?")
		args = append(args, strings.TrimSpace(query.EventType))
	}
	if strings.TrimSpace(query.Keyword) != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(query.Keyword)) + "%"
		where = append(where, "(LOWER(se.event_type) LIKE ? OR LOWER(se.severity) LIKE ? OR LOWER(se.message) LIKE ? OR LOWER(se.detail_json) LIKE ?)")
		args = append(args, like, like, like, like)
	}
	return s.logEventsPage(`system_events se`, `se.id, 0, '', se.event_type, '', 0, se.severity, se.message, se.detail_json, se.created_at`, strings.Join(where, " AND "), args, query.Page, query.PageSize)
}

func (s *Store) logEventsPage(from, columns, where string, args []any, page, pageSize int) (LogEventsPage, error) {
	countArgs := append([]any{}, args...)
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM `+from+` WHERE `+where, countArgs...).Scan(&total); err != nil {
		return LogEventsPage{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := s.db.Query(`SELECT `+columns+` FROM `+from+` WHERE `+where+` ORDER BY 1 DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return LogEventsPage{}, err
	}
	defer rows.Close()
	events := []LogEvent{}
	for rows.Next() {
		event, err := scanLogEvent(rows)
		if err != nil {
			return LogEventsPage{}, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return LogEventsPage{}, err
	}
	return LogEventsPage{Events: events, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

func encodeLogDetail(detail any) (string, error) {
	if detail == nil {
		return "", nil
	}
	if text, ok := detail.(string); ok {
		return text, nil
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func normalizeLogPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func scanLogEvent(scanner interface{ Scan(...any) error }) (LogEvent, error) {
	var event LogEvent
	var created string
	if err := scanner.Scan(&event.ID, &event.ActorUserID, &event.ActorName, &event.EventType, &event.TargetType, &event.TargetID, &event.Severity, &event.Message, &event.DetailJSON, &created); err != nil {
		return LogEvent{}, err
	}
	event.CreatedAt = parseDBTime(created)
	return event, nil
}

func scanDirectory(scanner interface{ Scan(...any) error }) (Directory, error) {
	var dir Directory
	var last sql.NullString
	var created string
	if err := scanner.Scan(&dir.ID, &dir.Path, &dir.Name, &dir.Status, &last, &created); err != nil {
		return Directory{}, err
	}
	if last.Valid && last.String != "" {
		t, err := time.Parse(time.RFC3339, last.String)
		if err == nil {
			dir.LastScannedAt = &t
		}
	}
	t, err := time.Parse(time.RFC3339, created)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", created)
	}
	if err == nil {
		dir.CreatedAt = t
	}
	return dir, nil
}

func scanCreatorRow(scanner interface{ Scan(...any) error }) (Creator, error) {
	var creator Creator
	if err := scanner.Scan(&creator.ID, &creator.DirectoryID, &creator.Name, &creator.WorkCount, &creator.TwitterUserID, &creator.TwitterUsername, &creator.TwitterProfileURL, &creator.OnlineIdentityStatus, &creator.AvatarURL, &creator.AvatarFile, &creator.Bio, &creator.ProfileUpdatedAt, &creator.ViewCount, &creator.LastWorkCreatedAt); err != nil {
		return Creator{}, err
	}
	if creator.AvatarFile != "" {
		creator.AvatarURL = "/api/creators/" + strconv.FormatInt(creator.ID, 10) + "/avatar"
	}
	return creator, nil
}

func scanCreatorSummary(scanner interface{ Scan(...any) error }) (CreatorSummary, error) {
	var summary CreatorSummary
	var avatarFile string
	if err := scanner.Scan(&summary.ID, &summary.Name, &summary.TwitterUsername, &summary.AvatarURL, &avatarFile); err != nil {
		return CreatorSummary{}, err
	}
	if avatarFile != "" {
		summary.AvatarURL = "/api/creators/" + strconv.FormatInt(summary.ID, 10) + "/avatar"
	}
	return summary, nil
}

func scanWork(scanner interface{ Scan(...any) error }) (Work, error) {
	var work Work
	var tagsJSON, created, creatorAvatarFile string
	if err := scanner.Scan(&work.ID, &work.DirectoryID, &work.CreatorID, &work.CreatorName, &work.CreatorAvatarURL, &creatorAvatarFile, &work.CreatorBio, &work.AwemeID, &work.Title, &work.Description, &work.MusicTitle, &work.SourceURL, &work.CoverURL, &work.CoverFile, &tagsJSON, &work.PublishedAt, &created); err != nil {
		return Work{}, err
	}
	if creatorAvatarFile != "" {
		work.CreatorAvatarURL = "/api/creators/" + strconv.FormatInt(work.CreatorID, 10) + "/avatar"
	}
	if work.CoverFile != "" {
		work.CoverURL = "/api/works/" + strconv.FormatInt(work.ID, 10) + "/cover"
	}
	_ = json.Unmarshal([]byte(tagsJSON), &work.Tags)
	t, err := time.Parse(time.RFC3339, created)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", created)
	}
	if err == nil {
		work.CreatedAt = t
	}
	return work, nil
}

func scanFavoriteFolder(scanner interface{ Scan(...any) error }) (FavoriteFolder, error) {
	var folder FavoriteFolder
	var isDefault int
	var created, updated string
	if err := scanner.Scan(&folder.ID, &folder.UserID, &folder.Type, &folder.Name, &isDefault, &created, &updated); err != nil {
		return FavoriteFolder{}, err
	}
	folder.IsDefault = isDefault == 1
	if t, err := time.Parse(time.RFC3339, created); err == nil {
		folder.CreatedAt = t
	} else if t, err := time.Parse("2006-01-02 15:04:05", created); err == nil {
		folder.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updated); err == nil {
		folder.UpdatedAt = t
	} else if t, err := time.Parse("2006-01-02 15:04:05", updated); err == nil {
		folder.UpdatedAt = t
	}
	return folder, nil
}

func scanUpdateQueue(scanner interface{ Scan(...any) error }) (UpdateQueue, error) {
	var queue UpdateQueue
	var isDefault, enabled int
	var created, updated string
	if err := scanner.Scan(&queue.ID, &queue.Name, &queue.Cron, &isDefault, &enabled, &queue.NextRunAt, &queue.LastRunSummary, &created, &updated, &queue.SubscribedCount); err != nil {
		return UpdateQueue{}, err
	}
	queue.IsDefault = isDefault == 1
	queue.Enabled = enabled == 1
	queue.CreatedAt = parseDBTime(created)
	queue.UpdatedAt = parseDBTime(updated)
	return queue, nil
}

func scanUpdateRun(scanner interface{ Scan(...any) error }) (UpdateRun, error) {
	var run UpdateRun
	var enqueued string
	if err := scanner.Scan(&run.ID, &run.QueueID, &run.QueueName, &enqueued, &run.TotalCreators, &run.SucceededCreators, &run.FailedCreators, &run.AddedWorks, &run.Status, &run.Summary); err != nil {
		return UpdateRun{}, err
	}
	run.EnqueuedAt = parseDBTime(enqueued)
	return run, nil
}

func scanUpdateJob(scanner interface{ Scan(...any) error }) (UpdateJob, error) {
	var job UpdateJob
	var created string
	if err := scanner.Scan(
		&job.ID,
		&job.CreatorID,
		&job.CreatorName,
		&job.ActorUserID,
		&job.ActorUsername,
		&job.CookieOwnerUserID,
		&job.CookieOwnerName,
		&job.QueueID,
		&job.RunID,
		&job.RetryOfJobID,
		&job.Attempt,
		&job.Source,
		&job.Status,
		&job.AddedWorks,
		&job.TotalTweets,
		&job.ProcessedTweets,
		&job.DownloadedVideos,
		&job.SkippedTweets,
		&job.DownloadSpeedBps,
		&job.ProgressMessage,
		&job.DownloadDirectory,
		&job.DownloadNotice,
		&job.Error,
		&job.ErrorDetail,
		&created,
		&job.StartedAt,
		&job.FinishedAt,
	); err != nil {
		return UpdateJob{}, err
	}
	if job.Attempt <= 0 {
		job.Attempt = 1
	}
	job.Source = normalizeUpdateJobSource(job.Source)
	job.CreatedAt = parseDBTime(created)
	return job, nil
}

func parseDBTime(value string) time.Time {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return t
	}
	return time.Time{}
}

var ErrNotFound = errors.New("not found")
