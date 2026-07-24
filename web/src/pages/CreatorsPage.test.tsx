import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { CreatorsPage } from './CreatorsPage';

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

const adminUser = { id: 1, username: 'ted', is_admin: true, created_at: '', updated_at: '' };
let scanRequests: string[] = [];

beforeEach(() => {
  scanRequests = [];
  class EventSourceMock {
    addEventListener = vi.fn();
    close = vi.fn();
  }
  Object.defineProperty(window, 'EventSource', { configurable: true, value: EventSourceMock });
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === '/api/creators/bulk-import/preview') {
      return jsonResponse({ items: [{ input: '@new_creator', profile_url: 'https://x.com/new_creator', username: 'new_creator', status: 'ready', existing: false }] });
    }
    if (url === '/api/creators/bulk-import') {
      return jsonResponse({ creators: [{ id: 3, directory_id: 1, name: 'new_creator', work_count: 0, twitter_username: 'new_creator', twitter_profile_url: 'https://x.com/new_creator', online_identity_status: 'ready' }] }, { status: 201 });
    }
    if (url === '/api/creators/1/scan' && init?.method === 'POST') {
      scanRequests.push(url);
      return jsonResponse({ status: { running: true, queue_length: 0, current_creator: 'mosenin_ho', scanned_creators: 0, total_creators: 1, scanned_works: 0, progress: 0, state: 'scanning', errors: [] } }, { status: 202 });
    }
    if (url === '/api/creators/1/links' && init?.method === 'PUT') {
      return jsonResponse({
        linked: [{ id: 2, directory_id: 1, name: 'DamiDamie233', work_count: 8, view_count: 1500, last_work_created_at: '2026-05-01T00:00:00Z', online_identity_status: 'missing' }],
        candidates: [
          { id: 2, directory_id: 1, name: 'DamiDamie233', work_count: 8, view_count: 1500, last_work_created_at: '2026-05-01T00:00:00Z', online_identity_status: 'missing' },
          { id: 4, directory_id: 1, name: 'empty_creator', work_count: 0, view_count: 999, last_work_created_at: '', online_identity_status: 'ready' },
        ],
      });
    }
    if (url === '/api/creators/1/links') {
      return jsonResponse({
        linked: [],
        candidates: [
          { id: 2, directory_id: 1, name: 'DamiDamie233', work_count: 8, view_count: 1500, last_work_created_at: '2026-05-01T00:00:00Z', online_identity_status: 'missing', bio: '新名字' },
          { id: 4, directory_id: 1, name: 'empty_creator', work_count: 0, view_count: 999, last_work_created_at: '', online_identity_status: 'ready' },
        ],
      });
    }
    if (url.startsWith('/api/creators')) {
      // Handle creator-links separately
      if (url.includes('/links')) {
        return jsonResponse({
          linked: [],
          candidates: [
            { id: 2, directory_id: 1, name: 'DamiDamie233', work_count: 8, view_count: 1500, last_work_created_at: '2026-05-01T00:00:00Z', online_identity_status: 'missing', bio: '新名字' },
            { id: 4, directory_id: 1, name: 'empty_creator', work_count: 0, view_count: 999, last_work_created_at: '', online_identity_status: 'ready' },
          ],
        });
      }
      const parsedUrl = new URL(url, 'http://localhost');
      const search = parsedUrl.searchParams.get('search') || '';
      const sortField = parsedUrl.searchParams.get('sort_field') || 'updated';
      const sortDir = parsedUrl.searchParams.get('sort_direction') || 'desc';
      const minWork = parseInt(parsedUrl.searchParams.get('min_work_count') || '0', 10);
      const page = parseInt(parsedUrl.searchParams.get('page') || '1', 10);
      const pageSize = parseInt(parsedUrl.searchParams.get('page_size') || '50', 10);

      let filtered = [...allCreators].filter((c) => {
        if (search && !c.name.toLowerCase().includes(search.toLowerCase())) return false;
        if (minWork > 0 && c.work_count < minWork) return false;
        return true;
      });

      filtered.sort((a, b) => {
        let cmp = 0;
        if (sortField === 'views') cmp = a.view_count - b.view_count;
        else if (sortField === 'updated') {
          const aTime = a.last_work_created_at || '';
          const bTime = b.last_work_created_at || '';
          if (!aTime && bTime) cmp = 1;
          else if (aTime && !bTime) cmp = -1;
          else cmp = aTime.localeCompare(bTime);
        } else cmp = a.name.localeCompare(b.name);
        return sortDir === 'desc' ? -cmp : cmp;
      });

      const total = filtered.length;
      const totalPages = Math.max(1, Math.ceil(total / pageSize));
      const offset = (page - 1) * pageSize;
      const creators = filtered.slice(offset, offset + pageSize);

      return jsonResponse({ creators, page, page_size: pageSize, total, total_pages: totalPages });
    }
    if (url === '/api/directories') {
      return jsonResponse({ directories: [{ id: 1, path: '/tmp/twitter', name: 'twitter', status: 'idle', creator_count: 2, work_count: 20, created_at: '' }] });
    }
    if (url === '/api/update/queues') {
      return jsonResponse({ queues: [{ id: 1, name: '默认更新队列', cron: '0 3 * * *', is_default: true, enabled: true, next_run_at: '', last_run_summary: '', subscribed_count: 0, created_at: '', updated_at: '' }] });
    }
    if (url === '/api/update/immediate' && init?.method === 'POST') {
      return jsonResponse({ status: { running: false, pending_creators: 1, total_creators: 1, succeeded_creators: 0, failed_creators: 0, added_works: 0, jobs: [] } });
    }
    if (url === '/api/update/queues/1/creators' && init?.method === 'POST') {
      return jsonResponse({ ok: true });
    }
    if (url.startsWith('/api/fs/')) {
      return jsonResponse({ roots: [], entries: [] });
    }
    return jsonResponse({}, { status: 404 });
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

function renderCreatorsPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <CreatorsPage currentUser={adminUser} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const allCreators = [
  { id: 1, directory_id: 1, name: 'mosenin_ho', work_count: 12, view_count: 12500, last_work_created_at: '2026-06-01T00:00:00Z', online_identity_status: 'ready', avatar_url: '/api/creators/1/avatar', bio: '原创视频' },
  { id: 2, directory_id: 1, name: 'DamiDamie233', work_count: 8, view_count: 1500, last_work_created_at: '2026-05-01T00:00:00Z', online_identity_status: 'missing', bio: '新名字' },
  { id: 4, directory_id: 1, name: 'empty_creator', work_count: 0, view_count: 999, last_work_created_at: '', online_identity_status: 'ready' },
];

it('filters creators by name and shows empty match state', async () => {
  const user = userEvent.setup();
  renderCreatorsPage();
  expect(await screen.findByText('mosenin_ho')).toBeInTheDocument();
  expect(screen.getByText('DamiDamie233')).toBeInTheDocument();
  expect(screen.queryByText('empty_creator')).not.toBeInTheDocument();

  await user.type(screen.getByLabelText('搜索博主'), 'dami');
  expect(screen.queryByText('mosenin_ho')).not.toBeInTheDocument();
  expect(screen.getByText('DamiDamie233')).toBeInTheDocument();

  await user.clear(screen.getByLabelText('搜索博主'));
  await user.type(screen.getByLabelText('搜索博主'), 'unknown');
  expect(screen.getByText('没有匹配的博主。')).toBeInTheDocument();
});

it('shows compact view counts and sorts creators', async () => {
  const user = userEvent.setup();
  renderCreatorsPage();

  expect(await screen.findByText('播放 13K')).toBeInTheDocument();
  expect(screen.getByText('播放 1.5K')).toBeInTheDocument();
  let names = screen.getAllByText(/mosenin_ho|DamiDamie233/).map((node) => node.textContent);
  expect(names.slice(0, 2)).toEqual(['mosenin_ho', 'DamiDamie233']);

  await user.click(screen.getByLabelText('按作品数过滤'));
  await user.selectOptions(screen.getByLabelText('排序方式'), 'views');
  await user.click(screen.getByRole('button', { name: '切换排序方向' }));
  names = screen.getAllByText(/mosenin_ho|DamiDamie233|empty_creator/).map((node) => node.textContent);
  expect(names.slice(0, 3)).toEqual(['empty_creator', 'DamiDamie233', 'mosenin_ho']);

  await user.selectOptions(screen.getByLabelText('排序方式'), 'name');
  names = screen.getAllByText(/mosenin_ho|DamiDamie233|empty_creator/).map((node) => node.textContent);
  expect(names.slice(0, 3)).toEqual(['DamiDamie233', 'empty_creator', 'mosenin_ho']);
});

it('filters creators by minimum work count', async () => {
  const user = userEvent.setup();
  renderCreatorsPage();

  expect(await screen.findByText('mosenin_ho')).toBeInTheDocument();
  expect(screen.queryByText('empty_creator')).not.toBeInTheDocument();
  expect(screen.getByLabelText('最小作品数')).toHaveValue(1);

  await user.click(screen.getByLabelText('按作品数过滤'));
  expect(await screen.findByText('empty_creator')).toBeInTheDocument();

  await user.click(screen.getByLabelText('按作品数过滤'));
  await user.clear(screen.getByLabelText('最小作品数'));
  await user.type(screen.getByLabelText('最小作品数'), '10');

  expect(screen.getByText('mosenin_ho')).toBeInTheDocument();
  expect(screen.queryByText('DamiDamie233')).not.toBeInTheDocument();
  expect(screen.queryByText('empty_creator')).not.toBeInTheDocument();
});

it('shows admin batch update subscribe and bulk import actions', async () => {
  const user = userEvent.setup();
  renderCreatorsPage();

  expect(await screen.findByRole('button', { name: /批量选择/ })).toBeInTheDocument();
  expect(await screen.findByText('mosenin_ho')).toBeInTheDocument();
  expect(screen.getAllByRole('button', { name: /立即更新/ })[0]).toBeDisabled();
  expect(screen.getByRole('button', { name: /批量订阅/ })).toBeDisabled();
  expect(screen.getByRole('button', { name: /批量新增关注/ })).toBeInTheDocument();
  expect(screen.getAllByRole('button', { name: '立即更新' }).length).toBeGreaterThan(0);
  expect(screen.getAllByRole('button', { name: '订阅更新' }).length).toBeGreaterThan(0);
  expect(screen.getAllByRole('button', { name: '立即扫描' }).length).toBeGreaterThan(0);
  expect(screen.getAllByRole('button', { name: '关联推主' }).length).toBeGreaterThan(0);
  expect(screen.getByText('原创视频')).toBeInTheDocument();
  expect(screen.getByAltText('mosenin_ho 头像')).toHaveAttribute('src', '/api/creators/1/avatar');
  expect(screen.getAllByText(/可在线更新/).length).toBeGreaterThanOrEqual(2);
  expect(screen.queryByText(/需补充主页信息/)).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: /批量选择/ }));
  await user.click(screen.getByLabelText('选择-mosenin_ho'));
  await user.click(screen.getAllByRole('button', { name: /立即更新/ })[0]);
  expect(await screen.findByText(/已加入立即更新队列/)).toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: /批量选择/ }));
  await user.click(screen.getByLabelText('选择-DamiDamie233'));
  await user.click(screen.getByRole('button', { name: /批量订阅/ }));
  expect(await screen.findByRole('dialog', { name: '订阅更新' })).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: '保存订阅' }));
  expect(await screen.findByText(/已保存订阅/)).toBeInTheDocument();
});

it('saves linked creators from the creator link dialog', async () => {
  const user = userEvent.setup();
  renderCreatorsPage();

  expect(await screen.findByText('mosenin_ho')).toBeInTheDocument();
  await user.click(screen.getAllByRole('button', { name: '关联推主' })[0]);
  expect(await screen.findByRole('dialog', { name: '关联推主' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '关闭关联推主' })).toBeInTheDocument();
  await user.click(screen.getByLabelText(/DamiDamie233/));
  await user.click(screen.getByRole('button', { name: '保存关联' }));

  await screen.findByText('mosenin_ho');
  expect(fetch).toHaveBeenCalledWith(
    '/api/creators/1/links',
    expect.objectContaining({
      method: 'PUT',
      body: JSON.stringify({ creator_ids: [2] }),
    }),
  );
});

it('enqueues a single creator scan from creator actions', async () => {
  const user = userEvent.setup();
  renderCreatorsPage();

  expect(await screen.findByText('mosenin_ho')).toBeInTheDocument();
  await user.click(screen.getAllByRole('button', { name: '立即扫描' })[0]);

  expect(await screen.findByText('已加入扫描队列：mosenin_ho')).toBeInTheDocument();
  expect(scanRequests).toEqual(['/api/creators/1/scan']);
});

it('previews and imports twitter creators from the bulk import dialog', async () => {
  const user = userEvent.setup();
  renderCreatorsPage();

  await user.click(await screen.findByRole('button', { name: /批量新增关注/ }));
  expect(screen.getByRole('dialog', { name: '批量新增关注' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '关闭批量新增关注' })).toBeInTheDocument();
  await user.type(screen.getByLabelText('批量关注文本'), '@new_creator');
  await user.click(screen.getByRole('button', { name: '解析' }));
  expect(await screen.findByText('https://x.com/new_creator')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: '提交' }));
  expect(await screen.findByText(/已导入 1 个博主/)).toBeInTheDocument();
});
