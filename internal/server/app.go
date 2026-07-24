package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type App struct {
	mu           sync.RWMutex
	store        *Store
	scanner      *Scanner
	hub          *EventHub
	updater      *UpdateManager
	browser      FileBrowser
	web          WebAssets
	configPath   string
	databaseDir  string
	databasePath string
}

type WebAssets struct {
	Dir string
	FS  fs.FS
}

func NewApp(dbPath string, webDir string) (*App, error) {
	return NewAppWithAssets(dbPath, WebAssets{Dir: webDir})
}

func NewAppWithAssets(dbPath string, web WebAssets) (*App, error) {
	store, err := OpenStore(dbPath)
	if err != nil {
		return nil, err
	}
	hub := NewEventHub()
	cleanDB := filepath.Clean(dbPath)
	return &App{store: store, scanner: NewScanner(store, hub), hub: hub, updater: NewUpdateManager(store, hub, nil), browser: NewFileBrowserFromEnv(), web: web, databaseDir: filepath.Dir(cleanDB), databasePath: cleanDB}, nil
}

func NewConfiguredApp(configPath string, defaultDatabaseDir string, webDir string) (*App, error) {
	return NewConfiguredAppWithAssets(configPath, defaultDatabaseDir, WebAssets{Dir: webDir})
}

func NewConfiguredAppWithAssets(configPath string, defaultDatabaseDir string, web WebAssets) (*App, error) {
	cfg, err := LoadConfig(configPath, defaultDatabaseDir)
	if err != nil {
		return nil, err
	}
	dbInfo, err := InspectDatabaseDirectory(cfg.DatabaseDir)
	if err != nil {
		return nil, err
	}
	store, err := OpenStore(dbInfo.Path)
	if err != nil {
		return nil, err
	}
	if err := SaveConfig(configPath, AppConfig{DatabaseDir: dbInfo.Directory}); err != nil {
		_ = store.Close()
		return nil, err
	}
	hub := NewEventHub()
	return &App{
		store:        store,
		scanner:      NewScanner(store, hub),
		hub:          hub,
		updater:      NewUpdateManager(store, hub, nil),
		browser:      NewFileBrowserFromEnv(),
		web:          web,
		configPath:   configPath,
		databaseDir:  dbInfo.Directory,
		databasePath: dbInfo.Path,
	}, nil
}

func (a *App) Close() error {
	a.mu.RLock()
	store := a.store
	updater := a.updater
	a.mu.RUnlock()
	if updater != nil {
		updater.Close()
	}
	return store.Close()
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	api := http.NewServeMux()
	api.HandleFunc("GET /api/auth/me", a.handleMe)
	api.HandleFunc("POST /api/auth/logout", a.handleLogout)
	api.HandleFunc("POST /api/auth/password", a.handleChangePassword)
	api.HandleFunc("GET /api/users", a.handleUsers)
	api.HandleFunc("POST /api/users", a.handleCreateUser)
	api.HandleFunc("GET /api/users/view-context", a.handleViewContext)
	api.HandleFunc("PUT /api/users/view-context", a.handleSetViewContext)
	api.HandleFunc("DELETE /api/users/view-context", a.handleClearViewContext)
	api.HandleFunc("POST /api/users/{id}/reset-password", a.handleResetUserPassword)
	api.HandleFunc("DELETE /api/users/{id}", a.handleDeleteUser)
	api.HandleFunc("GET /api/about", a.handleAbout)
	api.HandleFunc("GET /api/logs/audit", a.handleAuditLogs)
	api.HandleFunc("GET /api/logs/system", a.handleSystemLogs)
	api.HandleFunc("POST /api/logs/player", a.handlePlayerLog)
	api.HandleFunc("GET /api/events", a.handleEvents)
	api.HandleFunc("GET /api/fs/roots", a.handleFSRoots)
	api.HandleFunc("GET /api/fs/list", a.handleFSList)
	api.HandleFunc("GET /api/config/database", a.handleDatabaseConfig)
	api.HandleFunc("PUT /api/config/database", a.handleUpdateDatabaseConfig)
	api.HandleFunc("GET /api/directories", a.handleDirectories)
	api.HandleFunc("POST /api/directories", a.handleCreateDirectory)
	api.HandleFunc("DELETE /api/directories/{id}", a.handleDeleteDirectory)
	api.HandleFunc("POST /api/directories/{id}/scan", a.handleScanDirectory)
	api.HandleFunc("POST /api/directories/scan-all", a.handleScanAll)
	api.HandleFunc("GET /api/scan/status", a.handleScanStatus)
	api.HandleFunc("GET /api/scan/events", a.handleScanEvents)
	api.HandleFunc("GET /api/creators", a.handleCreators)
	api.HandleFunc("GET /api/creators/{id}/avatar", a.handleCreatorAvatar)
	api.HandleFunc("GET /api/creators/{id}/links", a.handleCreatorLinks)
	api.HandleFunc("PUT /api/creators/{id}/links", a.handleSaveCreatorLinks)
	api.HandleFunc("POST /api/creators/{id}/scan", a.handleScanCreator)
	api.HandleFunc("POST /api/creators/bulk-import/preview", a.handlePreviewBulkImportCreators)
	api.HandleFunc("POST /api/creators/bulk-import", a.handleBulkImportCreators)
	api.HandleFunc("GET /api/feed", a.handleFeed)
	api.HandleFunc("GET /api/works/{id}", a.handleWork)
	api.HandleFunc("POST /api/works/{id}/view", a.handleRecordWorkView)
	api.HandleFunc("GET /api/works/{id}/cover", a.handleWorkCover)
	api.HandleFunc("GET /api/media/{id}", a.handleMedia)
	api.HandleFunc("GET /api/update/status", a.handleUpdateStatus)
	api.HandleFunc("GET /api/update/jobs", a.handleUpdateJobs)
	api.HandleFunc("DELETE /api/update/jobs/{id}", a.handleDeleteUpdateJob)
	api.HandleFunc("POST /api/update/jobs/{id}/retry", a.handleRetryUpdateJob)
	api.HandleFunc("GET /api/update/events", a.handleUpdateEvents)
	api.HandleFunc("POST /api/update/immediate", a.handleUpdateImmediate)
	api.HandleFunc("POST /api/update/immediate/reset", a.handleResetUpdateImmediate)
	api.HandleFunc("GET /api/update/queues", a.handleUpdateQueues)
	api.HandleFunc("POST /api/update/queues", a.handleCreateUpdateQueue)
	api.HandleFunc("PATCH /api/update/queues/{id}", a.handlePatchUpdateQueue)
	api.HandleFunc("DELETE /api/update/queues/{id}", a.handleDeleteUpdateQueue)
	api.HandleFunc("GET /api/update/queues/{id}/creators", a.handleUpdateQueueCreators)
	api.HandleFunc("POST /api/update/queues/{id}/creators", a.handleAddUpdateQueueCreators)
	api.HandleFunc("DELETE /api/update/queues/{id}/creators/{creator_id}", a.handleRemoveUpdateQueueCreator)
	api.HandleFunc("POST /api/update/queues/{id}/run-now", a.handleRunUpdateQueueNow)
	api.HandleFunc("GET /api/update/auth", a.handleUpdateAuth)
	api.HandleFunc("PATCH /api/update/auth", a.handleSaveUpdateAuth)
	api.HandleFunc("POST /api/update/auth/validate", a.handleValidateUpdateAuth)
	api.HandleFunc("GET /api/favorite-folders", a.handleFavoriteFolders)
	api.HandleFunc("POST /api/favorite-folders", a.handleCreateFavoriteFolder)
	api.HandleFunc("PATCH /api/favorite-folders/{id}", a.handleUpdateFavoriteFolder)
	api.HandleFunc("GET /api/favorites/status", a.handleFavoriteStatus)
	api.HandleFunc("GET /api/favorites/works", a.handleFavoriteWorks)
	api.HandleFunc("POST /api/favorites/works", a.handleAddWorkFavorite)
	api.HandleFunc("DELETE /api/favorites/works/{id}", a.handleRemoveWorkFavorite)
	api.HandleFunc("GET /api/favorites/creators", a.handleFavoriteCreators)
	api.HandleFunc("POST /api/favorites/creators", a.handleAddCreatorFavorite)
	api.HandleFunc("DELETE /api/favorites/creators/{id}", a.handleRemoveCreatorFavorite)
	mux.Handle("/api/", a.authMiddleware(api))
	mux.HandleFunc("/", a.handleWeb)
	return logging(cors(mux))
}

type contextKey string

const currentUserContextKey contextKey = "current-user"
const currentViewUserContextKey contextKey = "current-view-user"

const viewUserCookieName = "localtwitter_view_user"

func (a *App) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := currentSessionToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "login required")
			return
		}
		user, err := a.currentStore().UserBySession(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "login required")
			return
		}
		viewUser := user
		if user.IsSuperAdmin {
			if cookie, err := r.Cookie(viewUserCookieName); err == nil {
				if id, err := strconv.ParseInt(cookie.Value, 10, 64); err == nil && id > 0 {
					if candidate, err := a.currentStore().User(id); err == nil {
						viewUser = candidate
					}
				}
			}
		}
		ctx := context.WithValue(r.Context(), currentUserContextKey, user)
		ctx = context.WithValue(ctx, currentViewUserContextKey, viewUser)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *App) currentUser(r *http.Request) (User, bool) {
	user, ok := r.Context().Value(currentUserContextKey).(User)
	return user, ok
}

func (a *App) currentViewUser(r *http.Request) (User, bool) {
	user, ok := r.Context().Value(currentViewUserContextKey).(User)
	if ok {
		return user, true
	}
	return a.currentUser(r)
}

func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) (User, bool) {
	user, ok := a.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "login required")
		return User{}, false
	}
	if !user.IsAdmin {
		writeError(w, http.StatusForbidden, "admin required")
		return User{}, false
	}
	return user, true
}

func (a *App) requireSuperAdmin(w http.ResponseWriter, r *http.Request) (User, bool) {
	user, ok := a.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "login required")
		return User{}, false
	}
	if !user.IsSuperAdmin {
		writeError(w, http.StatusForbidden, "super admin required")
		return User{}, false
	}
	return user, true
}

func (a *App) requireUpdater(w http.ResponseWriter, r *http.Request) (User, bool) {
	user, ok := a.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "login required")
		return User{}, false
	}
	if !user.CanUpdate {
		writeError(w, http.StatusForbidden, "update permission required")
		return User{}, false
	}
	return user, true
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	store := a.currentStore()
	user, err := store.Authenticate(body.Username, body.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	token, err := store.CreateSession(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.SetCookie(w, store.SessionCookie(token))
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token, ok := currentSessionToken(r); ok {
		_ = a.currentStore().DeleteSession(token)
	}
	http.SetCookie(w, a.currentStore().ExpiredSessionCookie())
	http.SetCookie(w, &http.Cookie{Name: viewUserCookieName, Value: "", Path: "/", MaxAge: -1, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	updated, err := a.currentStore().ChangePassword(user.ID, body.OldPassword, body.NewPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}

func (a *App) handleUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperAdmin(w, r); !ok {
		return
	}
	users, err := a.currentStore().Users()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (a *App) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperAdmin(w, r); !ok {
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, err := a.currentStore().CreateUser(body.Username, body.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (a *App) handleViewContext(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	view, _ := a.currentViewUser(r)
	writeJSON(w, http.StatusOK, ViewContext{ActorUser: actor, ViewUser: view})
}

func (a *App) handleSetViewContext(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	view, err := a.currentStore().User(body.UserID)
	if err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, sql.ErrNoRows) {
			status = http.StatusInternalServerError
		}
		writeError(w, status, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     viewUserCookieName,
		Value:    strconv.FormatInt(view.ID, 10),
		Path:     "/",
		MaxAge:   int(sessionLifetime.Seconds()),
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, ViewContext{ActorUser: actor, ViewUser: view})
}

func (a *App) handleClearViewContext(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	http.SetCookie(w, &http.Cookie{Name: viewUserCookieName, Value: "", Path: "/", MaxAge: -1, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, ViewContext{ActorUser: actor, ViewUser: actor})
}

func (a *App) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperAdmin(w, r); !ok {
		return
	}
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, err := a.currentStore().ResetUserPassword(id, body.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (a *App) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	current, ok := a.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	if id == current.ID {
		writeError(w, http.StatusBadRequest, "cannot delete current user")
		return
	}
	if err := a.currentStore().DeleteUser(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAbout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"about": About()})
}

func (a *App) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperAdmin(w, r); !ok {
		return
	}
	query := logQueryFromRequest(r)
	page, err := a.currentStore().AuditEvents(query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (a *App) handlePlayerLog(w http.ResponseWriter, r *http.Request) {
	user, _ := a.currentUser(r)
	var body struct {
		Action         string `json:"action"`
		PlaySessionID  string `json:"play_session_id"`
		WorkID         int64  `json:"work_id"`
		MediaID        int64  `json:"media_id"`
		FileName       string `json:"file_name"`
		LoadTimeMs     int    `json:"load_time_ms"`
		MetadataLoadMs int    `json:"metadata_load_ms"`
		LoadedDataMs   int    `json:"loaded_data_ms"`
		CanPlayMs      int    `json:"can_play_ms"`
		PlayingMs      int    `json:"playing_ms"`
		FirstFrameMs   int    `json:"first_frame_ms"`
		WaitingCount   int    `json:"waiting_count"`
		StalledCount   int    `json:"stalled_count"`
		SeekingCount   int    `json:"seeking_count"`
		LastSeekMs     int    `json:"last_seek_ms"`
		ErrorCode      int    `json:"error_code"`
		Preloaded      bool   `json:"preloaded"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	severity := "info"
	if body.LoadTimeMs > 1000 {
		severity = "warning"
	}
	detail := map[string]any{
		"action":           body.Action,
		"play_session_id":  body.PlaySessionID,
		"work_id":          body.WorkID,
		"media_id":         body.MediaID,
		"file_name":        body.FileName,
		"load_time_ms":     body.LoadTimeMs,
		"metadata_load_ms": body.MetadataLoadMs,
		"loaded_data_ms":   body.LoadedDataMs,
		"can_play_ms":      body.CanPlayMs,
		"playing_ms":       body.PlayingMs,
		"first_frame_ms":   body.FirstFrameMs,
		"waiting_count":    body.WaitingCount,
		"stalled_count":    body.StalledCount,
		"seeking_count":    body.SeekingCount,
		"last_seek_ms":     body.LastSeekMs,
		"error_code":       body.ErrorCode,
		"preloaded":        body.Preloaded,
	}
	message := playerTimingMessage("前端播放器", body.FileName, body.LoadTimeMs, body.MetadataLoadMs, body.CanPlayMs, body.FirstFrameMs)
	_ = a.currentStore().RecordSystemEvent(SystemEventInput{
		EventType: "player_load",
		Severity:  severity,
		Message:   message,
		Detail:    detail,
	})
	_ = a.currentStore().RecordAuditEvent(AuditEventInput{
		ActorUserID: user.ID,
		EventType:   "player_load",
		TargetType:  "media",
		TargetID:    body.MediaID,
		Message:     message,
		Detail:      detail,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func playerTimingMessage(prefix, fileName string, totalMs, metadataMs, canPlayMs, firstFrameMs int) string {
	name := strings.TrimSpace(fileName)
	if name == "" {
		name = "未知视频"
	}
	parts := []string{prefix + " " + name}
	if totalMs > 0 {
		parts = append(parts, "总耗时 "+strconv.Itoa(totalMs)+"ms")
	}
	if metadataMs > 0 {
		parts = append(parts, "metadata "+strconv.Itoa(metadataMs)+"ms")
	}
	if canPlayMs > 0 {
		parts = append(parts, "canplay "+strconv.Itoa(canPlayMs)+"ms")
	}
	if firstFrameMs > 0 {
		parts = append(parts, "首帧 "+strconv.Itoa(firstFrameMs)+"ms")
	}
	return strings.Join(parts, " · ")
}

func formatPlayerLoadMsg(ms int) string {
	if ms <= 0 {
		return "加载完成"
	}
	if ms < 300 {
		return "快速加载"
	}
	if ms < 1000 {
		return "加载较慢"
	}
	return "加载缓慢"
}

func (a *App) handleSystemLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperAdmin(w, r); !ok {
		return
	}
	query := logQueryFromRequest(r)
	page, err := a.currentStore().SystemEvents(query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func logQueryFromRequest(r *http.Request) LogQuery {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	userID, _ := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	return LogQuery{
		Page:      page,
		PageSize:  pageSize,
		UserID:    userID,
		EventType: r.URL.Query().Get("event_type"),
		Keyword:   r.URL.Query().Get("keyword"),
	}
}

func (a *App) handleFSRoots(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"roots": a.browser.Roots()})
}

func (a *App) handleFSList(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	entries, err := a.browser.List(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (a *App) handleDatabaseConfig(w http.ResponseWriter, r *http.Request) {
	info, err := a.databaseConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"database": info})
}

func (a *App) handleUpdateDatabaseConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Directory string `json:"directory"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	info, err := InspectDatabaseDirectory(body.Directory)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !info.Writable {
		writeError(w, http.StatusBadRequest, "directory is not writable")
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.scanner.Status().Running {
		writeError(w, http.StatusConflict, "cannot switch database while scan is running")
		return
	}
	if filepath.Clean(info.Path) == filepath.Clean(a.databasePath) {
		writeJSON(w, http.StatusOK, map[string]any{"database": info})
		return
	}
	store, err := OpenStore(info.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := SaveConfig(a.configPath, AppConfig{DatabaseDir: info.Directory}); err != nil {
		_ = store.Close()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	oldStore := a.store
	oldUpdater := a.updater
	a.store = store
	a.scanner = NewScanner(store, a.hub)
	a.updater = NewUpdateManager(store, a.hub, nil)
	a.databaseDir = info.Directory
	a.databasePath = info.Path
	if oldUpdater != nil {
		oldUpdater.Close()
	}
	_ = oldStore.Close()
	info.Exists = true
	writeJSON(w, http.StatusOK, map[string]any{"database": info})
}

func (a *App) handleDirectories(w http.ResponseWriter, r *http.Request) {
	store := a.currentStore()
	dirs, err := store.Directories()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"directories": dirs})
}

func (a *App) handleCreateDirectory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := a.browser.Validate(body.Path); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	store := a.currentStore()
	dir, err := store.SaveDirectory(body.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"directory": dir})
}

func (a *App) handleDeleteDirectory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	store := a.currentStore()
	if err := store.DeleteDirectory(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleScanDirectory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	store, scanner := a.currentStoreAndScanner()
	if _, err := store.Directory(id); err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, sql.ErrNoRows) {
			status = http.StatusInternalServerError
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": scanner.Enqueue(id)})
}

func (a *App) handleScanAll(w http.ResponseWriter, r *http.Request) {
	store, scanner := a.currentStoreAndScanner()
	dirs, err := store.Directories()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ids := make([]int64, 0, len(dirs))
	for _, dir := range dirs {
		ids = append(ids, dir.ID)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": scanner.Enqueue(ids...)})
}

func (a *App) handleScanStatus(w http.ResponseWriter, r *http.Request) {
	_, scanner := a.currentStoreAndScanner()
	writeJSON(w, http.StatusOK, map[string]any{"status": scanner.Status()})
}

func (a *App) handleScanEvents(w http.ResponseWriter, r *http.Request) {
	a.streamEvents(w, r, "scan")
}

func (a *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	events := []string{"scan"}
	if user.CanUpdate {
		events = append(events, "update")
	}
	a.streamEvents(w, r, events...)
}

func (a *App) streamEvents(w http.ResponseWriter, r *http.Request, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()
	hub := a.currentHub()
	ch, unsubscribe := hub.Subscribe(events...)
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-ch:
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Name, event.Payload)
			flusher.Flush()
		}
	}
}

func (a *App) handleCreators(w http.ResponseWriter, r *http.Request) {
	store := a.currentStore()
	viewUser, _ := a.currentViewUser(r)
	search := r.URL.Query().Get("search")

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	minWorkCount, _ := strconv.Atoi(r.URL.Query().Get("min_work_count"))
	sortField := r.URL.Query().Get("sort_field")
	sortDirection := r.URL.Query().Get("sort_direction")

	if sortField == "" {
		sortField = "updated"
	}
	if sortDirection == "" {
		sortDirection = "desc"
	}

	if page > 0 || pageSize > 0 {
		creators, total, totalPages, err := store.CreatorsForUserPage(viewUser.ID, search, minWorkCount, sortField, sortDirection, page, pageSize)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"creators":    creators,
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		})
		return
	}

	creators, err := store.CreatorsForUser(viewUser.ID, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"creators": creators})
}

func (a *App) handleCreatorAvatar(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	path, err := a.currentStore().CreatorAvatarFile(id)
	if err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, os.ErrNotExist) {
			status = http.StatusInternalServerError
		}
		writeError(w, status, err.Error())
		return
	}
	http.ServeFile(w, r, path)
}

func (a *App) handleCreatorLinks(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	links, err := a.currentStore().CreatorLinks(id)
	if err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, sql.ErrNoRows) {
			status = http.StatusInternalServerError
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, links)
}

func (a *App) handleSaveCreatorLinks(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	var body struct {
		CreatorIDs []int64 `json:"creator_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := a.currentStore().SetCreatorLinks(id, body.CreatorIDs); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	links, err := a.currentStore().CreatorLinks(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, links)
}

func (a *App) handleScanCreator(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	store, scanner := a.currentStoreAndScanner()
	if _, err := store.Creator(id); err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, sql.ErrNoRows) {
			status = http.StatusInternalServerError
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": scanner.EnqueueCreator(id)})
}

func (a *App) handlePreviewBulkImportCreators(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	var body struct {
		DirectoryID int64  `json:"directory_id"`
		Text        string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.DirectoryID <= 0 {
		writeError(w, http.StatusBadRequest, "directory_id is required")
		return
	}
	store := a.currentStore()
	if _, err := store.Directory(body.DirectoryID); err != nil {
		status := http.StatusBadRequest
		if !errors.Is(err, sql.ErrNoRows) {
			status = http.StatusInternalServerError
		}
		writeError(w, status, err.Error())
		return
	}
	items := PreviewBulkImportCreators(r.Context(), store, body.DirectoryID, body.Text, nil)
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleBulkImportCreators(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	var body struct {
		DirectoryID int64    `json:"directory_id"`
		ProfileURLs []string `json:"profile_urls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.DirectoryID <= 0 {
		writeError(w, http.StatusBadRequest, "directory_id is required")
		return
	}
	creators, err := a.currentStore().ImportCreatorPlaceholders(body.DirectoryID, body.ProfileURLs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"creators": creators})
}

func (a *App) handleFeed(w http.ResponseWriter, r *http.Request) {
	requestStart := time.Now()
	user, _ := a.currentUser(r)
	creatorID, _ := strconv.ParseInt(r.URL.Query().Get("creator_id"), 10, 64)
	directoryID, _ := strconv.ParseInt(r.URL.Query().Get("directory_id"), 10, 64)
	favoriteFolderID, _ := strconv.ParseInt(r.URL.Query().Get("favorite_folder_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	store := a.currentStore()
	viewUser, _ := a.currentViewUser(r)
	queryStart := time.Now()
	works, err := store.FeedWithOptions(FeedOptions{UserID: viewUser.ID, CreatorID: creatorID, DirectoryID: directoryID, FavoriteFolderID: favoriteFolderID, Search: r.URL.Query().Get("search"), Limit: limit})
	queryMs := elapsedMillis(queryStart)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonStart := time.Now()
	writeJSON(w, http.StatusOK, map[string]any{"works": works})
	jsonMs := elapsedMillis(jsonStart)
	totalMs := elapsedMillis(requestStart)
	detail := map[string]any{
		"creator_id":         creatorID,
		"directory_id":       directoryID,
		"favorite_folder_id": favoriteFolderID,
		"search":             r.URL.Query().Get("search"),
		"limit":              limit,
		"works_count":        len(works),
		"query_ms":           queryMs,
		"json_ms":            jsonMs,
		"total_ms":           totalMs,
	}
	severity := "info"
	if totalMs > 1000 {
		severity = "warning"
	}
	_ = store.RecordSystemEvent(SystemEventInput{
		EventType: "feed_load",
		Severity:  severity,
		Message:   fmt.Sprintf("Feed 加载 · %d 个作品 · 总耗时 %dms · 查询 %dms", len(works), totalMs, queryMs),
		Detail:    detail,
	})
	_ = store.RecordAuditEvent(AuditEventInput{
		ActorUserID: user.ID,
		EventType:   "feed_load",
		TargetType:  "feed",
		Message:     fmt.Sprintf("Feed 加载 · %d 个作品 · 总耗时 %dms", len(works), totalMs),
		Detail:      detail,
	})
}

func (a *App) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": a.updater.Status()})
}

func (a *App) handleUpdateJobs(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	jobs, err := a.currentStore().UpdateJobsPage(page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jobs":        jobs.Jobs,
		"page":        jobs.Page,
		"page_size":   jobs.PageSize,
		"total":       jobs.Total,
		"total_pages": jobs.TotalPages,
	})
}

func (a *App) handleRetryUpdateJob(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUpdater(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	status, job, retried, err := a.updater.RetryJobForUser(id, user.ID)
	if err != nil {
		writeError(w, updateErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": status, "job": job, "retried": retried})
}

func (a *App) handleDeleteUpdateJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	if err := a.currentStore().DeleteUpdateJob(id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "update job not found")
			return
		}
		if strings.Contains(err.Error(), "running update job") {
			writeError(w, http.StatusConflict, "运行中的任务不能删除")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleUpdateEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	a.streamEvents(w, r, "update")
}

func (a *App) handleUpdateImmediate(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUpdater(w, r)
	if !ok {
		return
	}
	var body struct {
		CreatorIDs []int64 `json:"creator_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	status, err := a.updater.EnqueueImmediateForUser(body.CreatorIDs, 0, user.ID)
	if err != nil {
		writeError(w, updateErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": status})
}

func (a *App) handleResetUpdateImmediate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	status, err := a.updater.ResetImmediate(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status})
}

func (a *App) handleUpdateQueues(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	queues, err := a.currentStore().UpdateQueues()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queues": queues})
}

func (a *App) handleCreateUpdateQueue(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
		Cron string `json:"cron"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	queue, err := a.currentStore().CreateUpdateQueue(body.Name, body.Cron)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"queue": queue})
}

func (a *App) handlePatchUpdateQueue(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	var body struct {
		Name      string `json:"name"`
		Cron      string `json:"cron"`
		IsDefault *bool  `json:"is_default"`
		Enabled   *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	queue, err := a.currentStore().UpdateUpdateQueue(id, body.Name, body.Cron, body.IsDefault, body.Enabled)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queue": queue})
}

func (a *App) handleDeleteUpdateQueue(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	if err := a.currentStore().DeleteUpdateQueue(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleUpdateQueueCreators(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	creators, err := a.currentStore().CreatorsForUpdateQueue(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"creators": creators})
}

func (a *App) handleAddUpdateQueueCreators(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	var body struct {
		CreatorIDs []int64 `json:"creator_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := a.currentStore().AddCreatorsToUpdateQueue(id, body.CreatorIDs); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleRemoveUpdateQueueCreator(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUpdater(w, r); !ok {
		return
	}
	queueID, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	creatorID, ok := parseID(w, r.PathValue("creator_id"))
	if !ok {
		return
	}
	if err := a.currentStore().RemoveCreatorFromUpdateQueue(queueID, creatorID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleRunUpdateQueueNow(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUpdater(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	run, err := a.updater.EnqueueQueueNowForUser(id, user.ID)
	if err != nil {
		writeError(w, updateErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run": run, "status": a.updater.Status()})
}

func (a *App) handleUpdateAuth(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUpdater(w, r)
	if !ok {
		return
	}
	auth, err := a.currentStore().UpdateAuthForUser(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"auth": auth})
}

func (a *App) handleSaveUpdateAuth(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUpdater(w, r)
	if !ok {
		return
	}
	var body struct {
		AuthToken             *string `json:"auth_token"`
		CT0                   *string `json:"ct0"`
		CookieHeader          *string `json:"cookie_header"`
		Proxy                 *string `json:"proxy"`
		DownloadMode          *string `json:"download_mode"`
		DownloadDirectory     *string `json:"download_directory"`
		DownloaderConcurrency *int    `json:"downloader_concurrency"`
		FetchLimit            *int    `json:"fetch_limit"`
		MediaOnly             *bool   `json:"media_only"`
		IncludeRetweets       *bool   `json:"include_retweets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.DownloadDirectory != nil && strings.TrimSpace(*body.DownloadDirectory) != "" {
		if err := a.browser.Validate(strings.TrimSpace(*body.DownloadDirectory)); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := ensureWritableDirectory(strings.TrimSpace(*body.DownloadDirectory)); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	err := a.currentStore().SaveUpdateAuthForUser(user.ID, UpdateAuthInput{
		AuthToken: body.AuthToken, CT0: body.CT0, CookieHeader: body.CookieHeader, Proxy: body.Proxy,
		DownloadMode: body.DownloadMode, DownloadDirectory: body.DownloadDirectory,
		DownloaderConcurrency: body.DownloaderConcurrency,
		FetchLimit:            body.FetchLimit, MediaOnly: body.MediaOnly, IncludeRetweets: body.IncludeRetweets,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	auth, _ := a.currentStore().UpdateAuthForUser(user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"auth": auth})
}

func (a *App) handleValidateUpdateAuth(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUpdater(w, r)
	if !ok {
		return
	}
	valid := true
	if err := a.updater.ValidateAuthForUser(r.Context(), user.ID); err != nil {
		valid = false
	}
	auth, _ := a.currentStore().UpdateAuthForUser(user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"auth": auth, "ok": valid})
}

func (a *App) handleFavoriteFolders(w http.ResponseWriter, r *http.Request) {
	viewUser, _ := a.currentViewUser(r)
	folders, err := a.currentStore().FavoriteFoldersForUser(viewUser.ID, r.URL.Query().Get("type"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"folders": folders})
}

func (a *App) handleCreateFavoriteFolder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	viewUser, _ := a.currentViewUser(r)
	folder, err := a.currentStore().CreateFavoriteFolderForUser(viewUser.ID, body.Type, body.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"folder": folder})
}

func (a *App) handleUpdateFavoriteFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	var body struct {
		Name      string `json:"name"`
		IsDefault *bool  `json:"is_default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	folder, err := a.currentStore().UpdateFavoriteFolder(id, body.Name, body.IsDefault)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"folder": folder})
}

func (a *App) handleFavoriteStatus(w http.ResponseWriter, r *http.Request) {
	workID, _ := strconv.ParseInt(r.URL.Query().Get("work_id"), 10, 64)
	creatorID, _ := strconv.ParseInt(r.URL.Query().Get("creator_id"), 10, 64)
	viewUser, _ := a.currentViewUser(r)
	status, err := a.currentStore().FavoriteStatusForUser(viewUser.ID, workID, creatorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status})
}

func (a *App) handleFavoriteWorks(w http.ResponseWriter, r *http.Request) {
	folderID, _ := strconv.ParseInt(r.URL.Query().Get("folder_id"), 10, 64)
	viewUser, _ := a.currentViewUser(r)
	works, err := a.currentStore().FavoriteWorksForUser(viewUser.ID, folderID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"works": works})
}

func (a *App) handleAddWorkFavorite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkID   int64 `json:"work_id"`
		FolderID int64 `json:"folder_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	viewUser, _ := a.currentViewUser(r)
	if err := a.currentStore().AddWorkFavoriteForUser(viewUser.ID, body.WorkID, body.FolderID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, _ := a.currentStore().FavoriteStatusForUser(viewUser.ID, body.WorkID, 0)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": status})
}

func (a *App) handleRemoveWorkFavorite(w http.ResponseWriter, r *http.Request) {
	workID, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	folderID, _ := strconv.ParseInt(r.URL.Query().Get("folder_id"), 10, 64)
	viewUser, _ := a.currentViewUser(r)
	if err := a.currentStore().RemoveWorkFavoriteForUser(viewUser.ID, workID, folderID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, _ := a.currentStore().FavoriteStatusForUser(viewUser.ID, workID, 0)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": status})
}

func (a *App) handleFavoriteCreators(w http.ResponseWriter, r *http.Request) {
	folderID, _ := strconv.ParseInt(r.URL.Query().Get("folder_id"), 10, 64)
	viewUser, _ := a.currentViewUser(r)
	creators, err := a.currentStore().FavoriteCreatorsForUser(viewUser.ID, folderID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"creators": creators})
}

func (a *App) handleAddCreatorFavorite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CreatorID int64 `json:"creator_id"`
		FolderID  int64 `json:"folder_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	viewUser, _ := a.currentViewUser(r)
	if err := a.currentStore().AddCreatorFavoriteForUser(viewUser.ID, body.CreatorID, body.FolderID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, _ := a.currentStore().FavoriteStatusForUser(viewUser.ID, 0, body.CreatorID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": status})
}

func (a *App) handleRemoveCreatorFavorite(w http.ResponseWriter, r *http.Request) {
	creatorID, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	folderID, _ := strconv.ParseInt(r.URL.Query().Get("folder_id"), 10, 64)
	viewUser, _ := a.currentViewUser(r)
	if err := a.currentStore().RemoveCreatorFavoriteForUser(viewUser.ID, creatorID, folderID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, _ := a.currentStore().FavoriteStatusForUser(viewUser.ID, 0, creatorID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": status})
}

func updateErrorStatus(err error) int {
	if errors.Is(err, ErrAuthRequired) {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadRequest
}

func (a *App) handleWork(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	store := a.currentStore()
	work, err := store.Work(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"work": work})
}

func (a *App) handleRecordWorkView(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	viewUser, ok := a.currentViewUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	if err := a.currentStore().RecordWorkView(viewUser.ID, id); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleWorkCover(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	path, name, err := a.currentStore().WorkCoverPath(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func (a *App) handleMedia(w http.ResponseWriter, r *http.Request) {
	user, _ := a.currentUser(r)
	requestStart := time.Now()
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	store := a.currentStore()
	lookupStart := time.Now()
	path, name, mediaType, err := store.MediaPath(id)
	lookupMs := elapsedMillis(lookupStart)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	openStart := time.Now()
	file, err := os.Open(path)
	openMs := elapsedMillis(openStart)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	defer file.Close()
	statStart := time.Now()
	info, err := file.Stat()
	statMs := elapsedMillis(statStart)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	headerStart := time.Now()
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if mediaType == "video" {
		w.Header().Set("Accept-Ranges", "bytes")
	}
	w.Header().Set("Content-Type", contentType)
	headerMs := elapsedMillis(headerStart)
	measuredFile := &timedReadSeeker{inner: file}
	measuredWriter := &timedResponseWriter{ResponseWriter: w}
	http.ServeContent(measuredWriter, r, name, info.ModTime(), measuredFile)
	totalMs := elapsedMillis(requestStart)
	firstReadMs := sinceOrZero(requestStart, measuredFile.firstReadAt)
	firstWriteMs := sinceOrZero(requestStart, measuredWriter.firstWriteAt)
	status := measuredWriter.status
	if status == 0 {
		status = http.StatusOK
	}
	if mediaType == "video" {
		rangeStart, rangeEnd, rangeSize := parseRangeSummary(r.Header.Get("Range"), info.Size())
		detail := map[string]any{
			"media_id":          id,
			"play_session_id":   r.URL.Query().Get("play_session_id"),
			"file_name":         name,
			"media_type":        mediaType,
			"content_type":      contentType,
			"range_header":      r.Header.Get("Range"),
			"range_start":       rangeStart,
			"range_end":         rangeEnd,
			"range_size":        rangeSize,
			"status":            status,
			"file_size":         info.Size(),
			"bytes_sent":        measuredWriter.bytesWritten,
			"bytes_read":        measuredFile.bytesRead,
			"sent_ratio":        ratioFloat(measuredWriter.bytesWritten, info.Size()),
			"lookup_ms":         lookupMs,
			"open_ms":           openMs,
			"stat_ms":           statMs,
			"header_ms":         headerMs,
			"first_read_ms":     firstReadMs,
			"first_write_ms":    firstWriteMs,
			"disk_read_ms":      measuredFile.readMillis(),
			"read_count":        measuredFile.readCount,
			"read_wait_max_ms":  millis(measuredFile.maxReadDuration),
			"read_slow_count":   measuredFile.slowReadCount,
			"write_ms":          measuredWriter.writeMillis(),
			"write_count":       measuredWriter.writeCount,
			"write_wait_max_ms": millis(measuredWriter.maxWriteDuration),
			"write_slow_count":  measuredWriter.slowWriteCount,
			"client_aborted":    isClientAbort(measuredWriter.writeErr),
			"completed":         measuredWriter.writeErr == nil,
			"error":             errorString(measuredWriter.writeErr),
			"total_ms":          totalMs,
		}
		severity := "info"
		if totalMs > 1000 || firstWriteMs > 500 || measuredWriter.maxWriteDuration > 500*time.Millisecond || measuredFile.maxReadDuration > 500*time.Millisecond {
			severity = "warning"
		}
		message := fmt.Sprintf("视频请求 %s · 总耗时 %dms · 首字节 %dms · 发送 %s", name, totalMs, firstWriteMs, formatBytes(measuredWriter.bytesWritten))
		_ = store.RecordAuditEvent(AuditEventInput{
			ActorUserID: user.ID,
			EventType:   "video_stream",
			TargetType:  "media",
			TargetID:    id,
			Message:     message,
			Detail:      detail,
		})
		_ = store.RecordSystemEvent(SystemEventInput{
			EventType: "video_stream",
			Severity:  severity,
			Message:   message,
			Detail:    detail,
		})
	}
}

type timedResponseWriter struct {
	http.ResponseWriter
	status           int
	bytesWritten     int64
	firstWriteAt     time.Time
	writeDuration    time.Duration
	maxWriteDuration time.Duration
	writeCount       int
	slowWriteCount   int
	writeErr         error
}

func (w *timedResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *timedResponseWriter) Write(p []byte) (int, error) {
	if w.firstWriteAt.IsZero() {
		w.firstWriteAt = time.Now()
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	start := time.Now()
	n, err := w.ResponseWriter.Write(p)
	duration := time.Since(start)
	w.writeDuration += duration
	w.writeCount++
	if duration > w.maxWriteDuration {
		w.maxWriteDuration = duration
	}
	if duration > 20*time.Millisecond {
		w.slowWriteCount++
	}
	if err != nil && w.writeErr == nil {
		w.writeErr = err
	}
	w.bytesWritten += int64(n)
	return n, err
}

func (w *timedResponseWriter) writeMillis() int {
	return millis(w.writeDuration)
}

type timedReadSeeker struct {
	inner           io.ReadSeeker
	bytesRead       int64
	readDuration    time.Duration
	maxReadDuration time.Duration
	readCount       int
	slowReadCount   int
	firstReadAt     time.Time
}

func (r *timedReadSeeker) Read(p []byte) (int, error) {
	if r.firstReadAt.IsZero() {
		r.firstReadAt = time.Now()
	}
	start := time.Now()
	n, err := r.inner.Read(p)
	duration := time.Since(start)
	r.readDuration += duration
	r.readCount++
	if duration > r.maxReadDuration {
		r.maxReadDuration = duration
	}
	if duration > 20*time.Millisecond {
		r.slowReadCount++
	}
	r.bytesRead += int64(n)
	return n, err
}

func (r *timedReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return r.inner.Seek(offset, whence)
}

func (r *timedReadSeeker) readMillis() int {
	return millis(r.readDuration)
}

func parseRangeSummary(header string, fileSize int64) (int64, int64, int64) {
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, 0
	}
	value := strings.TrimPrefix(header, "bytes=")
	if strings.Contains(value, ",") {
		value = strings.Split(value, ",")[0]
	}
	parts := strings.SplitN(value, "-", 2)
	if len(parts) != 2 {
		return 0, 0, 0
	}
	var start, end int64
	var err error
	if strings.TrimSpace(parts[0]) == "" {
		suffix, suffixErr := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if suffixErr != nil || suffix <= 0 || fileSize <= 0 {
			return 0, 0, 0
		}
		if suffix > fileSize {
			suffix = fileSize
		}
		start = fileSize - suffix
		end = fileSize - 1
		return start, end, end - start + 1
	}
	start, err = strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || start < 0 {
		return 0, 0, 0
	}
	if strings.TrimSpace(parts[1]) == "" {
		end = fileSize - 1
	} else {
		end, err = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return 0, 0, 0
		}
	}
	if fileSize > 0 && end >= fileSize {
		end = fileSize - 1
	}
	if end < start {
		return start, end, 0
	}
	return start, end, end - start + 1
}

func ratioFloat(value, total int64) float64 {
	if value <= 0 || total <= 0 {
		return 0
	}
	return float64(value) / float64(total)
}

func millis(duration time.Duration) int {
	return int(duration / time.Millisecond)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func isClientAbort(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset") || strings.Contains(msg, "aborted")
}

func elapsedMillis(start time.Time) int {
	return int(time.Since(start) / time.Millisecond)
}

func sinceOrZero(start, end time.Time) int {
	if end.IsZero() {
		return 0
	}
	return int(end.Sub(start) / time.Millisecond)
}

func formatBytes(value int64) string {
	if value < 1024 {
		return strconv.FormatInt(value, 10) + " B"
	}
	if value < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(value)/1024)
	}
	if value < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(value)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(value)/(1024*1024*1024))
}

func (a *App) handleWeb(w http.ResponseWriter, r *http.Request) {
	cleanPath := strings.TrimPrefix(filepath.Clean(r.URL.Path), string(filepath.Separator))
	if a.web.Dir != "" {
		path := filepath.Join(a.web.Dir, cleanPath)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		index := filepath.Join(a.web.Dir, "index.html")
		if _, err := os.Stat(index); err == nil {
			http.ServeFile(w, r, index)
			return
		}
	}
	if a.web.FS != nil {
		if cleanPath != "." && cleanPath != "" {
			if file, err := a.web.FS.Open(cleanPath); err == nil {
				defer file.Close()
				if info, err := file.Stat(); err == nil && !info.IsDir() {
					http.ServeContent(w, r, cleanPath, info.ModTime(), file.(io.ReadSeeker))
					return
				}
			} else if strings.HasPrefix(cleanPath, "assets/") {
				http.NotFound(w, r)
				return
			}
		}
		if data, err := fs.ReadFile(a.web.FS, "index.html"); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
	}
	if a.web.Dir == "" && a.web.FS == nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "LocalTwitter API is running. Build web assets with npm run build in web/."})
		return
	}
	http.NotFound(w, r)
}

func (a *App) currentStore() *Store {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.store
}

func (a *App) currentHub() *EventHub {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.hub
}

func (a *App) currentStoreAndScanner() (*Store, *Scanner) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.store, a.scanner
}

func (a *App) databaseConfig() (DatabaseConfig, error) {
	a.mu.RLock()
	dir := a.databaseDir
	path := a.databasePath
	a.mu.RUnlock()
	info, err := InspectDatabaseDirectory(dir)
	if err != nil {
		return DatabaseConfig{}, err
	}
	if path != "" {
		info.Path = path
		if _, err := os.Stat(path); err == nil {
			info.Exists = true
		}
	}
	return info, nil
}

func parseID(w http.ResponseWriter, raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logging(next http.Handler) http.Handler {
	return newAccessLoggerFromEnv().middleware(next)
}
