import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { LogsPage } from './LogsPage';

beforeEach(() => {
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/logs/audit')) {
      return jsonResponse({
        events: [
          {
            id: 1,
            actor_user_id: 1,
            actor_name: 'ted',
            event_type: 'video_stream',
            target_type: 'media',
            target_id: 7,
            message: '视频请求 very-long-file-name-that-should-not-break-layout.mp4 · 总耗时 1200ms',
            detail_json: JSON.stringify({
              file_name: 'very-long-file-name-that-should-not-break-layout.mp4',
              total_ms: 1200,
              first_write_ms: 80,
              disk_read_ms: 35,
              write_ms: 900,
              write_wait_max_ms: 120,
              read_wait_max_ms: 8,
              play_session_id: 'session-abcdef',
              bytes_sent: 1048576,
            }),
            created_at: '2026-06-17T12:00:00Z',
          },
        ],
        page: 1,
        page_size: 50,
        total: 1,
        total_pages: 1,
      });
    }
    return jsonResponse({ events: [], page: 1, page_size: 50, total: 0, total_pages: 0 });
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

it('renders player timing logs as a compact filename and metrics row', async () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <LogsPage />
    </QueryClientProvider>,
  );

  expect(await screen.findByText('video_stream')).toBeInTheDocument();
  expect(screen.getByText('ted')).toBeInTheDocument();
  expect(screen.getByText('very-long-file-name-that-should-not-break-layout.mp4')).toBeInTheDocument();
  expect(screen.getByText(/总 1200ms/)).toHaveTextContent('首字节 80ms');
  expect(screen.getByText(/总 1200ms/)).toHaveTextContent('写出 900ms');
  expect(screen.getByText(/总 1200ms/)).toHaveTextContent('最大写等 120ms');
  expect(screen.getByText(/总 1200ms/)).toHaveTextContent('会话 session-');
  expect(screen.getByText(/总 1200ms/)).toHaveTextContent('发送 1.0 MB');
});
