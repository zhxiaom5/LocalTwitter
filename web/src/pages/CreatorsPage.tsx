import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { CheckSquare, FolderSearch, RefreshCw, Search, Square, UserPlus, UserRound, X } from 'lucide-react';
import { DirectoryBrowser } from '../components/DirectoryBrowser';
import { api } from '../lib/api';
import type { BulkImportItem, Creator, Directory, UpdateQueue, User } from '../lib/types';

type SubscribeTarget = Creator | 'batch' | { type: 'bulk'; ids: number[] } | null;
type Notice = { type: 'success' | 'error'; text: string } | null;
type CreatorSortField = 'updated' | 'name' | 'views';
type SortDirection = 'asc' | 'desc';

export function CreatorsPage({ currentUser }: { currentUser?: User } = {}) {
  const queryClient = useQueryClient();
  const isAdmin = Boolean(currentUser?.can_update ?? currentUser?.is_admin);
  const [search, setSearch] = useState('');
  const [batchMode, setBatchMode] = useState(false);
  const [selected, setSelected] = useState<number[]>([]);
  const [subscribeTarget, setSubscribeTarget] = useState<SubscribeTarget>(null);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [notice, setNotice] = useState<Notice>(null);
  const [workCountFilterEnabled, setWorkCountFilterEnabled] = useState(true);
  const [minWorkCount, setMinWorkCount] = useState('1');
  const [linkTarget, setLinkTarget] = useState<Creator | null>(null);
  const [sortField, setSortField] = useState<CreatorSortField>('updated');
  const [sortDirection, setSortDirection] = useState<SortDirection>('desc');
  const [page, setPage] = useState(1);
  const pageSize = 50;

  const creators = useQuery({
    queryKey: ['creators', page, pageSize, sortField, sortDirection, search, minWorkCount, workCountFilterEnabled],
    queryFn: () => api.creators({ search, page, pageSize, sortField, sortDirection, minWorkCount: workCountFilterEnabled ? normalizeMinWorkCount(minWorkCount) : 0 }),
  });
  const directories = useQuery({ queryKey: ['directories'], queryFn: api.directories, enabled: isAdmin });
  const queues = useQuery({ queryKey: ['update-queues'], queryFn: api.updateQueues, enabled: isAdmin });
  const creatorList = creators.data?.creators ?? [];
  const displayedCreators = creatorList;
  const totalPages = creators.data?.total_pages ?? 0;
  const totalCreators = creators.data?.total ?? 0;
  const queueList = queues.data?.queues ?? [];
  const selectedCreators = subscribeTarget === 'batch' ? selected : subscribeTarget && 'type' in subscribeTarget ? subscribeTarget.ids : subscribeTarget ? [subscribeTarget.id] : [];

  const immediate = useMutation({
    mutationFn: (ids: number[]) => api.enqueueImmediateUpdate(ids),
    onSuccess: (_data, ids) => {
      setNotice({ type: 'success', text: `已加入立即更新队列：${ids.length} 个博主` });
      setSelected([]);
      setBatchMode(false);
      void queryClient.invalidateQueries({ queryKey: ['update-status'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });
  const subscribe = useMutation({
    mutationFn: async ({ queueIds, creatorIds }: { queueIds: number[]; creatorIds: number[] }) => {
      await Promise.all(queueIds.map((queueId) => api.addUpdateQueueCreators(queueId, creatorIds)));
      return { ok: true };
    },
    onSuccess: (_data, variables) => {
      setSubscribeTarget(null);
      setSelected([]);
      setBatchMode(false);
      setNotice({ type: 'success', text: `已保存订阅：${variables.creatorIds.length} 个博主加入 ${variables.queueIds.length} 个队列` });
      void queryClient.invalidateQueries({ queryKey: ['update-queues'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });
  const scanCreator = useMutation({
    mutationFn: (creator: Creator) => api.scanCreator(creator.id).then((result) => ({ result, creator })),
    onSuccess: ({ creator }) => {
      setNotice({ type: 'success', text: `已加入扫描队列：${creator.name}` });
      void queryClient.invalidateQueries({ queryKey: ['scan-status'] });
      void queryClient.invalidateQueries({ queryKey: ['creators'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });

  return (
    <div className="page narrow-page">
      <header className="page-header">
        <h1>关注</h1>
        <p>来自已保存目录的博主列表</p>
      </header>
      <div className="creator-toolbar">
        <label className="page-search">
          <Search size={18} />
          <input value={search} onChange={(event) => { setSearch(event.target.value); setPage(1); }} placeholder="搜索博主" aria-label="搜索博主" />
        </label>
        {isAdmin && (
          <div className="creator-batch-actions">
            <button type="button" onClick={() => setBatchMode((value) => !value)}>
              {batchMode ? <CheckSquare size={16} /> : <Square size={16} />} 批量选择
            </button>
            <button type="button" disabled={selected.length === 0 || immediate.isPending} onClick={() => immediate.mutate(selected)}>
              <RefreshCw size={16} /> 立即更新
            </button>
            <button type="button" disabled={selected.length === 0} onClick={() => setSubscribeTarget('batch')}>
              批量订阅
            </button>
            <button type="button" onClick={() => setBulkOpen(true)}>
              <UserPlus size={16} /> 批量新增关注
            </button>
            <label className="creator-work-count-filter">
              <input type="checkbox" checked={workCountFilterEnabled} onChange={(event) => { setWorkCountFilterEnabled(event.target.checked); setPage(1); }} aria-label="按作品数过滤" />
              <span>按作品数过滤</span>
              <input
                type="number"
                min="0"
                step="1"
                value={minWorkCount}
                disabled={!workCountFilterEnabled}
                onChange={(event) => { setMinWorkCount(event.target.value.replace(/[^\d]/g, '')); setPage(1); }}
                onBlur={() => {
                  if (minWorkCount.trim() === '') {
                    setMinWorkCount('1');
                  }
                }}
                aria-label="最小作品数"
              />
            </label>
          </div>
        )}
        <div className="creator-sort-actions" aria-label="博主排序">
          <label>
            排序
            <select value={sortField} onChange={(event) => { setSortField(event.target.value as CreatorSortField); setPage(1); }} aria-label="排序方式">
              <option value="updated">最近更新时间</option>
              <option value="name">博主名</option>
              <option value="views">博主访问量</option>
            </select>
          </label>
          <button type="button" onClick={() => { setSortDirection((value) => (value === 'asc' ? 'desc' : 'asc')); setPage(1); }} aria-label="切换排序方向" title={sortDirection === 'asc' ? '正排' : '倒排'}>
            {sortDirection === 'asc' ? '正排 ↑' : '倒排 ↓'}
          </button>
        </div>
      </div>
      {notice && (
        <p className={notice.type === 'success' ? 'success-line' : 'error-line'} role="status">
          {notice.text}
        </p>
      )}
      <div className="creator-list">
        {displayedCreators.map((creator) => (
          <article className="creator-card creator-card-with-actions" key={creator.id}>
            {batchMode && (
              <input
                aria-label={`选择-${creator.name}`}
                type="checkbox"
                checked={selected.includes(creator.id)}
                onChange={(event) => setSelected((current) => (event.target.checked ? [...current, creator.id] : current.filter((id) => id !== creator.id)))}
              />
            )}
            <Link className="creator-card-main" to={`/creator/${creator.id}`}>
              <div className="avatar">
                <CreatorAvatar creator={creator} size={44} />
              </div>
              <div>
                <div className="creator-title-line">
                  <strong>{creator.name}</strong>
                  {creator.bio && <span className="creator-bio-inline" title={creator.bio}>{creator.bio}</span>}
                </div>
                <span className="creator-meta-line">
                  <span>{creator.work_count} 个作品 · {canResolveTwitterAccount(creator) ? '可在线更新' : '无法解析账号'}</span>
                  <span className="creator-view-count" title={`播放 ${creator.view_count ?? 0}`}>播放 {formatCompactCount(creator.view_count ?? 0)}</span>
                </span>
              </div>
            </Link>
            {isAdmin && (
              <div className="creator-update-actions">
                <button type="button" onClick={() => immediate.mutate([creator.id])} disabled={immediate.isPending}>
                  立即更新
                </button>
                <button type="button" onClick={() => setSubscribeTarget(creator)}>
                  订阅更新
                </button>
                <button type="button" onClick={() => scanCreator.mutate(creator)} disabled={scanCreator.isPending}>
                  立即扫描
                </button>
                <button type="button" onClick={() => setLinkTarget(creator)}>
                  关联推主
                </button>
              </div>
            )}
          </article>
        ))}
      </div>
      {!creators.isLoading && displayedCreators.length === 0 && <div className="empty-state">{search.trim() || totalCreators > 0 ? '没有匹配的博主。' : '扫描成功后，博主会出现在这里。'}</div>}

      {totalPages > 1 && (
        <div className="pagination-bar">
          <button type="button" disabled={page <= 1} onClick={() => setPage((current) => Math.max(1, current - 1))}>
            上一页
          </button>
          <span>
            第 {page} / {totalPages} 页 · 共 {totalCreators} 个博主
          </span>
          <button type="button" disabled={page >= totalPages} onClick={() => setPage((current) => current + 1)}>
            下一页
          </button>
        </div>
      )}
      {subscribeTarget && (
        <SubscribeDialog
          queues={queueList}
          creatorIds={selectedCreators}
          onClose={() => setSubscribeTarget(null)}
          onSubmit={(queueIds) => subscribe.mutate({ queueIds, creatorIds: selectedCreators })}
          pending={subscribe.isPending}
        />
      )}
      {bulkOpen && (
        <BulkImportDialog
          directories={directories.data?.directories ?? []}
          onClose={() => setBulkOpen(false)}
          onImported={() => {
            void queryClient.invalidateQueries({ queryKey: ['creators'] });
            void queryClient.invalidateQueries({ queryKey: ['directories'] });
          }}
          onImmediateAsync={(ids) => immediate.mutateAsync(ids)}
          onSubscribe={(ids) => {
            setBulkOpen(false);
            setSubscribeTarget({ type: 'bulk', ids });
          }}
        />
      )}
      {linkTarget && (
        <CreatorLinksDialog
          creator={linkTarget}
          onClose={() => setLinkTarget(null)}
          onSaved={() => {
            void queryClient.invalidateQueries({ queryKey: ['creators'] });
          }}
        />
      )}
    </div>
  );
}

export function CreatorAvatar({ creator, size = 36 }: { creator: Pick<Creator, 'name' | 'avatar_url'>; size?: number }) {
  if (creator.avatar_url) {
    return <img className="creator-avatar-img" src={creator.avatar_url} alt={`${creator.name} 头像`} style={{ width: size, height: size }} />;
  }
  return <UserRound size={Math.max(18, Math.floor(size * 0.55))} />;
}

function CreatorLinksDialog({ creator, onClose, onSaved }: { creator: Creator; onClose: () => void; onSaved: () => void }) {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const links = useQuery({ queryKey: ['creator-links', creator.id], queryFn: () => api.creatorLinks(creator.id) });
  const initialLinkedIds = useMemo(() => (links.data?.linked ?? []).map((item) => item.id), [links.data?.linked]);
  const [checked, setChecked] = useState<number[]>([]);

  useEffect(() => {
    setChecked(initialLinkedIds);
  }, [initialLinkedIds]);

  const save = useMutation({
    mutationFn: () => api.saveCreatorLinks(creator.id, checked),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['creator-links', creator.id] });
      onSaved();
      onClose();
    },
  });

  const candidates = (links.data?.candidates ?? []).filter((item) => {
    const keyword = search.trim().toLowerCase();
    if (!keyword) return true;
    return item.name.toLowerCase().includes(keyword) || (item.twitter_username ?? '').toLowerCase().includes(keyword);
  });

  return (
    <div className="modal-backdrop" role="dialog" aria-label="关联推主">
      <div className="modal creator-links-modal">
        <div className="modal-header">
          <h2>关联推主</h2>
          <button className="modal-close" type="button" onClick={onClose} aria-label="关闭关联推主" title="关闭">
            <X size={18} />
          </button>
        </div>
        <p>将更名前后的推主归为一组。进入任一推主 Feed 时，会一起浏览整组作品。</p>
        <div className="creator-link-target">
          <CreatorAvatar creator={creator} size={40} />
          <strong>{creator.name}</strong>
        </div>
        <label className="page-search modal-search">
          <Search size={16} />
          <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索可关联推主" aria-label="搜索可关联推主" />
        </label>
        <div className="creator-link-list">
          {candidates.map((candidate) => (
            <label key={candidate.id} className="creator-link-choice">
              <input
                type="checkbox"
                checked={checked.includes(candidate.id)}
                onChange={(event) => setChecked((current) => (event.target.checked ? [...current, candidate.id] : current.filter((id) => id !== candidate.id)))}
              />
              <CreatorAvatar creator={candidate} size={34} />
              <span>
                <strong>{candidate.name}</strong>
                {candidate.bio && <small>{candidate.bio}</small>}
              </span>
            </label>
          ))}
          {!links.isLoading && candidates.length === 0 && <div className="empty-state">没有可关联的推主。</div>}
        </div>
        {save.error && <p className="error-line">{errorMessage(save.error)}</p>}
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            取消
          </button>
          <button type="button" onClick={() => save.mutate()} disabled={save.isPending}>
            保存关联
          </button>
        </div>
      </div>
    </div>
  );
}

function BulkImportDialog({
  directories,
  onClose,
  onImported,
  onImmediateAsync,
  onSubscribe,
}: {
  directories: Directory[];
  onClose: () => void;
  onImported: () => void;
  onImmediateAsync: (ids: number[]) => Promise<unknown>;
  onSubscribe: (ids: number[]) => void;
}) {
  const queryClient = useQueryClient();
  const [directoryID, setDirectoryID] = useState<number>(directories[0]?.id ?? 0);
  const [directoryPath, setDirectoryPath] = useState('');
  const [browserOpen, setBrowserOpen] = useState(false);
  const [savedDirectories, setSavedDirectories] = useState<Directory[]>([]);
  const [text, setText] = useState('');
  const [items, setItems] = useState<BulkImportItem[]>([]);
  const [importedCreators, setImportedCreators] = useState<Creator[]>([]);
  const [actionMessage, setActionMessage] = useState<Notice>(null);
  const allDirectories = [...savedDirectories, ...directories.filter((directory) => !savedDirectories.some((saved) => saved.id === directory.id))];
  const selectedDirectory = allDirectories.find((directory) => directory.id === directoryID);
  const validProfileURLs = items.filter((item) => item.status === 'ready' && item.profile_url).map((item) => item.profile_url as string);
  const importedIDs = importedCreators.map((creator) => creator.id);

  useEffect(() => {
    if (!directoryID && directories[0]) {
      setDirectoryID(directories[0].id);
    }
  }, [directories, directoryID]);

  const saveDirectory = useMutation({
    mutationFn: api.saveDirectory,
    onSuccess: (data) => {
      setSavedDirectories((current) => [data.directory, ...current.filter((directory) => directory.id !== data.directory.id)]);
      setDirectoryID(data.directory.id);
      setDirectoryPath(data.directory.path);
      void queryClient.invalidateQueries({ queryKey: ['directories'] });
    },
  });

  const preview = useMutation({
    mutationFn: () => api.previewBulkImportCreators(directoryID, text),
    onSuccess: (data) => {
      setItems(data.items ?? []);
      setImportedCreators([]);
      setActionMessage(null);
    },
  });
  const submit = useMutation({
    mutationFn: () => api.bulkImportCreators(directoryID, validProfileURLs),
    onSuccess: (data) => {
      setImportedCreators(data.creators);
      onImported();
    },
  });

  async function enqueueImported() {
    setActionMessage(null);
    try {
      await onImmediateAsync(importedIDs);
      setActionMessage({ type: 'success', text: `已加入立即更新队列：${importedIDs.length} 个博主` });
    } catch (cause) {
      setActionMessage({ type: 'error', text: errorMessage(cause) });
    }
  }

  return (
    <div className="modal-backdrop" role="dialog" aria-label="批量新增关注">
      <div className="modal bulk-import-modal">
        <div className="modal-header">
          <h2>批量新增关注</h2>
          <button className="modal-close" type="button" onClick={onClose} aria-label="关闭批量新增关注" title="关闭">
            <X size={18} />
          </button>
        </div>
        <p>粘贴 X/Twitter 主页、@用户名或分享文案，解析后可从 0 加入更新队列。</p>
        <div className="bulk-directory-picker">
          <label className="field-block">
            <span>保存目录路径</span>
            <input aria-label="保存目录路径" value={directoryPath} onChange={(event) => setDirectoryPath(event.target.value)} placeholder="/path/to/twitter" />
          </label>
          <button type="button" onClick={() => setBrowserOpen(true)} title="浏览目录">
            <FolderSearch size={18} /> 浏览
          </button>
          <button type="button" disabled={!directoryPath.trim() || saveDirectory.isPending} onClick={() => saveDirectory.mutate(directoryPath.trim())}>
            保存目录
          </button>
        </div>
        {saveDirectory.error && <p className="error-line">{saveDirectory.error.message}</p>}
        <div className="bulk-selected-directory">当前目录：{selectedDirectory ? `${selectedDirectory.name} · ${selectedDirectory.path}` : '请先保存或选择一个目录'}</div>
        <label className="field-block">
          <span>批量关注文本</span>
          <textarea
            className="bulk-import-textarea"
            aria-label="批量关注文本"
            value={text}
            onChange={(event) => setText(event.target.value)}
            placeholder="每行粘贴一个 X/Twitter 主页、@用户名或分享文案"
          />
        </label>
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            取消
          </button>
          <button type="button" disabled={!directoryID || !text.trim() || preview.isPending} onClick={() => preview.mutate()}>
            解析
          </button>
          <button type="button" disabled={validProfileURLs.length === 0 || submit.isPending} onClick={() => submit.mutate()}>
            提交
          </button>
        </div>
        {preview.error && <p className="error-line">{errorMessage(preview.error)}</p>}
        {submit.error && <p className="error-line">{errorMessage(submit.error)}</p>}
        {items.length > 0 && (
          <div className="bulk-import-results">
            <div className="bulk-import-row bulk-import-head">
              <span>原始内容</span>
              <span>主页地址</span>
              <span>状态</span>
            </div>
            {items.map((item, index) => (
              <div className="bulk-import-row" key={`${item.input}-${index}`}>
                <span title={item.input}>{item.input}</span>
                <span>{item.profile_url || '-'}</span>
                <span>{item.status === 'ready' ? (item.existing ? '已存在' : '可导入') : item.error}</span>
              </div>
            ))}
          </div>
        )}
        {importedCreators.length > 0 && (
          <div className="bulk-import-success">
            <strong>已导入 {importedCreators.length} 个博主</strong>
            <p>下载完成后会自动入库，可在关注页或 Feed 中查看。</p>
            {actionMessage && <p className={actionMessage.type === 'success' ? 'success-line' : 'error-line'}>{actionMessage.text}</p>}
            <div className="modal-actions">
              <button type="button" onClick={enqueueImported}>
                立即更新
              </button>
              <button type="button" onClick={() => onSubscribe(importedIDs)}>
                添加到定时更新队列
              </button>
            </div>
          </div>
        )}
      </div>
      <DirectoryBrowser
        open={browserOpen}
        onClose={() => setBrowserOpen(false)}
        onSelect={(selected) => {
          setDirectoryPath(selected);
          setBrowserOpen(false);
        }}
      />
    </div>
  );
}

function SubscribeDialog({
  queues,
  creatorIds,
  onClose,
  onSubmit,
  pending,
}: {
  queues: UpdateQueue[];
  creatorIds: number[];
  onClose: () => void;
  onSubmit: (queueIds: number[]) => void;
  pending: boolean;
}) {
  const defaultQueues = queues.filter((queue) => queue.is_default).map((queue) => queue.id);
  const [checked, setChecked] = useState<number[]>(defaultQueues.length > 0 ? defaultQueues : queues.slice(0, 1).map((queue) => queue.id));
  return (
    <div className="modal-backdrop" role="dialog" aria-label="订阅更新">
      <div className="modal">
        <div className="modal-header">
          <h2>订阅更新</h2>
          <button className="modal-close" type="button" onClick={onClose} aria-label="关闭订阅更新" title="关闭">
            <X size={18} />
          </button>
        </div>
        <p>选择要加入的定时更新队列，共 {creatorIds.length} 个博主。</p>
        <div className="queue-choice-list">
          {queues.map((queue) => (
            <label key={queue.id}>
              <input
                type="checkbox"
                checked={checked.includes(queue.id)}
                onChange={(event) => setChecked((current) => (event.target.checked ? [...current, queue.id] : current.filter((id) => id !== queue.id)))}
              />
              <span>{queue.name}</span>
              <small>
                {queue.cron}
                {queue.is_default ? ' · 默认' : ''}
              </small>
            </label>
          ))}
          {queues.length === 0 && <div className="empty-state">还没有定时队列。</div>}
        </div>
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            取消
          </button>
          <button type="button" disabled={checked.length === 0 || creatorIds.length === 0 || pending} onClick={() => onSubmit(checked)}>
            保存订阅
          </button>
        </div>
      </div>
    </div>
  );
}

function errorMessage(cause: unknown) {
  return cause instanceof Error ? cause.message : '操作失败';
}

function compareCreators(a: Creator, b: Creator, field: CreatorSortField, direction: SortDirection) {
  let value = 0;
  if (field === 'name') {
    value = a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
  } else if (field === 'views') {
    value = (a.view_count ?? 0) - (b.view_count ?? 0);
  } else {
    value = compareOptionalTime(a.last_work_created_at, b.last_work_created_at, direction);
    if (value === 0) {
      value = a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
    }
    return value;
  }
  if (value === 0) {
    value = a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
  }
  return direction === 'asc' ? value : -value;
}

function compareOptionalTime(a: string | undefined, b: string | undefined, direction: SortDirection) {
  const aTime = parseCreatorTime(a);
  const bTime = parseCreatorTime(b);
  const aValid = Number.isFinite(aTime);
  const bValid = Number.isFinite(bTime);
  if (!aValid && !bValid) return 0;
  if (!aValid) return 1;
  if (!bValid) return -1;
  return direction === 'asc' ? aTime - bTime : bTime - aTime;
}

function parseCreatorTime(value?: string) {
  if (!value) return Number.NaN;
  const direct = Date.parse(value);
  if (Number.isFinite(direct)) return direct;
  return Date.parse(value.replace(' ', 'T'));
}

export function formatCompactCount(value: number) {
  const count = Math.max(0, Math.floor(Number.isFinite(value) ? value : 0));
  if (count < 1000) return String(count);
  if (count < 1_000_000) return `${trimCompactNumber(count / 1000)}K`;
  if (count < 1_000_000_000) return `${trimCompactNumber(count / 1_000_000)}M`;
  return `${trimCompactNumber(count / 1_000_000_000)}B`;
}

function normalizeMinWorkCount(value: string) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : 1;
}

function trimCompactNumber(value: number) {
  return value >= 10 ? value.toFixed(0) : value.toFixed(1).replace(/\.0$/, '');
}

function canResolveTwitterAccount(creator: Creator) {
  if (creator.online_identity_status === 'ready') return true;
  if (isTwitterUsername(creator.twitter_username ?? '')) return true;
  return isTwitterUsername(creator.name);
}

function isTwitterUsername(value: string) {
  const username = value.trim().replace(/^@/, '');
  return /^[A-Za-z0-9_]{1,15}$/.test(username);
}
