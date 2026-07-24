import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { SettingsPage } from './SettingsPage';

vi.mock('../lib/useScanEvents', () => ({ useScanEvents: () => undefined }));

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <SettingsPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === '/api/config/database') {
        return jsonResponse({ database: { directory: '/tmp/db', path: '/tmp/db/localtwitter.db', exists: true, writable: true } });
      }
      if (url === '/api/directories') {
        if (init?.method === 'POST') {
          return jsonResponse({ directory: { id: 1, path: '/tmp/twitter', name: 'twitter', status: 'idle', creator_count: 0, work_count: 0, created_at: '' } });
        }
        return jsonResponse({ directories: [{ id: 1, path: '/tmp/twitter', name: 'twitter', status: 'idle', creator_count: 1, work_count: 2, created_at: '' }] });
      }
      if (url === '/api/scan/status') {
        return jsonResponse({ status: { running: true, queue_length: 0, current_directory: '/tmp/twitter', current_creator: '阿言', scanned_creators: 1, total_creators: 4, scanned_works: 2, progress: 25, state: 'scanning', errors: [] } });
      }
      if (url === '/api/update/auth') {
        if (init?.method === 'PATCH') {
          const body = JSON.parse(String(init.body)) as { downloader_concurrency?: number };
          return jsonResponse({ auth: { configured: false, downloader_concurrency: body.downloader_concurrency ?? 1, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
        }
        return jsonResponse({ auth: { configured: false, downloader_concurrency: 1, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } });
      }
      if (url === '/api/fs/roots') {
        return jsonResponse({ roots: [{ path: '/tmp', name: 'tmp' }] });
      }
      if (url.startsWith('/api/fs/list')) {
        return jsonResponse({ entries: [{ path: '/tmp/twitter', name: 'twitter' }] });
      }
      return jsonResponse({ ok: true, status: { running: true, progress: 0 } });
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

it('renders saved directories and scan progress', async () => {
  renderPage();
  expect(await screen.findByText('twitter')).toBeInTheDocument();
  expect(screen.getAllByText('/tmp/twitter').length).toBeGreaterThan(0);
  expect(screen.getByText(/localtwitter.db/)).toBeInTheDocument();
  expect(screen.getByText('阿言')).toBeInTheDocument();
  expect(screen.getByText('25%')).toBeInTheDocument();
});

it('saves typed path and can open browser', async () => {
  const user = userEvent.setup();
  renderPage();
  await user.type(screen.getByPlaceholderText('/path/to/twitter'), '/tmp/twitter');
  await user.click(screen.getAllByText('保存')[1]);
  await waitFor(() =>
    expect(fetch).toHaveBeenCalledWith(
      '/api/directories',
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ path: '/tmp/twitter' }) }),
    ),
  );
  await user.click(screen.getAllByText('浏览')[1]);
  expect(await screen.findByRole('dialog')).toBeInTheDocument();
});

it('saves database directory', async () => {
  const user = userEvent.setup();
  renderPage();
  await user.type(await screen.findByPlaceholderText('/tmp/db'), '/tmp/db');
  await user.click(screen.getAllByText('保存')[0]);
  await waitFor(() =>
    expect(fetch).toHaveBeenCalledWith(
      '/api/config/database',
      expect.objectContaining({ method: 'PUT', body: JSON.stringify({ directory: '/tmp/db' }) }),
    ),
  );
});

it('saves downloader concurrency', async () => {
  const user = userEvent.setup();
  renderPage();
  const input = await screen.findByLabelText('下载器并发数');
  await waitFor(() => expect(input).toHaveValue(1));
  fireEvent.change(input, { target: { value: '3' } });
  const form = input.closest('form');
  if (!form) throw new Error('downloader concurrency form missing');
  await user.click(within(form).getByRole('button', { name: '保存' }));
  await waitFor(() =>
    expect(fetch).toHaveBeenCalledWith(
      '/api/update/auth',
      expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ downloader_concurrency: 3 }) }),
    ),
  );
});
