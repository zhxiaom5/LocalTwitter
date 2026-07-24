export type Directory = {
  id: number;
  path: string;
  name: string;
  status: string;
  creator_count: number;
  work_count: number;
  last_scanned_at?: string;
  created_at: string;
};

export type Creator = {
  id: number;
  directory_id: number;
  name: string;
  work_count: number;
  twitter_user_id?: string;
  twitter_username?: string;
  twitter_profile_url?: string;
  online_identity_status?: string;
  avatar_url?: string;
  avatar_file?: string;
  bio?: string;
  profile_updated_at?: string;
  view_count: number;
  last_work_created_at?: string;
  linked_creators?: CreatorSummary[];
};

export type CreatorSummary = {
  id: number;
  name: string;
  twitter_username?: string;
  avatar_url?: string;
};

export type CreatorLinks = {
  linked: Creator[];
  candidates: Creator[];
};

export type BulkImportItem = {
  input: string;
  profile_url?: string;
  username?: string;
  status: string;
  existing: boolean;
  existing_creator_id?: number;
  error?: string;
};

export type Media = {
  id: number;
  work_id: number;
  type: 'video' | 'image' | 'audio';
  file_name: string;
  url: string;
  ordinal: number;
};

export type Work = {
  id: number;
  directory_id: number;
  creator_id: number;
  creator_name: string;
  creator_avatar_url?: string;
  creator_bio?: string;
  linked_creators?: CreatorSummary[];
  aweme_id: string;
  title: string;
  description: string;
  music_title: string;
  source_url: string;
  cover_url?: string;
  cover_file?: string;
  tags: string[] | null;
  published_at: string;
  media: Media[];
};

export type FavoriteFolderType = 'work' | 'creator';

export type FavoriteFolder = {
  id: number;
  user_id: number;
  type: FavoriteFolderType;
  name: string;
  is_default: boolean;
  created_at: string;
  updated_at: string;
};

export type FavoriteStatus = {
  work_folder_ids: number[] | null;
  creator_folder_ids: number[] | null;
  work_favorited: boolean;
  creator_favorited: boolean;
};

export type User = {
  id: number;
  username: string;
  role?: 'super_admin' | 'admin' | string;
  is_admin: boolean;
  is_super_admin?: boolean;
  can_update?: boolean;
  created_at: string;
  updated_at: string;
};

export type ViewContext = {
  actor_user: User;
  view_user: User;
};

export type LogEvent = {
  id: number;
  actor_user_id?: number;
  actor_name?: string;
  event_type: string;
  target_type?: string;
  target_id?: number;
  severity?: string;
  message: string;
  detail_json?: string;
  created_at: string;
};

export type LogEventsPage = {
  events: LogEvent[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
};

export type AboutInfo = {
  name: string;
  version: string;
  commit?: string;
  build_date?: string;
  go_version?: string;
  platform?: string;
  build_mode?: string;
  description: string;
};

export type ScanStatus = {
  running: boolean;
  queue_length: number;
  current_task_id?: string;
  current_directory_id?: number;
  current_directory?: string;
  current_creator?: string;
  scanned_creators: number;
  total_creators: number;
  scanned_works: number;
  progress: number;
  state: string;
  errors: string[] | null;
};

export type ScanEvent = {
  type: string;
  status: ScanStatus;
  creator?: Creator;
};

export type FSEntry = {
  path: string;
  name: string;
};

export type DatabaseConfig = {
  directory: string;
  path: string;
  exists: boolean;
  writable: boolean;
};

export type UpdateQueue = {
  id: number;
  name: string;
  cron: string;
  is_default: boolean;
  enabled: boolean;
  next_run_at: string;
  last_run_summary: string;
  subscribed_count: number;
  created_at: string;
  updated_at: string;
};

export type UpdateJob = {
  id: number;
  creator_id: number;
  creator_name: string;
  actor_user_id?: number;
  actor_username?: string;
  cookie_owner_user_id?: number;
  cookie_owner_name?: string;
  queue_id?: number;
  run_id?: number;
  retry_of_job_id?: number;
  attempt: number;
  source: string;
  status: string;
  added_works: number;
  total_tweets: number;
  processed_tweets: number;
  downloaded_videos: number;
  skipped_tweets: number;
  download_speed_bytes_per_second: number;
  progress_message?: string;
  download_directory?: string;
  download_notice?: string;
  error?: string;
  error_detail?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
};

export type UpdateStatus = {
  running: boolean;
  current_creator?: string;
  pending_creators: number;
  total_creators: number;
  succeeded_creators: number;
  failed_creators: number;
  added_works: number;
  progress: number;
  state: string;
  errors: string[] | null;
  running_jobs: UpdateJob[];
  jobs: UpdateJob[];
};

export type UpdateJobsPage = {
  jobs: UpdateJob[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
};

export type UpdateAuth = {
  configured: boolean;
  updated_at?: string;
  validation_status: string;
  validation_message?: string;
  auth_token?: string;
  ct0?: string;
  cookie_header_saved?: boolean;
  proxy?: string;
  download_mode?: 'original' | 'custom';
  download_directory?: string;
  downloader_concurrency: number;
  fetch_limit: number;
  media_only: boolean;
  include_retweets: boolean;
};


export type CreatorsPage = {
  creators: Creator[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
};
