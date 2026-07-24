import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { UpdatesPage } from './UpdatesPage';

let eventSourceConstructor: ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

beforeEach(() => {
  eventSourceConstructor = vi.fn(function EventSourceMock(this: { addEventListener: ReturnType<typeof vi.fn>; close: ReturnType<typeof vi.fn> }) {
    this.addEventListener = vi.fn();
    this.close = vi.fn();
  });
  Object.defineProperty(window, 'EventSource', { configurable: true, value: eventSourceConstructor });
});

afterEach(() => {
  vi.restoreAllMocks();
});

function renderUpdatesPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <UpdatesPage />
    </QueryClientProvider>,
  );
}

it('does not open a page-level update EventSource', async () => {
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({ status: { running: false, pending_creators: 0, total_creators: 0, succeeded_creators: 0, failed_creators: 0, added_works: 0, progress: 0, state: 'idle', errors: [], jobs: [] } });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({ jobs: [], page: 1, page_size: 10, total: 0, total_pages: 0 });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({ auth: { configured: false, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  expect(await screen.findByText('更新管理')).toBeInTheDocument();
  expect(eventSourceConstructor).not.toHaveBeenCalled();
});

it('fills saved twitter auth settings into the update form', async () => {
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({ status: { running: false, pending_creators: 0, total_creators: 0, succeeded_creators: 0, failed_creators: 0, added_works: 0, progress: 0, state: 'idle', errors: [], jobs: [] } });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({ jobs: [], page: 1, page_size: 10, total: 0, total_pages: 0 });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({
        auth: {
          configured: true,
          auth_token: 'saved-auth',
          ct0: 'saved-ct0',
          proxy: 'socks5://127.0.0.1:1080',
          download_mode: 'custom',
          download_directory: '/tmp/twitter-updates',
          fetch_limit: 120,
          media_only: true,
          include_retweets: false,
          validation_status: 'valid',
        },
      });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  expect(await screen.findByDisplayValue('saved-auth')).toBeInTheDocument();
  expect(screen.getByDisplayValue('saved-ct0')).toBeInTheDocument();
  expect(screen.getByDisplayValue('socks5://127.0.0.1:1080')).toBeInTheDocument();
  expect(screen.getByDisplayValue('/tmp/twitter-updates')).toBeInTheDocument();
  expect(screen.getByRole('radio', { name: '自定义保存目录' })).toBeChecked();
  expect(screen.getByDisplayValue('120')).toBeInTheDocument();
});

it('shows tweet-level progress and friendly job errors', async () => {
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({
        status: {
          running: true,
          current_creator: 'twuser',
          pending_creators: 0,
          total_creators: 1,
          succeeded_creators: 0,
          failed_creators: 1,
          added_works: 0,
          progress: 40,
          state: 'running',
          errors: ['twuser: Twitter 登录配置不可用，请更新完整 Cookie 或重新登录 X'],
          jobs: [],
        },
      });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({
        jobs: [
          {
            id: 1,
            creator_id: 1,
            creator_name: 'twuser',
            status: 'running',
            added_works: 0,
            total_tweets: 10,
            processed_tweets: 4,
          downloaded_videos: 2,
          skipped_tweets: 1,
          download_speed_bytes_per_second: 12939427,
          progress_message: '正在处理推文 4/10',
            download_directory: '/tmp/twitter/twuser/video',
            download_notice: '未找到原始目录，已使用自定义保存目录',
            created_at: '',
          },
          {
            id: 2,
            creator_id: 2,
            creator_name: 'badcookie',
            status: 'failed',
            added_works: 0,
            error: 'Twitter 登录配置不可用，请更新完整 Cookie 或重新登录 X',
            error_detail: 'response status 401 Unauthorized',
            created_at: '',
          },
        ],
        page: 1,
        page_size: 10,
        total: 2,
        total_pages: 1,
      });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({ auth: { configured: false, proxy: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  expect(await screen.findByText('运行中')).toBeInTheDocument();
  expect(screen.getByText(/推文 4 \/ 10/)).toBeInTheDocument();
  expect(screen.getByText(/已下载视频 2/)).toBeInTheDocument();
  expect(screen.getByText(/已跳过推文 1/)).toBeInTheDocument();
  expect(screen.getByText(/12.3 MB\/s/)).toBeInTheDocument();
  expect(screen.getByText(/保存到：\/tmp\/twitter\/twuser\/video/)).toBeInTheDocument();
  expect(screen.getByText('未找到原始目录，已使用自定义保存目录')).toBeInTheDocument();
  expect(screen.getByText('失败')).toBeInTheDocument();
  expect(screen.getByText('Twitter 登录配置不可用，请更新完整 Cookie 或重新登录 X')).toBeInTheDocument();
  expect(screen.getByText('错误详情')).toBeInTheDocument();
});

it('does not render pending marker as part of creator names', async () => {
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({ status: { running: false, pending_creators: 1, total_creators: 1, succeeded_creators: 0, failed_creators: 0, added_works: 0, progress: 0, state: 'queued', errors: [], jobs: [] } });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({
        jobs: [
          {
            id: 1,
            creator_id: 1,
            creator_name: '待更新-on200693',
            status: 'pending',
            added_works: 0,
            total_tweets: 0,
            processed_tweets: 0,
            downloaded_videos: 0,
            skipped_tweets: 0,
            created_at: '',
          },
        ],
        page: 1,
        page_size: 10,
        total: 1,
        total_pages: 1,
      });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({ auth: { configured: false, proxy: '', downloader_concurrency: 1, download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  expect(await screen.findByText('on200693')).toBeInTheDocument();
  expect(screen.queryByText('待更新-on200693')).not.toBeInTheDocument();
  expect(screen.getByText('待更新')).toBeInTheDocument();
});

it('retries failed update jobs without retrying completed jobs', async () => {
  const user = userEvent.setup();
  const retryCalls: string[] = [];
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({ status: { running: false, pending_creators: 0, total_creators: 0, succeeded_creators: 1, failed_creators: 1, added_works: 0, progress: 100, state: 'idle', errors: [], jobs: [] } });
    }
    if (url.startsWith('/api/update/jobs/2/retry') && init?.method === 'POST') {
      retryCalls.push(url);
      return jsonResponse({ retried: true, job: { id: 2, creator_id: 2, creator_name: 'failed-user', source: 'manual', attempt: 2, status: 'pending', added_works: 0, total_tweets: 0, processed_tweets: 0, downloaded_videos: 0, skipped_tweets: 0, download_speed_bytes_per_second: 0, created_at: '' }, status: { running: false, pending_creators: 1, total_creators: 1, succeeded_creators: 1, failed_creators: 0, added_works: 0, progress: 50, state: 'queued', errors: [], jobs: [] } });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({
        jobs: [
          { id: 1, creator_id: 1, creator_name: 'done-user', source: 'scheduled', attempt: 1, status: 'succeeded', added_works: 1, total_tweets: 0, processed_tweets: 0, downloaded_videos: 0, skipped_tweets: 0, download_speed_bytes_per_second: 0, created_at: '' },
          { id: 2, creator_id: 2, creator_name: 'failed-user', source: 'manual', attempt: 1, status: 'failed', added_works: 0, total_tweets: 0, processed_tweets: 0, downloaded_videos: 0, skipped_tweets: 0, download_speed_bytes_per_second: 0, error: '网络或代理不可用，请检查代理地址', created_at: '' },
        ],
        page: 1,
        page_size: 10,
        total: 2,
        total_pages: 1,
      });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({ auth: { configured: false, proxy: '', downloader_concurrency: 1, download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  expect(await screen.findByText('done-user')).toBeInTheDocument();
  const doneRow = screen.getByText('done-user').closest('.job-row');
  const failedRow = screen.getByText('failed-user').closest('.job-row');
  expect(doneRow).not.toBeNull();
  expect(failedRow).not.toBeNull();
  expect(within(doneRow as HTMLElement).queryByRole('button', { name: '重试' })).not.toBeInTheDocument();
  const retryButton = within(failedRow as HTMLElement).getByRole('button', { name: '重试' });
  expect(retryButton).toBeEnabled();
  await user.click(retryButton);

  expect(retryCalls).toEqual(['/api/update/jobs/2/retry']);
  expect(await screen.findByText('failed-user 已重新加入更新队列')).toBeInTheDocument();
});

it('shows job source and attempt, and deletes non-running records only', async () => {
  const user = userEvent.setup();
  const deleteCalls: string[] = [];
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({
        status: {
          running: true,
          pending_creators: 0,
          total_creators: 1,
          succeeded_creators: 0,
          failed_creators: 0,
          added_works: 0,
          progress: 10,
          state: 'running',
          errors: [],
          running_jobs: [{ id: 1, creator_id: 1, creator_name: 'running-user', source: 'manual', attempt: 1, status: 'running', added_works: 0, total_tweets: 10, processed_tweets: 1, downloaded_videos: 0, skipped_tweets: 0, download_speed_bytes_per_second: 0, created_at: '' }],
          jobs: [],
        },
      });
    }
    if (url === '/api/update/jobs/2' && init?.method === 'DELETE') {
      deleteCalls.push(url);
      return jsonResponse({ ok: true });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({
        jobs: [
          { id: 1, creator_id: 1, creator_name: 'running-user', source: 'manual', attempt: 1, status: 'running', added_works: 0, total_tweets: 10, processed_tweets: 1, downloaded_videos: 0, skipped_tweets: 0, download_speed_bytes_per_second: 0, created_at: '' },
          { id: 2, creator_id: 2, creator_name: 'scheduled-user', source: 'scheduled', attempt: 3, status: 'failed', added_works: 0, total_tweets: 0, processed_tweets: 0, downloaded_videos: 0, skipped_tweets: 0, download_speed_bytes_per_second: 0, error: 'bad auth', created_at: '' },
        ],
        page: 1,
        page_size: 10,
        total: 2,
        total_pages: 1,
      });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({ auth: { configured: true, downloader_concurrency: 1, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'valid' } });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  expect((await screen.findAllByText('running-user')).length).toBeGreaterThan(0);
  expect(screen.getByText(/人工触发 · 正在更新/)).toBeInTheDocument();
  expect(screen.getByText('失败 · 第 3 次')).toBeInTheDocument();
  expect(screen.getByText('定时更新')).toBeInTheDocument();
  const runningRow = screen.getAllByText('running-user')[0].closest('.job-row');
  const scheduledRow = screen.getByText('scheduled-user').closest('.job-row');
  expect(runningRow).not.toBeNull();
  expect(scheduledRow).not.toBeNull();
  expect(within(runningRow as HTMLElement).queryByRole('button', { name: '删除' })).not.toBeInTheDocument();
  await user.click(within(scheduledRow as HTMLElement).getByRole('button', { name: '删除' }));
  expect(deleteCalls).toEqual(['/api/update/jobs/2']);
  expect(await screen.findByText('更新记录已删除')).toBeInTheDocument();
});

it('keeps long update job paths inside the detail column', async () => {
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({ status: { running: false, pending_creators: 0, total_creators: 0, succeeded_creators: 1, failed_creators: 0, added_works: 0, progress: 100, state: 'idle', errors: [], running_jobs: [], jobs: [] } });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({
        jobs: [
          {
            id: 9,
            creator_id: 9,
            creator_name: 'long-path-user',
            source: 'manual',
            attempt: 1,
            status: 'succeeded',
            added_works: 0,
            total_tweets: 223,
            processed_tweets: 223,
            downloaded_videos: 0,
            skipped_tweets: 223,
            download_speed_bytes_per_second: 0,
            download_directory: '/vol1/1000/SynoDisks/disk_for_yule5/hermesclaw_data/twitter/very/deep/path/that/should/wrap/in/the/detail/column',
            created_at: '2026-07-11T14:36:04Z',
            started_at: '2026-07-11T14:36:04Z',
            finished_at: '2026-07-11T14:36:18Z',
          },
        ],
        page: 1,
        page_size: 10,
        total: 1,
        total_pages: 1,
      });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({ auth: { configured: true, downloader_concurrency: 1, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'valid' } });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  const row = (await screen.findByText('long-path-user')).closest('.job-row');
  expect(row).not.toBeNull();
  const detail = row?.querySelector('.job-detail');
  expect(detail).not.toBeNull();
  expect(within(detail as HTMLElement).getByText(/保存到：\/vol1\/1000\/SynoDisks/)).toBeInTheDocument();
  expect(row?.querySelector('.job-row-meta-line')).toBeNull();
});

it('shows automatic download retry progress and exhausted timeout message', async () => {
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({ status: { running: true, pending_creators: 0, total_creators: 2, succeeded_creators: 0, failed_creators: 1, added_works: 0, progress: 50, state: 'running', errors: [], jobs: [] } });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({
        jobs: [
          {
            id: 1,
            creator_id: 1,
            creator_name: 'large-video',
            status: 'running',
            added_works: 0,
            total_tweets: 1,
            processed_tweets: 1,
            downloaded_videos: 0,
            skipped_tweets: 0,
            download_speed_bytes_per_second: 0,
            progress_message: '视频下载失败，5 秒后重试（2/3）',
            created_at: '',
          },
          {
            id: 2,
            creator_id: 2,
            creator_name: 'timeout-video',
            status: 'failed',
            added_works: 0,
            total_tweets: 1,
            processed_tweets: 1,
            downloaded_videos: 0,
            skipped_tweets: 0,
            download_speed_bytes_per_second: 0,
            error: '视频下载超时，已自动重试后仍失败',
            error_detail: '视频下载失败，已自动重试 3 次: context deadline exceeded (Client.Timeout or context cancellation while reading body)',
            created_at: '',
          },
        ],
        page: 1,
        page_size: 10,
        total: 2,
        total_pages: 1,
      });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({ auth: { configured: true, downloader_concurrency: 1, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'valid' } });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  expect(await screen.findByText(/视频下载失败，5 秒后重试（2\/3）/)).toBeInTheDocument();
  expect(screen.getByText('视频下载超时，已自动重试后仍失败')).toBeInTheDocument();
  expect(screen.getByText('错误详情')).toBeInTheDocument();
});

it('paginates immediate update job records with ten rows by default', async () => {
  const user = userEvent.setup();
  const pageOne = Array.from({ length: 10 }, (_, index) => ({
    id: 12 - index,
    creator_id: 1,
    creator_name: `creator-${12 - index}`,
    status: 'succeeded',
    added_works: index,
    total_tweets: 0,
    processed_tweets: 0,
    downloaded_videos: 0,
    skipped_tweets: 0,
    created_at: '',
  }));
  const pageTwo = [
    {
      id: 2,
      creator_id: 1,
      creator_name: 'creator-2',
      status: 'succeeded',
      added_works: 0,
      total_tweets: 0,
      processed_tweets: 0,
      downloaded_videos: 0,
      skipped_tweets: 0,
      created_at: '',
    },
    {
      id: 1,
      creator_id: 1,
      creator_name: 'creator-1',
      status: 'succeeded',
      added_works: 0,
      total_tweets: 0,
      processed_tweets: 0,
      downloaded_videos: 0,
      skipped_tweets: 0,
      created_at: '',
    },
  ];
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({ status: { running: false, pending_creators: 0, total_creators: 0, succeeded_creators: 0, failed_creators: 0, added_works: 0, progress: 0, state: 'idle', errors: [], jobs: [] } });
    }
    if (url === '/api/update/jobs?page=2&page_size=10') {
      return jsonResponse({ jobs: pageTwo, page: 2, page_size: 10, total: 12, total_pages: 2 });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({ jobs: pageOne, page: 1, page_size: 10, total: 12, total_pages: 2 });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({ auth: { configured: false, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  expect(await screen.findByText('creator-12')).toBeInTheDocument();
  expect(screen.getByText('creator-3')).toBeInTheDocument();
  expect(screen.queryByText('creator-2')).not.toBeInTheDocument();
  expect(screen.getByText('第 1 / 2 页 · 共 12 条')).toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: '下一页' }));

  expect(await screen.findByText('creator-2')).toBeInTheDocument();
  expect(screen.getByText('creator-1')).toBeInTheDocument();
  expect(screen.queryByText('creator-12')).not.toBeInTheDocument();
  expect(screen.getByText('第 2 / 2 页 · 共 12 条')).toBeInTheDocument();
});

it('extracts auth_token and ct0 from pasted cookie json', async () => {
  const user = userEvent.setup();
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({ status: { running: false, pending_creators: 0, total_creators: 0, succeeded_creators: 0, failed_creators: 0, added_works: 0, progress: 0, state: 'idle', errors: [], jobs: [] } });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({ jobs: [], page: 1, page_size: 10, total: 0, total_pages: 0 });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({ auth: { configured: false, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  const authSection = await screen.findByRole('region', { name: 'Twitter/X 登录配置' });
  await user.click(within(authSection).getByLabelText('Cookie JSON'));
  await user.paste(
    JSON.stringify([
      { name: 'guest_id', value: 'guest' },
      { name: 'auth_token', value: 'auth-from-cookie' },
      { name: 'ct0', value: 'ct0-from-cookie' },
    ]),
  );
  await user.click(within(authSection).getByRole('button', { name: '提取 Cookie' }));

  expect(within(authSection).getByDisplayValue('auth-from-cookie')).toBeInTheDocument();
  expect(within(authSection).getByDisplayValue('ct0-from-cookie')).toBeInTheDocument();
});

it('saves pasted full cookie text with extracted token fields', async () => {
  const user = userEvent.setup();
  const savedBodies: Array<{ cookie_header?: string; auth_token?: string; ct0?: string }> = [];
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({ status: { running: false, pending_creators: 0, total_creators: 0, succeeded_creators: 0, failed_creators: 0, added_works: 0, progress: 0, state: 'idle', errors: [], jobs: [] } });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({ jobs: [], page: 1, page_size: 10, total: 0, total_pages: 0 });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/update/auth' && init?.method === 'PATCH') {
      const savedBody = JSON.parse(String(init.body)) as { cookie_header?: string; auth_token?: string; ct0?: string; download_mode?: string; download_directory?: string };
      savedBodies.push(savedBody);
      return jsonResponse({ auth: { configured: true, auth_token: savedBody?.auth_token, ct0: savedBody?.ct0, cookie_header_saved: true, proxy: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({ auth: { configured: false, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  const authSection = await screen.findByRole('region', { name: 'Twitter/X 登录配置' });
  const cookieText = 'auth_token=auth-from-cookie; ct0=ct0-from-cookie; guest_id=guest';
  await user.click(within(authSection).getByLabelText('Cookie JSON'));
  await user.paste(cookieText);
  await user.click(within(authSection).getByRole('button', { name: '提取 Cookie' }));
  await user.click(within(authSection).getByRole('button', { name: '保存配置' }));

  const savedBody = savedBodies[savedBodies.length - 1];
  expect(savedBody?.auth_token).toBe('auth-from-cookie');
  expect(savedBody?.ct0).toBe('ct0-from-cookie');
  expect(savedBody?.cookie_header).toBe(cookieText);
});

it('saves custom update download directory selected from browser', async () => {
  const user = userEvent.setup();
  const savedBodies: Array<{ download_mode?: string; download_directory?: string }> = [];
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === '/api/update/status') {
      return jsonResponse({ status: { running: false, pending_creators: 0, total_creators: 0, succeeded_creators: 0, failed_creators: 0, added_works: 0, progress: 0, state: 'idle', errors: [], jobs: [] } });
    }
    if (url.startsWith('/api/update/jobs')) {
      return jsonResponse({ jobs: [], page: 1, page_size: 10, total: 0, total_pages: 0 });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [] });
    }
    if (url === '/api/fs/roots') {
      return jsonResponse({ roots: [{ name: 'tmp', path: '/tmp' }] });
    }
    if (url === '/api/fs/list?path=%2Ftmp') {
      return jsonResponse({ entries: [{ name: 'twitter-updates', path: '/tmp/twitter-updates' }] });
    }
    if (url === '/api/update/auth' && init?.method === 'PATCH') {
      const savedBody = JSON.parse(String(init.body)) as { download_mode?: string; download_directory?: string };
      savedBodies.push(savedBody);
      return jsonResponse({ auth: { configured: false, proxy: '', download_mode: savedBody.download_mode, download_directory: savedBody.download_directory, fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
    }
    if (url === '/api/update/auth') {
      return jsonResponse({ auth: { configured: false, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
    }
    return jsonResponse({}, { status: 404 });
  });

  renderUpdatesPage();

  const authSection = await screen.findByRole('region', { name: 'Twitter/X 登录配置' });
  await user.click(within(authSection).getByRole('radio', { name: '自定义保存目录' }));
  await user.click(within(authSection).getByRole('button', { name: '浏览保存目录' }));
  await user.click(await screen.findByText('tmp'));
  await user.click(await screen.findByText('twitter-updates'));
  await user.click(screen.getByRole('button', { name: '选择此目录' }));
  expect(within(authSection).getByDisplayValue('/tmp/twitter-updates')).toBeInTheDocument();

  await user.click(within(authSection).getByRole('button', { name: '保存配置' }));

  const savedBody = savedBodies[savedBodies.length - 1];
  expect(savedBody?.download_mode).toBe('custom');
  expect(savedBody?.download_directory).toBe('/tmp/twitter-updates');
});
