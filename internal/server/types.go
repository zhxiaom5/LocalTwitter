package server

import "time"

type Directory struct {
	ID            int64      `json:"id"`
	Path          string     `json:"path"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	CreatorCount  int64      `json:"creator_count"`
	WorkCount     int64      `json:"work_count"`
	LastScannedAt *time.Time `json:"last_scanned_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Creator struct {
	ID                   int64            `json:"id"`
	DirectoryID          int64            `json:"directory_id"`
	Name                 string           `json:"name"`
	WorkCount            int64            `json:"work_count"`
	TwitterUserID        string           `json:"twitter_user_id"`
	TwitterUsername      string           `json:"twitter_username"`
	TwitterProfileURL    string           `json:"twitter_profile_url"`
	OnlineIdentityStatus string           `json:"online_identity_status"`
	AvatarURL            string           `json:"avatar_url"`
	AvatarFile           string           `json:"avatar_file,omitempty"`
	Bio                  string           `json:"bio"`
	ProfileUpdatedAt     string           `json:"profile_updated_at,omitempty"`
	ViewCount            int64            `json:"view_count"`
	LastWorkCreatedAt    string           `json:"last_work_created_at,omitempty"`
	LinkedCreators       []CreatorSummary `json:"linked_creators,omitempty"`
}

type CreatorSummary struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	TwitterUsername string `json:"twitter_username,omitempty"`
	AvatarURL       string `json:"avatar_url,omitempty"`
}

type Media struct {
	ID       int64  `json:"id"`
	WorkID   int64  `json:"work_id"`
	Type     string `json:"type"`
	FileName string `json:"file_name"`
	URL      string `json:"url"`
	Ordinal  int    `json:"ordinal"`
}

type Work struct {
	ID               int64            `json:"id"`
	DirectoryID      int64            `json:"directory_id"`
	CreatorID        int64            `json:"creator_id"`
	CreatorName      string           `json:"creator_name"`
	CreatorAvatarURL string           `json:"creator_avatar_url,omitempty"`
	CreatorBio       string           `json:"creator_bio,omitempty"`
	LinkedCreators   []CreatorSummary `json:"linked_creators,omitempty"`
	AwemeID          string           `json:"aweme_id"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	MusicTitle       string           `json:"music_title"`
	SourceURL        string           `json:"source_url"`
	CoverURL         string           `json:"cover_url,omitempty"`
	CoverFile        string           `json:"cover_file,omitempty"`
	Tags             []string         `json:"tags"`
	PublishedAt      string           `json:"published_at"`
	CreatedAt        time.Time        `json:"created_at"`
	Media            []Media          `json:"media"`
}

type ScanStatus struct {
	Running            bool     `json:"running"`
	QueueLength        int      `json:"queue_length"`
	CurrentTaskID      string   `json:"current_task_id,omitempty"`
	CurrentDirectoryID int64    `json:"current_directory_id,omitempty"`
	CurrentDirectory   string   `json:"current_directory,omitempty"`
	CurrentCreator     string   `json:"current_creator,omitempty"`
	ScannedCreators    int      `json:"scanned_creators"`
	TotalCreators      int      `json:"total_creators"`
	ScannedWorks       int      `json:"scanned_works"`
	Progress           int      `json:"progress"`
	State              string   `json:"state"`
	Errors             []string `json:"errors"`
}

type ScanEvent struct {
	Type    string     `json:"type"`
	Status  ScanStatus `json:"status"`
	Creator *Creator   `json:"creator,omitempty"`
}

type FavoriteFolder struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type FavoriteStatus struct {
	WorkFolderIDs    []int64 `json:"work_folder_ids"`
	CreatorFolderIDs []int64 `json:"creator_folder_ids"`
	WorkFavorited    bool    `json:"work_favorited"`
	CreatorFavorited bool    `json:"creator_favorited"`
}

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Role         string    `json:"role"`
	IsAdmin      bool      `json:"is_admin"`
	IsSuperAdmin bool      `json:"is_super_admin"`
	CanUpdate    bool      `json:"can_update"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type UpdateQueue struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Cron            string    `json:"cron"`
	IsDefault       bool      `json:"is_default"`
	Enabled         bool      `json:"enabled"`
	NextRunAt       string    `json:"next_run_at"`
	LastRunSummary  string    `json:"last_run_summary"`
	SubscribedCount int64     `json:"subscribed_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type UpdateRun struct {
	ID                int64     `json:"id"`
	QueueID           int64     `json:"queue_id"`
	QueueName         string    `json:"queue_name"`
	EnqueuedAt        time.Time `json:"enqueued_at"`
	TotalCreators     int       `json:"total_creators"`
	SucceededCreators int       `json:"succeeded_creators"`
	FailedCreators    int       `json:"failed_creators"`
	AddedWorks        int       `json:"added_works"`
	Status            string    `json:"status"`
	Summary           string    `json:"summary"`
}

type UpdateJob struct {
	ID                int64     `json:"id"`
	CreatorID         int64     `json:"creator_id"`
	CreatorName       string    `json:"creator_name"`
	ActorUserID       int64     `json:"actor_user_id,omitempty"`
	ActorUsername     string    `json:"actor_username,omitempty"`
	CookieOwnerUserID int64     `json:"cookie_owner_user_id,omitempty"`
	CookieOwnerName   string    `json:"cookie_owner_name,omitempty"`
	QueueID           int64     `json:"queue_id,omitempty"`
	RunID             int64     `json:"run_id,omitempty"`
	RetryOfJobID      int64     `json:"retry_of_job_id,omitempty"`
	Attempt           int       `json:"attempt"`
	Source            string    `json:"source"`
	Status            string    `json:"status"`
	AddedWorks        int       `json:"added_works"`
	TotalTweets       int       `json:"total_tweets"`
	ProcessedTweets   int       `json:"processed_tweets"`
	DownloadedVideos  int       `json:"downloaded_videos"`
	SkippedTweets     int       `json:"skipped_tweets"`
	DownloadSpeedBps  int64     `json:"download_speed_bytes_per_second"`
	ProgressMessage   string    `json:"progress_message,omitempty"`
	DownloadDirectory string    `json:"download_directory,omitempty"`
	DownloadNotice    string    `json:"download_notice,omitempty"`
	Error             string    `json:"error,omitempty"`
	ErrorDetail       string    `json:"error_detail,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	StartedAt         string    `json:"started_at,omitempty"`
	FinishedAt        string    `json:"finished_at,omitempty"`
}

type UpdateStatus struct {
	Running           bool        `json:"running"`
	CurrentCreator    string      `json:"current_creator,omitempty"`
	PendingCreators   int         `json:"pending_creators"`
	TotalCreators     int         `json:"total_creators"`
	SucceededCreators int         `json:"succeeded_creators"`
	FailedCreators    int         `json:"failed_creators"`
	AddedWorks        int         `json:"added_works"`
	Progress          int         `json:"progress"`
	State             string      `json:"state"`
	Errors            []string    `json:"errors"`
	RunningJobs       []UpdateJob `json:"running_jobs"`
	Jobs              []UpdateJob `json:"jobs"`
}

type UpdateJobsPage struct {
	Jobs       []UpdateJob `json:"jobs"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	Total      int         `json:"total"`
	TotalPages int         `json:"total_pages"`
}

type UpdateEvent struct {
	Type   string       `json:"type"`
	Status UpdateStatus `json:"status"`
	Job    *UpdateJob   `json:"job,omitempty"`
}

type UpdateAuth struct {
	Configured            bool   `json:"configured"`
	UpdatedAt             string `json:"updated_at,omitempty"`
	ValidationStatus      string `json:"validation_status"`
	ValidationMessage     string `json:"validation_message,omitempty"`
	AuthToken             string `json:"auth_token,omitempty"`
	CT0                   string `json:"ct0,omitempty"`
	CookieHeader          string `json:"cookie_header,omitempty"`
	CookieHeaderSaved     bool   `json:"cookie_header_saved"`
	Proxy                 string `json:"proxy,omitempty"`
	DownloadMode          string `json:"download_mode"`
	DownloadDirectory     string `json:"download_directory,omitempty"`
	DownloaderConcurrency int    `json:"downloader_concurrency"`
	FetchLimit            int    `json:"fetch_limit"`
	MediaOnly             bool   `json:"media_only"`
	IncludeRetweets       bool   `json:"include_retweets"`
}

type ViewContext struct {
	ActorUser User `json:"actor_user"`
	ViewUser  User `json:"view_user"`
}

type LogEvent struct {
	ID          int64     `json:"id"`
	ActorUserID int64     `json:"actor_user_id,omitempty"`
	ActorName   string    `json:"actor_name,omitempty"`
	EventType   string    `json:"event_type"`
	TargetType  string    `json:"target_type,omitempty"`
	TargetID    int64     `json:"target_id,omitempty"`
	Severity    string    `json:"severity,omitempty"`
	Message     string    `json:"message"`
	DetailJSON  string    `json:"detail_json,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type LogEventsPage struct {
	Events     []LogEvent `json:"events"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	Total      int        `json:"total"`
	TotalPages int        `json:"total_pages"`
}

type AboutInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Commit      string `json:"commit,omitempty"`
	BuildDate   string `json:"build_date,omitempty"`
	GoVersion   string `json:"go_version,omitempty"`
	Platform    string `json:"platform,omitempty"`
	BuildMode   string `json:"build_mode,omitempty"`
	Description string `json:"description"`
}
