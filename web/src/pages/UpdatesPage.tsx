import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarClock, FolderSearch, KeyRound, Play, Plus, RefreshCw, Settings, Trash2, X } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { DirectoryBrowser } from '../components/DirectoryBrowser';
import { api } from '../lib/api';
import type { Creator, UpdateJob, UpdateQueue } from '../lib/types';

export function UpdatesPage() {
  const queryClient = useQueryClient();
  const [newName, setNewName] = useState('');
  const [newCron, setNewCron] = useState('0 3 * * *');
  const [authToken, setAuthToken] = useState('');
  const [ct0, setCt0] = useState('');
  const [proxy, setProxy] = useState('');
  const [downloadMode, setDownloadMode] = useState<'original' | 'custom'>('original');
  const [downloadDirectory, setDownloadDirectory] = useState('');
  const [downloadBrowserOpen, setDownloadBrowserOpen] = useState(false);
  const [fetchLimit, setFetchLimit] = useState(300);
  const [downloaderConcurrency, setDownloaderConcurrency] = useState(1);
  const [mediaOnly, setMediaOnly] = useState(true);
  const [includeRetweets, setIncludeRetweets] = useState(false);
  const [cookieJSON, setCookieJSON] = useState('');
  const [authHelp, setAuthHelp] = useState(false);
  const [editingQueue, setEditingQueue] = useState<UpdateQueue | null>(null);
  const [notice, setNotice] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [jobPage, setJobPage] = useState(1);
  const jobPageSize = 10;
  const status = useQuery({ queryKey: ['update-status'], queryFn: api.updateStatus, refetchInterval: 3000 });
  const updateJobs = useQuery({ queryKey: ['update-jobs', jobPage, jobPageSize], queryFn: () => api.updateJobs(jobPage, jobPageSize) });
  const queues = useQuery({ queryKey: ['update-queues'], queryFn: api.updateQueues });
  const auth = useQuery({ queryKey: ['update-auth'], queryFn: api.updateAuth });
  const queueList = queues.data?.queues ?? [];
  const statusValue = status.data?.status;
  const runningJobs = statusValue?.running_jobs ?? [];
  const jobsPage = updateJobs.data;
  const jobList = jobsPage?.jobs ?? [];

  useEffect(() => {
    const value = auth.data?.auth;
    if (!value) return;
    setAuthToken(value.auth_token ?? '');
    setCt0(value.ct0 ?? '');
    setProxy(value.proxy ?? '');
    setDownloadMode(value.download_mode === 'custom' ? 'custom' : 'original');
    setDownloadDirectory(value.download_directory ?? '');
    setFetchLimit(value.fetch_limit || 300);
    setDownloaderConcurrency(value.downloader_concurrency || 1);
    setMediaOnly(value.media_only);
    setIncludeRetweets(value.include_retweets);
  }, [auth.data?.auth]);

  const nextThreeDays = useMemo(() => queueList.filter((queue) => withinThreeDays(queue.next_run_at)), [queueList]);
  const createQueue = useMutation({
    mutationFn: () => api.createUpdateQueue(newName, newCron),
    onSuccess: () => {
      setNewName('');
      setNewCron('0 3 * * *');
      setNotice({ type: 'success', text: '定时更新队列已创建' });
      queryClient.invalidateQueries({ queryKey: ['update-queues'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });
  const patchQueue = useMutation({
    mutationFn: ({ id, body }: { id: number; body: { name?: string; cron?: string; is_default?: boolean; enabled?: boolean } }) => api.updateQueue(id, body),
    onSuccess: () => {
      setNotice({ type: 'success', text: '定时更新队列已更新' });
      queryClient.invalidateQueries({ queryKey: ['update-queues'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });
  const deleteQueue = useMutation({
    mutationFn: api.deleteUpdateQueue,
    onSuccess: () => {
      setNotice({ type: 'success', text: '定时更新队列已删除' });
      queryClient.invalidateQueries({ queryKey: ['update-queues'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });
  const runQueue = useMutation({
    mutationFn: api.runUpdateQueueNow,
    onSuccess: (_data, queueID) => {
      const queue = queueList.find((item) => item.id === queueID);
      setNotice({ type: 'success', text: `${queue?.name ?? '定时队列'} 已加入立即执行队列` });
      queryClient.invalidateQueries({ queryKey: ['update-status'] });
      queryClient.invalidateQueries({ queryKey: ['update-jobs'] });
      queryClient.invalidateQueries({ queryKey: ['update-queues'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });
  const saveAuth = useMutation({
    mutationFn: () =>
      api.saveUpdateAuth({
        auth_token: authToken,
        ct0,
        cookie_header: cookieJSON.trim() ? cookieJSON : undefined,
        proxy,
        download_mode: downloadMode,
        download_directory: downloadDirectory,
        fetch_limit: fetchLimit,
        media_only: mediaOnly,
        include_retweets: includeRetweets,
      }),
    onSuccess: (data) => {
      setAuthToken(data.auth.auth_token ?? '');
      setCt0(data.auth.ct0 ?? '');
      setNotice({ type: 'success', text: 'Twitter 登录配置已保存' });
      queryClient.invalidateQueries({ queryKey: ['update-auth'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });
  const saveRuntime = useMutation({
    mutationFn: () => api.saveUpdateAuth({ downloader_concurrency: downloaderConcurrency }),
    onSuccess: (data) => {
      setDownloaderConcurrency(data.auth.downloader_concurrency || 1);
      setNotice({ type: 'success', text: '运行设置已保存' });
      queryClient.invalidateQueries({ queryKey: ['update-auth'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });
  const validateAuth = useMutation({
    mutationFn: api.validateUpdateAuth,
    onSuccess: () => {
      setNotice({ type: 'success', text: 'Twitter 登录配置校验已完成' });
      queryClient.invalidateQueries({ queryKey: ['update-auth'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });
  const resetImmediate = useMutation({
    mutationFn: api.resetImmediateUpdate,
    onSuccess: () => {
      setNotice({ type: 'success', text: '立即执行队列已重置' });
      setJobPage(1);
      queryClient.invalidateQueries({ queryKey: ['update-status'] });
      queryClient.invalidateQueries({ queryKey: ['update-jobs'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });
  const retryJob = useMutation({
    mutationFn: api.retryUpdateJob,
    onSuccess: (data) => {
      setNotice({ type: 'success', text: data.retried ? `${data.job.creator_name} 已重新加入更新队列` : `${data.job.creator_name} 当前不需要重试` });
      queryClient.invalidateQueries({ queryKey: ['update-status'] });
      queryClient.invalidateQueries({ queryKey: ['update-jobs'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });
  const deleteJob = useMutation({
    mutationFn: api.deleteUpdateJob,
    onSuccess: () => {
      setNotice({ type: 'success', text: '更新记录已删除' });
      queryClient.invalidateQueries({ queryKey: ['update-status'] });
      queryClient.invalidateQueries({ queryKey: ['update-jobs'] });
    },
    onError: (cause) => setNotice({ type: 'error', text: errorMessage(cause) }),
  });

  return (
    <div className="page updates-page">
      <header className="page-header">
        <h1>更新管理</h1>
        <p>管理立即更新队列、定时更新队列、订阅博主和 Twitter/X 登录配置。</p>
      </header>
      {notice && (
        <p className={notice.type === 'success' ? 'success-line' : 'error-line'} role="status">
          {notice.text}
        </p>
      )}

      <section className="account-card update-runtime-card">
        <div className="account-card-head">
          <Settings size={18} />
          <div>
            <h2>运行设置</h2>
            <p>控制 Twitter/X 下载器同时更新的推主数量。</p>
          </div>
        </div>
        <form
          className="inline-form runtime-form"
          onSubmit={(event) => {
            event.preventDefault();
            saveRuntime.mutate();
          }}
        >
          <label>
            下载器并发数量
            <input
              aria-label="下载器并发数量"
              type="number"
              min={1}
              max={8}
              value={downloaderConcurrency}
              onChange={(event) => setDownloaderConcurrency(Math.max(1, Math.min(8, Number(event.target.value) || 1)))}
            />
          </label>
          <button type="submit" disabled={saveRuntime.isPending}>保存运行设置</button>
        </form>
        <p className="muted-line">当前并发：{auth.data?.auth.downloader_concurrency ?? downloaderConcurrency}，默认 1，最大 8。</p>
      </section>

      <section className="updates-grid">
        <article className="account-card update-status-card">
          <div className="account-card-head">
            <RefreshCw size={18} />
            <div>
              <h2>立即执行队列</h2>
              <p>{statusValue?.running ? `正在更新 ${statusValue.current_creator ?? '-'}` : '当前没有正在执行的更新任务'}</p>
            </div>
          </div>
          <div className="row-actions update-card-actions">
            <button
              type="button"
              className="danger"
              onClick={() => {
                if (window.confirm('确定重置立即更新队列？当前任务会被取消，历史记录也会被清空。')) resetImmediate.mutate();
              }}
            >
              重置
            </button>
          </div>
          <div className="progress-line">
            <span style={{ width: `${statusValue?.progress ?? 0}%` }} />
          </div>
          <div className="metric-grid">
            <Metric label="待更新博主" value={statusValue?.pending_creators ?? 0} />
            <Metric label="成功博主" value={statusValue?.succeeded_creators ?? 0} />
            <Metric label="失败博主" value={statusValue?.failed_creators ?? 0} />
            <Metric label="新增作品" value={statusValue?.added_works ?? 0} />
          </div>
          <section className="running-job-section">
            <h3>正在更新</h3>
            <div className="job-list">
              {runningJobs.map((job) => (
                <RunningJobCard key={job.id} job={job} />
              ))}
              {runningJobs.length === 0 && <div className="empty-state">当前没有正在执行的更新任务。</div>}
            </div>
          </section>
        </article>

        <article className="account-card">
          <div className="account-card-head">
            <CalendarClock size={18} />
            <div>
              <h2>最近 3 天要执行的队列</h2>
              <p>未来任务可直接加入立即执行队列。</p>
            </div>
          </div>
          <div className="queue-list">
            {nextThreeDays.map((queue) => (
              <QueueRow key={queue.id} queue={queue} onRun={() => runQueue.mutate(queue.id)} />
            ))}
            {nextThreeDays.length === 0 && <div className="empty-state">最近 3 天没有定时任务。</div>}
          </div>
        </article>
      </section>

      <section className="account-card job-history-section">
        <div className="section-title-row">
          <h2>更新记录</h2>
          <span>共 {jobsPage?.total ?? 0} 条</span>
        </div>
        <div className="job-list">
          {jobList.map((job) => (
            <UpdateJobRow
              key={job.id}
              job={job}
              onRetry={(id) => retryJob.mutate(id)}
              onDelete={(id) => deleteJob.mutate(id)}
              retryPending={retryJob.isPending}
              deletePending={deleteJob.isPending}
            />
          ))}
          {!updateJobs.isLoading && jobList.length === 0 && <div className="empty-state">暂无更新任务。</div>}
        </div>
        <div className="pager-row" aria-label="更新记录分页">
          <button type="button" disabled={jobPage <= 1 || updateJobs.isLoading} onClick={() => setJobPage((page) => Math.max(1, page - 1))}>
            上一页
          </button>
          <span>
            第 {jobsPage?.page ?? jobPage} / {jobsPage?.total_pages || 1} 页 · 共 {jobsPage?.total ?? 0} 条
          </span>
          <button type="button" disabled={jobPage >= (jobsPage?.total_pages || 1) || updateJobs.isLoading} onClick={() => setJobPage((page) => page + 1)}>
            下一页
          </button>
        </div>
      </section>

      <section className="account-card">
        <div className="account-card-head">
          <CalendarClock size={18} />
          <div>
            <h2>定时更新的队列</h2>
            <p>使用 5 段 crontab 表达式，例如每天 3 点：0 3 * * *</p>
          </div>
        </div>
        <form
          className="inline-form"
          onSubmit={(event) => {
            event.preventDefault();
            createQueue.mutate();
          }}
        >
          <input aria-label="队列名称" value={newName} onChange={(event) => setNewName(event.target.value)} placeholder="队列名称" />
          <input aria-label="cron 表达式" value={newCron} onChange={(event) => setNewCron(event.target.value)} placeholder="0 3 * * *" />
          <button type="submit" disabled={!newName.trim() || !newCron.trim()}>
            <Plus size={16} /> 新增队列
          </button>
        </form>
        <div className="queue-list">
          {queueList.map((queue) => (
            <div key={queue.id} className="queue-row">
              <div>
                <strong>{queue.name}</strong>
                <span>{queue.cron} · {queue.subscribed_count} 个博主 · 下次 {formatTime(queue.next_run_at)}</span>
                <small>{queue.last_run_summary || '暂无上次运行摘要'}</small>
              </div>
              <div className="row-actions">
                <button type="button" onClick={() => runQueue.mutate(queue.id)}>
                  <Play size={16} /> 立即更新
                </button>
                <button type="button" onClick={() => setEditingQueue(queue)}>
                  编辑
                </button>
                <button type="button" disabled={queue.is_default} onClick={() => patchQueue.mutate({ id: queue.id, body: { is_default: true } })}>
                  设为默认
                </button>
                <button type="button" onClick={() => patchQueue.mutate({ id: queue.id, body: { enabled: !queue.enabled } })}>
                  {queue.enabled ? '停用' : '启用'}
                </button>
                <button type="button" className="danger" onClick={() => deleteQueue.mutate(queue.id)}>
                  <Trash2 size={16} /> 删除
                </button>
              </div>
            </div>
          ))}
        </div>
      </section>

      <section className="account-card" aria-label="Twitter/X 登录配置">
        <div className="account-card-head">
          <KeyRound size={18} />
          <div>
            <h2>Twitter/X 登录配置</h2>
            <p>
              当前状态：{auth.data?.auth.configured ? '已保存' : '未保存'} ·
              {auth.data?.auth.cookie_header_saved ? ' 已保存完整 Cookie ·' : ''} 校验 {auth.data?.auth.validation_status ?? 'unknown'}
            </p>
          </div>
        </div>
        <div className="account-form update-auth-form">
          <label>
            <span>auth_token</span>
            <input aria-label="auth_token" value={authToken} onChange={(event) => setAuthToken(event.target.value)} placeholder="粘贴 auth_token；清空后保存会删除" autoComplete="off" />
          </label>
          <label>
            <span>ct0</span>
            <input aria-label="ct0" value={ct0} onChange={(event) => setCt0(event.target.value)} placeholder="粘贴 ct0；清空后保存会删除" autoComplete="off" />
          </label>
          <label>
            <span>代理</span>
            <input aria-label="代理" value={proxy} onChange={(event) => setProxy(event.target.value)} placeholder="http://127.0.0.1:7890，可选" />
          </label>
          <label>
            <span>默认抓取数量</span>
            <input aria-label="默认抓取数量" type="number" min={1} max={5000} value={fetchLimit} onChange={(event) => setFetchLimit(Number(event.target.value) || 300)} />
          </label>
          <label className="inline-check">
            <input type="checkbox" checked={mediaOnly} onChange={(event) => setMediaOnly(event.target.checked)} />
            <span>只抓媒体推文</span>
          </label>
          <label className="inline-check">
            <input type="checkbox" checked={includeRetweets} onChange={(event) => setIncludeRetweets(event.target.checked)} />
            <span>包含转推</span>
          </label>
        </div>
        <div className="update-download-settings">
          <h3>更新保存目录</h3>
          <div className="download-mode-options">
            <label className="inline-check">
              <input type="radio" name="download-mode" checked={downloadMode === 'original'} onChange={() => setDownloadMode('original')} />
              <span>原目录下载</span>
            </label>
            <label className="inline-check">
              <input type="radio" name="download-mode" checked={downloadMode === 'custom'} onChange={() => setDownloadMode('custom')} />
              <span>自定义保存目录</span>
            </label>
          </div>
          <div className="path-form update-download-path">
            <input aria-label="更新保存目录" value={downloadDirectory} onChange={(event) => setDownloadDirectory(event.target.value)} placeholder="/path/to/twitter" />
            <button type="button" onClick={() => setDownloadBrowserOpen(true)} title="浏览保存目录">
              <FolderSearch size={18} /> 浏览保存目录
            </button>
          </div>
          <p className="muted-line">原目录下载会优先使用博主已有目录；没有原始目录时会回退到这里配置的自定义目录。</p>
        </div>
        <label className="field-block">
          <span>Cookie JSON</span>
          <textarea
            className="cookie-editor"
            aria-label="Cookie JSON"
            value={cookieJSON}
            onChange={(event) => setCookieJSON(event.target.value)}
            placeholder='粘贴浏览器导出的 Cookie JSON，点击“提取 Cookie”自动填入 auth_token 和 ct0'
          />
        </label>
        <div className="row-actions">
          <button type="button" onClick={() => saveAuth.mutate()} disabled={saveAuth.isPending}>
            保存配置
          </button>
          <button type="button" onClick={() => validateAuth.mutate()} disabled={!auth.data?.auth.configured}>
            校验配置
          </button>
          <button type="button" onClick={() => setAuthHelp((value) => !value)}>
            获取 auth_token/ct0
          </button>
          <button
            type="button"
            onClick={() => {
              const extracted = extractTwitterCookies(cookieJSON);
              if (!extracted.authToken || !extracted.ct0) {
                setNotice({ type: 'error', text: '没有从 Cookie JSON 中找到 auth_token 和 ct0' });
                return;
              }
              setAuthToken(extracted.authToken);
              setCt0(extracted.ct0);
              setNotice({ type: 'success', text: '已从 Cookie JSON 提取 auth_token 和 ct0' });
            }}
          >
            提取 Cookie
          </button>
        </div>
        {auth.data?.auth.validation_message && <p className="muted-line">{auth.data.auth.validation_message}</p>}
        {authHelp && (
          <div className="hint-box">
            在浏览器登录 x.com 后，从开发者工具的 Cookie 中复制 <code>auth_token</code> 和 <code>ct0</code>。这些值只保存在本地 SQLite，用于服务端更新下载。
          </div>
        )}
      </section>
      <DirectoryBrowser
        open={downloadBrowserOpen}
        onClose={() => setDownloadBrowserOpen(false)}
        onSelect={(selected) => {
          setDownloadDirectory(selected);
          setDownloadMode('custom');
          setDownloadBrowserOpen(false);
        }}
      />
      {editingQueue && <QueueEditor queue={editingQueue} onClose={() => setEditingQueue(null)} />}
    </div>
  );
}

function QueueEditor({ queue, onClose }: { queue: UpdateQueue; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(queue.name);
  const [cron, setCron] = useState(queue.cron);
  const [creatorSearch, setCreatorSearch] = useState('');
  const members = useQuery({ queryKey: ['update-queue-creators', queue.id], queryFn: () => api.updateQueueCreators(queue.id) });
  const creators = useQuery({ queryKey: ['creators', creatorSearch], queryFn: () => api.creators({search: creatorSearch}) });
  const memberList = members.data?.creators ?? [];
  const creatorList = creators.data?.creators ?? [];
  const memberIDs = new Set(memberList.map((creator) => creator.id));
  const save = useMutation({
    mutationFn: () => api.updateQueue(queue.id, { name, cron }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['update-queues'] });
      onClose();
    },
  });
  const addCreator = useMutation({
    mutationFn: (creatorID: number) => api.addUpdateQueueCreators(queue.id, [creatorID]),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['update-queues'] });
      queryClient.invalidateQueries({ queryKey: ['update-queue-creators', queue.id] });
    },
  });
  const removeCreator = useMutation({
    mutationFn: (creatorID: number) => api.removeUpdateQueueCreator(queue.id, creatorID),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['update-queues'] });
      queryClient.invalidateQueries({ queryKey: ['update-queue-creators', queue.id] });
    },
  });

  return (
    <div className="modal-backdrop" role="dialog" aria-label="编辑定时队列">
      <div className="modal queue-editor-modal">
        <div className="modal-header">
          <h2>编辑定时队列</h2>
          <button className="modal-close" type="button" onClick={onClose} aria-label="关闭编辑定时队列" title="关闭">
            <X size={18} />
          </button>
        </div>
        <div className="account-form">
          <label>
            <span>队列名称</span>
            <input aria-label="队列名称" value={name} onChange={(event) => setName(event.target.value)} />
          </label>
          <label>
            <span>cron 表达式</span>
            <input aria-label="cron 表达式" value={cron} onChange={(event) => setCron(event.target.value)} />
          </label>
        </div>
        <section className="queue-editor-section">
          <h3>已订阅博主</h3>
          <div className="queue-member-list">
            {memberList.map((creator) => (
              <CreatorMemberRow key={creator.id} creator={creator} actionLabel="移除" onAction={() => removeCreator.mutate(creator.id)} />
            ))}
            {!members.isLoading && memberList.length === 0 && <div className="empty-state">这个队列还没有博主。</div>}
          </div>
        </section>
        <section className="queue-editor-section">
          <h3>添加博主</h3>
          <input className="queue-search-input" aria-label="搜索可添加博主" value={creatorSearch} onChange={(event) => setCreatorSearch(event.target.value)} placeholder="搜索博主" />
          <div className="queue-member-list">
            {creatorList.filter((creator) => !memberIDs.has(creator.id)).map((creator) => (
              <CreatorMemberRow key={creator.id} creator={creator} actionLabel="添加" onAction={() => addCreator.mutate(creator.id)} />
            ))}
          </div>
        </section>
        <div className="modal-actions">
          <button type="button" onClick={onClose}>取消</button>
          <button type="button" disabled={!name.trim() || !cron.trim() || save.isPending} onClick={() => save.mutate()}>保存</button>
        </div>
      </div>
    </div>
  );
}

function CreatorMemberRow({ creator, actionLabel, onAction }: { creator: Creator; actionLabel: string; onAction: () => void }) {
  return (
    <div className="queue-member-row">
      <div>
        <strong>{creator.name}</strong>
        <span>{creator.work_count} 个作品</span>
      </div>
      <button type="button" onClick={onAction}>{actionLabel}</button>
    </div>
  );
}

function QueueRow({ queue, onRun }: { queue: UpdateQueue; onRun: () => void }) {
  return (
    <div className="queue-row">
      <div>
        <strong>{queue.name}</strong>
        <span>{formatTime(queue.next_run_at)} · {queue.subscribed_count} 个博主</span>
      </div>
      <button type="button" onClick={onRun}>
        <Play size={16} /> 立即更新
      </button>
    </div>
  );
}

function RunningJobCard({ job }: { job: UpdateJob }) {
  const progress = job.total_tweets > 0 ? `${job.processed_tweets} / ${job.total_tweets}` : '-';
  return (
    <div className="job-row running-job-row">
      <div className="job-creator">
        <strong>{displayCreatorName(job.creator_name)}</strong>
        <span>{sourceLabel(job.source)} · {job.progress_message || '正在更新'}</span>
      </div>
      <div className="job-status">
        <span>处理推文 {progress}</span>
        <small>已下载视频 {job.downloaded_videos} · 已跳过推文 {job.skipped_tweets}</small>
        <small>{formatJobTiming(job)}</small>
      </div>
      <div className="job-detail">
        <span>{formatSpeed(job.download_speed_bytes_per_second)}</span>
        {job.download_directory && <small>保存到：{job.download_directory}</small>}
      </div>
    </div>
  );
}

function UpdateJobRow({
  job,
  onRetry,
  onDelete,
  retryPending,
  deletePending,
}: {
  job: UpdateJob;
  onRetry: (id: number) => void;
  onDelete: (id: number) => void;
  retryPending?: boolean;
  deletePending?: boolean;
}) {
  const creatorName = displayCreatorName(job.creator_name);
  return (
    <div className={`job-row job-row-${job.status}`}>
      <div className="job-creator">
        <strong>{creatorName}</strong>
        <span>{jobStatusLabel(job.status)}{job.attempt && job.attempt > 1 ? ` · 第 ${job.attempt} 次` : ''}</span>
        <small>{sourceLabel(job.source)}</small>
      </div>
      <div className="job-detail">
        <span>{formatJobSummary(job)}</span>
        <small>{formatJobTiming(job)}</small>
        <small>{formatJobDetail(job)}</small>
        {job.download_notice && <small className="warning-line">{job.download_notice}</small>}
        {job.error_detail && job.error_detail !== job.error && (
          <details className="job-error-detail">
            <summary>错误详情</summary>
            <code>{job.error_detail}</code>
          </details>
        )}
      </div>
      <div className="row-actions job-actions">
        {job.status === 'failed' && (
          <button type="button" className="tiny-button" disabled={job.status !== 'failed' || retryPending} onClick={() => onRetry(job.id)}>
            重试
          </button>
        )}
        {job.status !== 'running' && (
          <button type="button" className="tiny-button danger" disabled={job.status === 'running' || deletePending} onClick={() => onDelete(job.id)}>
            删除
          </button>
        )}
      </div>
    </div>
  );
}

function formatJobTiming(job: UpdateJob) {
  return `创建 ${formatTime(job.created_at)} · 开始 ${formatTime(job.started_at || '')} · 结束 ${formatTime(job.finished_at || '')}`;
}

function formatJobSummary(job: UpdateJob) {
  if (job.status === 'running') return `${job.progress_message || '正在更新'} · ${formatSpeed(job.download_speed_bytes_per_second)}`;
  if (job.status === 'failed') return job.error || '更新失败';
  if (job.added_works > 0) return `新增 ${job.added_works}`;
  if (job.status === 'succeeded') return '更新完成';
  return job.progress_message || '-';
}

function formatJobDetail(job: UpdateJob) {
  const detail = [
    job.total_tweets > 0 || job.processed_tweets > 0 ? `推文 ${job.processed_tweets} / ${job.total_tweets}` : '',
    job.downloaded_videos > 0 ? `已下载视频 ${job.downloaded_videos}` : '',
    job.skipped_tweets > 0 ? `已跳过推文 ${job.skipped_tweets}` : '',
    job.download_directory ? `保存到：${job.download_directory}` : '',
  ].filter(Boolean);
  return detail.join(' · ') || '-';
}

function sourceLabel(source?: string) {
  switch (source) {
    case 'scheduled':
      return '定时更新';
    default:
      return '人工触发';
  }
}

function displayCreatorName(value: string) {
  return value.startsWith('待更新-') ? value.slice('待更新-'.length) : value;
}

function formatSpeed(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '-';
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB/s`;
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB/s`;
  return `${Math.round(value)} B/s`;
}

function jobStatusLabel(status: string) {
  switch (status) {
    case 'pending':
      return '待更新';
    case 'running':
      return '运行中';
    case 'succeeded':
      return '成功';
    case 'failed':
      return '失败';
    default:
      return status || '-';
  }
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="metric-card">
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  );
}

function withinThreeDays(value: string) {
  if (!value) return false;
  const date = new Date(value);
  const now = Date.now();
  return Number.isFinite(date.getTime()) && date.getTime() >= now - 60_000 && date.getTime() <= now + 3 * 24 * 60 * 60 * 1000;
}

function formatTime(value: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return value;
  return date.toLocaleString();
}

function errorMessage(cause: unknown) {
  return cause instanceof Error ? cause.message : '操作失败';
}

function extractTwitterCookies(raw: string) {
  const result: { authToken?: string; ct0?: string } = {};
  const text = raw.trim();
  if (!text) return result;
  try {
    const parsed = JSON.parse(text);
    const cookies = Array.isArray(parsed) ? parsed : [parsed];
    for (const cookie of cookies) {
      if (!cookie || typeof cookie !== 'object') continue;
      const item = cookie as { name?: unknown; value?: unknown };
      if (item.name === 'auth_token' && typeof item.value === 'string') result.authToken = item.value;
      if (item.name === 'ct0' && typeof item.value === 'string') result.ct0 = item.value;
    }
    return result;
  } catch {
    for (const part of text.split(';')) {
      const [key, ...rest] = part.trim().split('=');
      const value = rest.join('=');
      if (key === 'auth_token' && value) result.authToken = value;
      if (key === 'ct0' && value) result.ct0 = value;
    }
    return result;
  }
}
