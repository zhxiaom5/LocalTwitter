import type { AboutInfo, BulkImportItem, Creator, CreatorLinks, DatabaseConfig, Directory, FavoriteFolder, FavoriteFolderType, FavoriteStatus, FSEntry, LogEventsPage, ScanStatus, UpdateAuth, UpdateJob, UpdateJobsPage, UpdateQueue, UpdateStatus, User, ViewContext, Work } from './types';

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
    this.name = 'ApiError';
  }
}

const unauthorizedEventName = 'localtwitter:unauthorized';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(options?.headers ?? {}) },
    ...options,
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({ error: response.statusText }));
    if (response.status === 401 && !path.startsWith('/api/auth/')) {
      window.dispatchEvent(new Event(unauthorizedEventName));
    }
    throw new ApiError(response.status, body.error ?? response.statusText);
  }
  return response.json() as Promise<T>;
}

export const api = {
  me: () => request<{ user: User }>('/api/auth/me'),
  login: (username: string, password: string) =>
    request<{ user: User }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  logout: () => request<{ ok: boolean }>('/api/auth/logout', { method: 'POST' }),
  changePassword: (oldPassword: string, newPassword: string) =>
    request<{ user: User }>('/api/auth/password', {
      method: 'POST',
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
    }),
  users: () => request<{ users: User[] }>('/api/users'),
  createUser: (username: string, password: string) =>
    request<{ user: User }>('/api/users', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  resetUserPassword: (id: number, password: string) =>
    request<{ user: User }>(`/api/users/${id}/reset-password`, {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),
  deleteUser: (id: number) => request<{ ok: boolean }>(`/api/users/${id}`, { method: 'DELETE' }),
  viewContext: () => request<ViewContext>('/api/users/view-context'),
  setViewContext: (userId: number) =>
    request<ViewContext>('/api/users/view-context', {
      method: 'PUT',
      body: JSON.stringify({ user_id: userId }),
    }),
  clearViewContext: () => request<ViewContext>('/api/users/view-context', { method: 'DELETE' }),
  auditLogs: (params: { page?: number; pageSize?: number; userId?: number; eventType?: string; keyword?: string } = {}) => {
    const query = logQuery(params);
    return request<LogEventsPage>(`/api/logs/audit?${query.toString()}`);
  },
  systemLogs: (params: { page?: number; pageSize?: number; eventType?: string; keyword?: string } = {}) => {
    const query = logQuery(params);
    return request<LogEventsPage>(`/api/logs/system?${query.toString()}`);
  },
  about: () => request<{ about: AboutInfo }>('/api/about'),

  roots: () => request<{ roots: FSEntry[] }>('/api/fs/roots'),
  listFS: (path: string) => request<{ entries: FSEntry[] }>(`/api/fs/list?path=${encodeURIComponent(path)}`),
  databaseConfig: () => request<{ database: DatabaseConfig }>('/api/config/database'),
  saveDatabaseDirectory: (directory: string) =>
    request<{ database: DatabaseConfig }>('/api/config/database', {
      method: 'PUT',
      body: JSON.stringify({ directory }),
    }),
  directories: () => request<{ directories: Directory[] }>('/api/directories'),
  saveDirectory: (path: string) =>
    request<{ directory: Directory }>('/api/directories', {
      method: 'POST',
      body: JSON.stringify({ path }),
    }),
  deleteDirectory: (id: number) =>
    request<{ ok: boolean }>(`/api/directories/${id}`, {
      method: 'DELETE',
    }),
  scanDirectory: (id: number) =>
    request<{ status: ScanStatus }>(`/api/directories/${id}/scan`, {
      method: 'POST',
    }),
  scanAll: () =>
    request<{ status: ScanStatus }>('/api/directories/scan-all', {
      method: 'POST',
    }),
  scanStatus: () => request<{ status: ScanStatus }>('/api/scan/status'),
  scanCreator: (id: number) =>
    request<{ status: ScanStatus }>(`/api/creators/${id}/scan`, {
      method: 'POST',
    }),
  creatorLinks: (id: number) => request<CreatorLinks>(`/api/creators/${id}/links`),
  saveCreatorLinks: (id: number, creatorIds: number[]) =>
    request<CreatorLinks>(`/api/creators/${id}/links`, {
      method: 'PUT',
      body: JSON.stringify({ creator_ids: creatorIds }),
    }),
  creators: (params: { search?: string; page?: number; pageSize?: number; sortField?: string; sortDirection?: string; minWorkCount?: number } = {}) => {
    const query = new URLSearchParams();
    if (params.search?.trim()) query.set('search', params.search.trim());
    if (params.page) query.set('page', String(params.page));
    if (params.pageSize) query.set('page_size', String(params.pageSize));
    if (params.sortField) query.set('sort_field', params.sortField);
    if (params.sortDirection) query.set('sort_direction', params.sortDirection);
    if (params.minWorkCount != null && params.minWorkCount > 0) query.set('min_work_count', String(params.minWorkCount));
    return request<{ creators: Creator[]; page: number; page_size: number; total: number; total_pages: number }>(`/api/creators${query.size ? `?${query.toString()}` : ''}`);
  },
  previewBulkImportCreators: (directoryId: number, text: string) =>
    request<{ items: BulkImportItem[] }>('/api/creators/bulk-import/preview', {
      method: 'POST',
      body: JSON.stringify({ directory_id: directoryId, text }),
    }),
  bulkImportCreators: (directoryId: number, profileUrls: string[]) =>
    request<{ creators: Creator[] }>('/api/creators/bulk-import', {
      method: 'POST',
      body: JSON.stringify({ directory_id: directoryId, profile_urls: profileUrls }),
    }),

  updateStatus: () => request<{ status: UpdateStatus }>('/api/update/status'),
  updateJobs: (page = 1, pageSize = 10) => request<UpdateJobsPage>(`/api/update/jobs?page=${page}&page_size=${pageSize}`),
  enqueueImmediateUpdate: (creatorIds: number[]) =>
    request<{ status: UpdateStatus }>('/api/update/immediate', {
      method: 'POST',
      body: JSON.stringify({ creator_ids: creatorIds }),
    }),
  resetImmediateUpdate: () => request<{ status: UpdateStatus }>('/api/update/immediate/reset', { method: 'POST' }),
  updateQueues: () => request<{ queues: UpdateQueue[] }>('/api/update/queues'),
  createUpdateQueue: (name: string, cron: string) =>
    request<{ queue: UpdateQueue }>('/api/update/queues', {
      method: 'POST',
      body: JSON.stringify({ name, cron }),
    }),
  updateQueue: (id: number, body: { name?: string; cron?: string; is_default?: boolean; enabled?: boolean }) =>
    request<{ queue: UpdateQueue }>(`/api/update/queues/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),
  deleteUpdateQueue: (id: number) => request<{ ok: boolean }>(`/api/update/queues/${id}`, { method: 'DELETE' }),
  updateQueueCreators: (id: number) => request<{ creators: Creator[] }>(`/api/update/queues/${id}/creators`),
  addUpdateQueueCreators: (id: number, creatorIds: number[]) =>
    request<{ ok: boolean }>(`/api/update/queues/${id}/creators`, {
      method: 'POST',
      body: JSON.stringify({ creator_ids: creatorIds }),
    }),
  removeUpdateQueueCreator: (queueId: number, creatorId: number) =>
    request<{ ok: boolean }>(`/api/update/queues/${queueId}/creators/${creatorId}`, { method: 'DELETE' }),
  runUpdateQueueNow: (id: number) => request<{ status: UpdateStatus }>(`/api/update/queues/${id}/run-now`, { method: 'POST' }),
  updateAuth: () => request<{ auth: UpdateAuth }>('/api/update/auth'),
  retryUpdateJob: (id: number) => request<{ status: UpdateStatus; job: UpdateJob; retried: boolean }>(`/api/update/jobs/${id}/retry`, { method: 'POST' }),
  deleteUpdateJob: (id: number) => request<{ ok: boolean }>(`/api/update/jobs/${id}`, { method: 'DELETE' }),
  saveUpdateAuth: (body: { auth_token?: string; ct0?: string; cookie_header?: string; proxy?: string; download_mode?: string; download_directory?: string; downloader_concurrency?: number; fetch_limit?: number; media_only?: boolean; include_retweets?: boolean }) =>
    request<{ auth: UpdateAuth }>('/api/update/auth', {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),
  validateUpdateAuth: () => request<{ auth: UpdateAuth; ok: boolean }>('/api/update/auth/validate', { method: 'POST' }),

  feed: (params: { creatorId?: number; directoryId?: number; favoriteFolderId?: number; search?: string; limit?: number } = {}) => {
    const query = new URLSearchParams();
    query.set('mode', 'random');
    query.set('limit', String(params.limit ?? 30));
    if (params.creatorId) query.set('creator_id', String(params.creatorId));
    if (params.directoryId) query.set('directory_id', String(params.directoryId));
    if (params.favoriteFolderId) query.set('favorite_folder_id', String(params.favoriteFolderId));
    if (params.search?.trim()) query.set('search', params.search.trim());
    return request<{ works: Work[] }>(`/api/feed?${query.toString()}`);
  },
  work: (id: number) => request<{ work: Work }>(`/api/works/${id}`),
  recordWorkView: (id: number) => request<{ ok: boolean }>(`/api/works/${id}/view`, { method: 'POST' }),

  favoriteFolders: (type: FavoriteFolderType) => request<{ folders: FavoriteFolder[] }>(`/api/favorite-folders?type=${type}`),
  createFavoriteFolder: (type: FavoriteFolderType, name: string) =>
    request<{ folder: FavoriteFolder }>('/api/favorite-folders', {
      method: 'POST',
      body: JSON.stringify({ type, name }),
    }),
  updateFavoriteFolder: (id: number, body: { name?: string; is_default?: boolean }) =>
    request<{ folder: FavoriteFolder }>(`/api/favorite-folders/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),
  favoriteStatus: (params: { workId?: number; creatorId?: number }) => {
    const query = new URLSearchParams();
    if (params.workId) query.set('work_id', String(params.workId));
    if (params.creatorId) query.set('creator_id', String(params.creatorId));
    return request<{ status: FavoriteStatus }>(`/api/favorites/status?${query.toString()}`);
  },
  favoriteWorks: (folderId: number) => request<{ works: Work[] }>(`/api/favorites/works?folder_id=${folderId}`),
  addWorkFavorite: (workId: number, folderId?: number) =>
    request<{ ok: boolean; status: FavoriteStatus }>('/api/favorites/works', {
      method: 'POST',
      body: JSON.stringify({ work_id: workId, folder_id: folderId }),
    }),
  removeWorkFavorite: (workId: number, folderId?: number) =>
    request<{ ok: boolean; status: FavoriteStatus }>(`/api/favorites/works/${workId}${folderId ? `?folder_id=${folderId}` : ''}`, {
      method: 'DELETE',
    }),
  favoriteCreators: (folderId: number) => request<{ creators: Creator[] }>(`/api/favorites/creators?folder_id=${folderId}`),
  addCreatorFavorite: (creatorId: number, folderId?: number) =>
    request<{ ok: boolean; status: FavoriteStatus }>('/api/favorites/creators', {
      method: 'POST',
      body: JSON.stringify({ creator_id: creatorId, folder_id: folderId }),
    }),
  removeCreatorFavorite: (creatorId: number, folderId?: number) =>
    request<{ ok: boolean; status: FavoriteStatus }>(`/api/favorites/creators/${creatorId}${folderId ? `?folder_id=${folderId}` : ''}`, {
      method: 'DELETE',
    }),

  logPlayerAction: (body: {
    action: string;
    play_session_id?: string;
    work_id?: number;
    media_id?: number;
    file_name?: string;
    load_time_ms?: number;
    metadata_load_ms?: number;
    loaded_data_ms?: number;
    can_play_ms?: number;
    playing_ms?: number;
    first_frame_ms?: number;
    waiting_count?: number;
    stalled_count?: number;
    seeking_count?: number;
    last_seek_ms?: number;
    error_code?: number;
    preloaded?: boolean;
  }) =>
    request<{ ok: boolean }>('/api/logs/player', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
};

function logQuery(params: { page?: number; pageSize?: number; userId?: number; eventType?: string; keyword?: string }) {
  const query = new URLSearchParams();
  query.set('page', String(params.page ?? 1));
  query.set('page_size', String(params.pageSize ?? 50));
  if (params.userId) query.set('user_id', String(params.userId));
  if (params.eventType?.trim()) query.set('event_type', params.eventType.trim());
  if (params.keyword?.trim()) query.set('keyword', params.keyword.trim());
  return query;
}
