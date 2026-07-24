import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { App } from './App';

class MockEventSource {
  static urls: string[] = [];
  url: string;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  addEventListener = vi.fn();
  removeEventListener = vi.fn();
  close = vi.fn();

  constructor(url: string) {
    this.url = url;
    MockEventSource.urls.push(url);
  }
}

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

beforeEach(() => {
  window.history.pushState({}, '', '/creators');
  MockEventSource.urls = [];
  vi.stubGlobal('EventSource', MockEventSource);
  let loggedIn = true;
  let viewedUserId = 1;
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/auth/me')) {
        if (!loggedIn) return jsonResponse({ error: 'unauthorized' }, { status: 401 });
        return jsonResponse({ user: { id: 1, username: 'ted', role: 'super_admin', is_admin: true, is_super_admin: true, can_update: true, created_at: '', updated_at: '' } });
      }
      if (url.endsWith('/api/auth/logout') && init?.method === 'POST') {
        loggedIn = false;
        return jsonResponse({ ok: true });
      }
      if (url.endsWith('/api/users/view-context') && (!init?.method || init.method === 'GET')) {
        return jsonResponse({
          actor_user: { id: 1, username: 'ted', role: 'super_admin', is_admin: true, is_super_admin: true, can_update: true, created_at: '', updated_at: '' },
          view_user:
            viewedUserId === 2
              ? { id: 2, username: 'alice', role: 'admin', is_admin: false, is_super_admin: false, can_update: true, created_at: '', updated_at: '' }
              : { id: 1, username: 'ted', role: 'super_admin', is_admin: true, is_super_admin: true, can_update: true, created_at: '', updated_at: '' },
        });
      }
      if (url.endsWith('/api/users/view-context') && init?.method === 'PUT') {
        viewedUserId = 2;
        return jsonResponse({
          actor_user: { id: 1, username: 'ted', role: 'super_admin', is_admin: true, is_super_admin: true, can_update: true, created_at: '', updated_at: '' },
          view_user: { id: 2, username: 'alice', role: 'admin', is_admin: false, is_super_admin: false, can_update: true, created_at: '', updated_at: '' },
        });
      }
      if (url.endsWith('/api/users/view-context') && init?.method === 'DELETE') {
        viewedUserId = 1;
        return jsonResponse({
          actor_user: { id: 1, username: 'ted', role: 'super_admin', is_admin: true, is_super_admin: true, can_update: true, created_at: '', updated_at: '' },
          view_user: { id: 1, username: 'ted', role: 'super_admin', is_admin: true, is_super_admin: true, can_update: true, created_at: '', updated_at: '' },
        });
      }
      if (url.endsWith('/api/users')) {
        return jsonResponse({
          users: [
            { id: 1, username: 'ted', role: 'super_admin', is_admin: true, is_super_admin: true, can_update: true, created_at: '', updated_at: '' },
            { id: 2, username: 'alice', role: 'admin', is_admin: false, is_super_admin: false, can_update: true, created_at: '', updated_at: '' },
          ],
        });
      }
      if (url.endsWith('/api/creators')) {
        return jsonResponse({ creators: [] });
      }
      return jsonResponse({ ok: true });
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

it('uses one unified realtime EventSource after login', async () => {
  render(<App />);
  await screen.findByText('ted');

  expect(MockEventSource.urls).toEqual(['/api/events']);
});

it('opens and closes the mobile navigation drawer', async () => {
  const user = userEvent.setup();
  const { container } = render(<App />);
  await screen.findByLabelText('打开菜单');
  const shell = container.querySelector('.app-shell')!;

  expect(shell).not.toHaveClass('mobile-nav-open');
  await user.click(screen.getByLabelText('打开菜单'));
  expect(shell).toHaveClass('mobile-nav-open');
  expect(screen.queryByLabelText('打开菜单')).not.toBeInTheDocument();
  expect(screen.getAllByLabelText('关闭菜单')).toHaveLength(1);

  await user.click(screen.getByLabelText('关闭菜单'));
  expect(shell).not.toHaveClass('mobile-nav-open');
  expect(screen.getByLabelText('打开菜单')).toBeInTheDocument();

  await user.click(screen.getByLabelText('打开菜单'));
  expect(shell).toHaveClass('mobile-nav-open');
  await user.click(screen.getByTitle('关注'));
  expect(shell).not.toHaveClass('mobile-nav-open');
  expect(window.location.pathname).toBe('/creators');
});

it('keeps the desktop sidebar collapse behavior', async () => {
  const user = userEvent.setup();
  const { container } = render(<App />);
  await screen.findByText('ted');
  const shell = container.querySelector('.app-shell')!;

  expect(shell).not.toHaveClass('sidebar-collapsed');
  await user.click(screen.getByLabelText('隐藏菜单'));
  expect(shell).toHaveClass('sidebar-collapsed');
  await user.click(screen.getByLabelText('展开菜单'));
  expect(shell).not.toHaveClass('sidebar-collapsed');
});

it('logs out and redirects to the login page without a manual refresh', async () => {
  const user = userEvent.setup();
  render(<App />);
  await screen.findByText('ted');

  await user.click(screen.getByRole('button', { name: '退出登录' }));

  expect(await screen.findByRole('heading', { name: '登录 LocalTwitter' })).toBeInTheDocument();
  expect(window.location.pathname).toBe('/login');
});

it('lets super admin switch and restore the viewed user from the sidebar', async () => {
  const user = userEvent.setup();
  render(<App />);
  await screen.findByText('ted');

  await user.click(screen.getByRole('button', { name: '切换用户' }));
  expect(await screen.findByRole('dialog', { name: '切换用户' })).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: '切换到 alice' }));

  expect(await screen.findByText('以 alice 身份浏览')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '切回用户' })).toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: '切回用户' }));
  expect(await screen.findByRole('button', { name: '切换用户' })).toBeInTheDocument();
  expect(screen.queryByText('以 alice 身份浏览')).not.toBeInTheDocument();
});
