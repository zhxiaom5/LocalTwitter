import { test, expect } from '@playwright/test';
import type { Locator, Page, Route } from '@playwright/test';

async function fulfillAuth(route: Route, url: URL) {
  if (url.pathname === '/api/auth/me') {
    await route.fulfill({ json: { user: { id: 1, username: 'ted', role: 'super_admin', is_admin: true, is_super_admin: true, can_update: true, created_at: '', updated_at: '' } } });
    return true;
  }
  return false;
}

async function fulfillCommon(route: Route, url: URL) {
  if (await fulfillAuth(route, url)) return true;
  if (url.pathname === '/api/events') {
    await route.abort();
    return true;
  }
  if (url.pathname === '/api/favorites/status') {
    await route.fulfill({
      json: {
        status: {
          work_folder_ids: [],
          creator_folder_ids: [],
          work_favorited: false,
          creator_favorited: false,
        },
      },
    });
    return true;
  }
  if (url.pathname === '/api/favorite-folders') {
    const type = url.searchParams.get('type') ?? 'work';
    await route.fulfill({
      json: {
        folders: [
          {
            id: type === 'creator' ? 2 : 1,
            type,
            name: type === 'creator' ? '默认作者收藏夹' : '默认作品收藏夹',
            is_default: true,
            created_at: '',
            updated_at: '',
          },
        ],
      },
    });
    return true;
  }
  return false;
}

test('settings page supports directory workflow shell', async ({ page }) => {
  let savedConcurrency = 0;
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (await fulfillCommon(route, url)) return;
    if (url.pathname === '/api/directories') {
      await route.fulfill({ json: { directories: [{ id: 1, path: '/sample/twitter', name: 'twitter', status: 'idle', creator_count: 1, work_count: 1, created_at: '' }] } });
      return;
    }
    if (url.pathname === '/api/config/database') {
      await route.fulfill({ json: { database: { directory: '/sample/db', path: '/sample/db/localtwitter.db', exists: true, writable: true } } });
      return;
    }
    if (url.pathname === '/api/scan/status') {
      await route.fulfill({ json: { status: { running: true, queue_length: 0, current_directory: '/sample/twitter', current_creator: 'creator', scanned_creators: 1, total_creators: 1, scanned_works: 1, progress: 100, state: 'completed', errors: [] } } });
      return;
    }
    if (url.pathname === '/api/update/auth') {
      if (route.request().method() === 'PATCH') {
        const body = route.request().postDataJSON() as { downloader_concurrency?: number };
        savedConcurrency = body.downloader_concurrency ?? 0;
        await route.fulfill({ json: { auth: { configured: false, downloader_concurrency: savedConcurrency, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } } });
        return;
      }
      await route.fulfill({ json: { auth: { configured: false, downloader_concurrency: 1, proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false, validation_status: 'unknown' } } });
      return;
    }
    if (url.pathname === '/api/fs/roots') {
      await route.fulfill({ json: { roots: [{ path: '/sample', name: 'sample' }] } });
      return;
    }
    await route.fulfill({ json: { ok: true, status: { running: true, progress: 0 } } });
  });

  await page.goto('/settings');
  await expect(page.getByText('目录配置')).toBeVisible();
  await expect(page.locator('.config-line').first()).toContainText('localtwitter.db');
  await expect(page.locator('.directory-row')).toContainText('/sample/twitter');
  await expect(page.getByText('creator')).toBeVisible();
  await page.getByLabel('下载器并发数').fill('2');
  await page.getByLabel('下载器并发数').press('Enter');
  await expect.poll(() => savedConcurrency).toBe(2);
  await page.getByRole('button', { name: /浏览/ }).nth(1).click();
  await expect(page.getByRole('dialog')).toBeVisible();
});

test('immersive feed shows overlays and sends search query', async ({ page }) => {
  let searched = false;
  let creatorScoped = false;
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (await fulfillCommon(route, url)) return;
    if (url.pathname === '/api/feed') {
      searched = url.searchParams.get('search') === '夏天';
      creatorScoped = url.searchParams.get('creator_id') === '1';
      await route.fulfill({
        json: {
          works: [
            {
              id: 1,
              directory_id: 1,
              creator_id: 1,
              creator_name: '阿言',
              aweme_id: '1001',
              title: '夏天',
              description: '第一条描述 https://pbs.twimg.com/amplify_video_thumb/1/img/a.jpg',
              music_title: '音乐',
              source_url: 'https://x.com/ayan/status/1001',
              tags: ['测试'],
              published_at: '',
              media: [{ id: 1, work_id: 1, type: 'video', file_name: 'a.mp4', url: 'data:video/mp4;base64,', ordinal: 0 }],
            },
          ],
        },
      });
      return;
    }
    await route.fulfill({ json: { ok: true } });
  });

  await page.goto('/');
  await expect(page.getByLabel('作品播放器')).toBeVisible();
  await page.getByLabel('作品播放器').hover();
  await expect(page.getByLabel('打开搜索')).toBeVisible();
  await page.getByLabel('打开搜索').click();
  await expect(page.getByLabel('搜索本地作品')).toBeVisible();
  await expect(page.getByLabel('作品操作')).toBeVisible();
  await expect(page.locator('.bottom-meta')).toBeVisible();
  await expect(page.locator('.creator-info-panel')).toBeVisible();
  await expect(page.locator('.top-meta')).toHaveCount(0);
  await expect(page.locator('.full-video-timeline')).toBeVisible();
  await expect(page.getByLabel('更多播放控制')).toBeVisible();
  await expect(page.getByRole('link', { name: '查看封面' }).first()).toBeVisible();
  await expect(page.getByRole('link', { name: '查看原推文' })).toBeVisible();
  await expect(page.getByText(/pbs\.twimg\.com/)).toHaveCount(0);
  await expect(page.getByText(/x\.com\/ayan\/status/)).toHaveCount(0);
  await expect(page.getByText('1/2')).toHaveCount(0);
  await page.getByRole('link', { name: '@阿言' }).click();
  await expect(page).toHaveURL(/\/creator\/1$/);
  await expect.poll(() => creatorScoped).toBe(true);
  await page.getByLabel('打开搜索').click();
  await page.getByLabel('搜索本地作品').fill('夏天');
  await page.getByRole('button', { name: /搜索/ }).click();
  await expect.poll(() => searched).toBe(true);
});

test('creators page filters creators by name', async ({ page }) => {
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (await fulfillCommon(route, url)) return;
    if (url.pathname === '/api/creators') {
      const search = (url.searchParams.get('search') || '').toLowerCase();
      const creators = [
        { id: 1, directory_id: 1, name: 'mosenin_ho', work_count: 12 },
        { id: 2, directory_id: 1, name: 'DamiDamie233', work_count: 8 },
      ].filter((creator) => !search || creator.name.toLowerCase().includes(search));
      await route.fulfill({
        json: {
          creators,
          page: 1,
          page_size: 50,
          total: creators.length,
          total_pages: creators.length > 0 ? 1 : 0,
        },
      });
      return;
    }
    await route.fulfill({ json: { ok: true } });
  });

  await page.goto('/creators');
  await expect(page.getByText('mosenin_ho')).toBeVisible();
  await expect(page.getByText('DamiDamie233')).toBeVisible();
  await page.getByLabel('搜索博主').fill('dami');
  await expect(page.getByText('DamiDamie233')).toBeVisible();
  await expect(page.getByText('mosenin_ho')).toHaveCount(0);
  await page.getByLabel('搜索博主').fill('unknown');
  await expect(page.getByText('没有匹配的博主。')).toBeVisible();
});

test('creators page enqueues a single creator scan', async ({ page }) => {
  let scannedCreator = false;
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (await fulfillCommon(route, url)) return;
    if (url.pathname === '/api/creators/1/scan') {
      scannedCreator = true;
      await route.fulfill({ status: 202, json: { status: { running: true, queue_length: 0, current_creator: 'mosenin_ho', scanned_creators: 0, total_creators: 1, scanned_works: 0, progress: 0, state: 'scanning', errors: [] } } });
      return;
    }
    if (url.pathname === '/api/creators') {
      await route.fulfill({ json: { creators: [{ id: 1, directory_id: 1, name: 'mosenin_ho', work_count: 12, online_identity_status: 'ready' }] } });
      return;
    }
    if (url.pathname === '/api/directories') {
      await route.fulfill({ json: { directories: [] } });
      return;
    }
    if (url.pathname === '/api/update/queues') {
      await route.fulfill({ json: { queues: [] } });
      return;
    }
    await route.fulfill({ json: { ok: true } });
  });

  await page.goto('/creators');
  await expect(page.getByText('mosenin_ho')).toBeVisible();
  await page.getByRole('button', { name: '立即扫描' }).click();
  await expect(page.getByText('已加入扫描队列：mosenin_ho')).toBeVisible();
  await expect.poll(() => scannedCreator).toBe(true);
});

test('updates page paginates immediate job records', async ({ page }) => {
  const firstPage = Array.from({ length: 10 }, (_, index) => ({
    id: 12 - index,
    creator_id: 1,
    creator_name: `creator-${12 - index}`,
    status: 'succeeded',
    added_works: 0,
    total_tweets: 0,
    processed_tweets: 0,
    downloaded_videos: 0,
    skipped_tweets: 0,
    created_at: '',
  }));
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (await fulfillCommon(route, url)) return;
    if (url.pathname === '/api/update/status') {
      await route.fulfill({ json: { status: { running: false, pending_creators: 0, total_creators: 0, succeeded_creators: 0, failed_creators: 0, added_works: 0, progress: 0, state: 'idle', errors: [], jobs: [] } } });
      return;
    }
    if (url.pathname === '/api/update/jobs' && url.searchParams.get('page') === '2') {
      await route.fulfill({ json: { jobs: [{ id: 2, creator_id: 1, creator_name: 'creator-2', status: 'succeeded', added_works: 0, total_tweets: 0, processed_tweets: 0, downloaded_videos: 0, skipped_tweets: 0, created_at: '' }], page: 2, page_size: 10, total: 12, total_pages: 2 } });
      return;
    }
    if (url.pathname === '/api/update/jobs') {
      await route.fulfill({ json: { jobs: firstPage, page: 1, page_size: 10, total: 12, total_pages: 2 } });
      return;
    }
    if (url.pathname === '/api/update/queues') {
      await route.fulfill({ json: { queues: [] } });
      return;
    }
    if (url.pathname === '/api/update/auth') {
      await route.fulfill({ json: { auth: { configured: false, validation_status: 'unknown', validation_message: '', proxy: '', download_mode: 'original', download_directory: '', fetch_limit: 300, media_only: true, include_retweets: false } } });
      return;
    }
    await route.fulfill({ json: { ok: true } });
  });

  await page.goto('/updates');
  await expect(page.getByText('creator-12')).toBeVisible();
  await expect(page.getByText('第 1 / 2 页 · 共 12 条')).toBeVisible();
  await page.getByRole('button', { name: '下一页' }).click();
  await expect(page.getByText('creator-2')).toBeVisible();
  await expect(page.getByText('第 2 / 2 页 · 共 12 条')).toBeVisible();
});

test('mobile viewport hides sidebar behind a floating menu and supports swipe feed switching', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (await fulfillCommon(route, url)) return;
    if (url.pathname === '/api/feed') {
      await route.fulfill({
        json: {
          works: [
            {
              id: 1,
              directory_id: 1,
              creator_id: 1,
              creator_name: '阿言',
              aweme_id: '1001',
              title: '第一条',
              description: '第一条描述',
              music_title: '',
              source_url: '',
              tags: [],
              published_at: '',
              media: [{ id: 1, work_id: 1, type: 'video', file_name: 'a.mp4', url: 'data:video/mp4;base64,', ordinal: 0 }],
            },
            {
              id: 2,
              directory_id: 1,
              creator_id: 1,
              creator_name: '阿言',
              aweme_id: '1002',
              title: '第二条',
              description: '第二条描述',
              music_title: '',
              source_url: '',
              tags: [],
              published_at: '',
              media: [{ id: 2, work_id: 2, type: 'video', file_name: 'b.mp4', url: 'data:video/mp4;base64,', ordinal: 0 }],
            },
          ],
        },
      });
      return;
    }
    if (url.pathname === '/api/creators') {
      await route.fulfill({ json: { creators: [] } });
      return;
    }
    await route.fulfill({ json: { ok: true } });
  });

  await page.goto('/');
  await expect(page.getByLabel('作品播放器')).toBeVisible();
  await expect(page.getByLabel('打开菜单')).toBeVisible();
  await expect(page.getByLabel('取消静音')).toBeVisible();
  await expect(page.getByTitle('收藏该作品')).toBeVisible();
  await expect(page.getByTitle('收藏该作者')).toBeVisible();
  await expect(page.locator('.action-rail .rail-button')).toHaveCount(3);
  await expect(page.locator('.action-rail .rail-button span').first()).toBeHidden();
  await expect(page.locator('.action-rail .favorite-expand').first()).toBeHidden();
  const mobileRailButtonBoxes = await page.locator('.action-rail .rail-button').evaluateAll((buttons) =>
    buttons.map((button) => {
      const box = button.getBoundingClientRect();
      return { width: box.width, height: box.height };
    }),
  );
  expect(mobileRailButtonBoxes.every((box) => box.width <= 48 && box.height <= 48)).toBe(true);
  const activeVideo = page.locator('.immersive-stage video');
  await expect.poll(() => activeVideo.evaluate((video: HTMLVideoElement) => video.muted)).toBe(true);
  await page.getByLabel('取消静音').click();
  await expect.poll(() => activeVideo.evaluate((video: HTMLVideoElement) => video.muted)).toBe(false);
  await expect(page.getByLabel('静音')).toBeVisible();
  await page.getByLabel('更多播放控制').click();
  await expect(page.locator('.volume-control')).toBeVisible();
  await expect(page.getByLabel('播放设置')).toBeVisible();
  await expect(page.getByLabel('画面设置')).toBeVisible();
  await expect(page.getByLabel('窗口设置')).toBeVisible();
  const advancedBox = await page.locator('.advanced-control-panel').boundingBox();
  expect(advancedBox).not.toBeNull();
  expect((advancedBox?.x ?? 0) + (advancedBox?.width ?? 0)).toBeLessThanOrEqual(390);
  expect(advancedBox?.x).toBeGreaterThanOrEqual(0);
  expect(advancedBox?.width).toBeLessThanOrEqual(390);
  const playBox = await page.locator('.advanced-panel-section[aria-label="播放设置"] .mode-icon-button').boundingBox();
  expect(playBox).not.toBeNull();
  expect(playBox?.width).toBeGreaterThan(120);
  await expect(page.locator('.app-shell')).not.toHaveClass(/mobile-nav-open/);
  await page.getByLabel('打开菜单').click();
  await expect(page.locator('.app-shell')).toHaveClass(/mobile-nav-open/);
  await page.getByTitle('关注').click();
  await expect(page).toHaveURL(/\/creators$/);
  await expect(page.locator('.app-shell')).not.toHaveClass(/mobile-nav-open/);

  await page.goto('/');
  await expect(page.getByText('第一条描述')).toBeVisible();
  await page.locator('.feed-viewer').dispatchEvent('touchstart', {
    touches: [{ identifier: 1, clientX: 180, clientY: 640 }],
  });
  await page.locator('.feed-viewer').dispatchEvent('touchend', {
    changedTouches: [{ identifier: 1, clientX: 176, clientY: 480 }],
  });
  await expect(page.getByText('第二条描述')).toBeVisible();
});

test('mobile management pages keep LocalDouyin card layout', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (await fulfillCommon(route, url)) return;
    if (url.pathname === '/api/favorites/works') {
      await route.fulfill({ json: { works: [] } });
      return;
    }
    if (url.pathname === '/api/favorites/creators') {
      await route.fulfill({ json: { creators: [] } });
      return;
    }
    if (url.pathname === '/api/users') {
      await route.fulfill({ json: { users: [{ id: 1, username: 'ted', role: 'super_admin', is_admin: true, is_super_admin: true, can_update: true, created_at: '', updated_at: '' }] } });
      return;
    }
    if (url.pathname === '/api/update/status') {
      await route.fulfill({
        json: {
          status: {
            running: false,
            current_creator: '',
            progress: 0,
            pending_creators: 0,
            succeeded_creators: 0,
            failed_creators: 0,
            added_works: 0,
            jobs: [],
          },
        },
      });
      return;
    }
    if (url.pathname === '/api/update/jobs') {
      await route.fulfill({ json: { jobs: [], page: 1, page_size: 10, total: 0, total_pages: 0 } });
      return;
    }
    if (url.pathname === '/api/update/queues') {
      await route.fulfill({ json: { queues: [] } });
      return;
    }
    if (url.pathname === '/api/update/auth') {
      await route.fulfill({ json: { auth: { configured: false, validation_status: 'unknown', validation_message: '', proxy: '', fetch_limit: 300, media_only: true, include_retweets: false } } });
      return;
    }
    await route.fulfill({ json: { ok: true } });
  });

  await page.goto('/favorite-works');
  await expect(page.locator('.favorite-manager')).toBeVisible();
  await expect(locatorFitsViewport(page.locator('.favorite-folder-create'), page)).resolves.toBe(true);
  await page.goto('/favorite-creators');
  await expect(page.locator('.favorite-manager')).toBeVisible();
  await expect(locatorFitsViewport(page.locator('.favorite-folder-row').first(), page)).resolves.toBe(true);
  await page.goto('/users');
  await expect(page.locator('.account-layout')).toBeVisible();
  await expect(locatorFitsViewport(page.locator('.account-card').first(), page)).resolves.toBe(true);
  await expect(locatorFitsViewport(page.locator('.user-row').first(), page)).resolves.toBe(true);
  await page.goto('/updates');
  await expect(page.locator('.updates-grid')).toBeVisible();
  await expect(locatorFitsViewport(page.locator('.account-card').first(), page)).resolves.toBe(true);

  await page.getByLabel('打开菜单').click();
  await expect(page.locator('.sidebar-user')).toBeVisible();
  await expect(page.locator('.sidebar')).toHaveCSS('transform', 'matrix(1, 0, 0, 1, 0, 0)');
  await expect(locatorFitsViewport(page.locator('.sidebar-user-info'), page)).resolves.toBe(true);
});

test('mobile login page keeps styled card layout', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (url.pathname === '/api/auth/me') {
      await route.fulfill({ status: 401, json: { error: 'unauthorized' } });
      return;
    }
    await route.fulfill({ json: { ok: true } });
  });

  await page.goto('/login');
  await expect(page.locator('.login-card')).toBeVisible();
  await expect(locatorFitsViewport(page.locator('.login-card'), page)).resolves.toBe(true);
  await expect(page.locator('.login-form label').first()).toHaveCSS('display', 'grid');
});

async function locatorFitsViewport(locator: Locator, page: Page) {
  const box = await locator.boundingBox();
  const viewport = page.viewportSize();
  if (!box || !viewport) return false;
  return box.x >= -1 && box.x + box.width <= viewport.width + 1;
}

test('media keeps natural ratio across viewport sizes', async ({ page }) => {
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (await fulfillCommon(route, url)) return;
    if (url.pathname === '/api/feed') {
      await route.fulfill({
        json: {
          works: [
            {
              id: 1,
              directory_id: 1,
              creator_id: 1,
              creator_name: '阿言',
              aweme_id: '1001',
              title: 'ratio',
              description: 'ratio',
              music_title: '',
              source_url: '',
              tags: [],
              published_at: '',
              media: [
                {
                  id: 1,
                  work_id: 1,
                  type: 'image',
                  file_name: 'ratio.svg',
                  url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22800%22 height=%22400%22 viewBox=%220 0 800 400%22%3E%3Crect width=%22800%22 height=%22400%22 fill=%22%23f43%22/%3E%3C/svg%3E',
                  ordinal: 0,
                },
              ],
            },
          ],
        },
      });
      return;
    }
    await route.fulfill({ json: { ok: true } });
  });

  for (const size of [
    { width: 1280, height: 720 },
    { width: 900, height: 560 },
    { width: 1500, height: 460 },
  ]) {
    await page.setViewportSize(size);
    await page.goto('/');
    const result = await page.locator('.media-content').evaluate((node) => {
      const media = node as HTMLImageElement;
      const mediaBox = media.getBoundingClientRect();
      const stageBox = document.querySelector('.immersive-stage')!.getBoundingClientRect();
      return {
        mediaRatio: mediaBox.width / mediaBox.height,
        naturalRatio: media.naturalWidth / media.naturalHeight,
        fill: Math.max(mediaBox.width / stageBox.width, mediaBox.height / stageBox.height),
        inside:
          mediaBox.left >= stageBox.left - 1 &&
          mediaBox.top >= stageBox.top - 1 &&
          mediaBox.right <= stageBox.right + 1 &&
          mediaBox.bottom <= stageBox.bottom + 1,
      };
    });
    expect(result.inside).toBe(true);
    expect(Math.abs(result.mediaRatio - result.naturalRatio)).toBeLessThan(0.02);
    expect(result.fill).toBeGreaterThan(0.75);
  }
});

test('media arrows fullscreen and wheel lock work reliably', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(HTMLElement.prototype, 'requestFullscreen', {
      configurable: true,
      value: function requestFullscreen() {
        window.localStorage.setItem('fullscreen-requested', 'yes');
        return Promise.resolve();
      },
    });
    Object.defineProperty(document, 'exitFullscreen', {
      configurable: true,
      value: () => Promise.resolve(),
    });
  });
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (await fulfillCommon(route, url)) return;
    if (url.pathname === '/api/feed') {
      await route.fulfill({
        json: {
          works: [
            {
              id: 1,
              directory_id: 1,
              creator_id: 1,
              creator_name: '阿言',
              aweme_id: '1001',
              title: '第一条',
              description: '第一条描述',
              music_title: '',
              source_url: '',
              tags: [],
              published_at: '',
              media: [
                { id: 1, work_id: 1, type: 'image', file_name: 'a.svg', url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22400%22 height=%22600%22%3E%3Crect width=%22400%22 height=%22600%22 fill=%22%23355%22/%3E%3C/svg%3E', ordinal: 0 },
                { id: 2, work_id: 1, type: 'image', file_name: 'b.svg', url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22400%22 height=%22600%22%3E%3Crect width=%22400%22 height=%22600%22 fill=%22%23553%22/%3E%3C/svg%3E', ordinal: 1 },
                { id: 3, work_id: 1, type: 'image', file_name: 'c.svg', url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22400%22 height=%22600%22%3E%3Crect width=%22400%22 height=%22600%22 fill=%22%23535%22/%3E%3C/svg%3E', ordinal: 2 },
              ],
            },
            {
              id: 2,
              directory_id: 1,
              creator_id: 1,
              creator_name: '阿言',
              aweme_id: '1002',
              title: '第二条',
              description: '第二条描述',
              music_title: '',
              source_url: '',
              tags: [],
              published_at: '',
              media: [{ id: 4, work_id: 2, type: 'image', file_name: 'd.svg', url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22400%22 height=%22600%22%3E%3Crect width=%22400%22 height=%22600%22 fill=%22%23333%22/%3E%3C/svg%3E', ordinal: 0 }],
            },
            {
              id: 3,
              directory_id: 1,
              creator_id: 1,
              creator_name: '阿言',
              aweme_id: '1003',
              title: '第三条',
              description: '第三条描述',
              music_title: '',
              source_url: '',
              tags: [],
              published_at: '',
              media: [{ id: 5, work_id: 3, type: 'image', file_name: 'e.svg', url: 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22400%22 height=%22600%22%3E%3Crect width=%22400%22 height=%22600%22 fill=%22%23666%22/%3E%3C/svg%3E', ordinal: 0 }],
            },
          ],
        },
      });
      return;
    }
    await route.fulfill({ json: { ok: true } });
  });

  await page.goto('/');
  await page.getByLabel('作品播放器').hover();
  await expect(page.getByText('1/3')).toBeVisible();
  await page.getByTitle('下一张').click();
  await expect(page.getByText('2/3')).toBeVisible();
  await page.getByTitle('下一张').click();
  await expect(page.getByText('3/3')).toBeVisible();

  await page.getByLabel('作品播放器').dispatchEvent('wheel', { deltaY: 120 });
  await page.getByLabel('作品播放器').dispatchEvent('wheel', { deltaY: 120 });
  await page.getByLabel('作品播放器').dispatchEvent('wheel', { deltaY: 120 });
  await expect(page.getByText('第二条描述')).toBeVisible();
  await expect(page.getByText('第三条描述')).not.toBeVisible();

  await page.getByLabel('网页全屏').click();
  await expect(page.locator('.feed-viewer')).toHaveClass(/page-fullscreen/);
  await expect(page.locator('.sidebar')).toBeHidden();
  await page.getByLabel('退出网页全屏').click();
  await expect(page.locator('.feed-viewer')).not.toHaveClass(/page-fullscreen/);

  await page.getByLabel('完整全屏').click();
  await expect.poll(() => page.evaluate(() => localStorage.getItem('fullscreen-requested'))).toBe('yes');
});

test('overlay toggle hides and restores floating controls', async ({ page }) => {
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (await fulfillCommon(route, url)) return;
    if (url.pathname === '/api/feed') {
      await route.fulfill({
        json: {
          works: [
            {
              id: 1,
              directory_id: 1,
              creator_id: 1,
              creator_name: '阿言',
              aweme_id: '1001',
              title: '第一条',
              description: '第一条描述',
              music_title: '',
              source_url: '',
              tags: [],
              published_at: '',
              media: [{ id: 1, work_id: 1, type: 'video', file_name: 'a.mp4', url: 'data:video/mp4;base64,', ordinal: 0 }],
            },
          ],
        },
      });
      return;
    }
    await route.fulfill({ json: { ok: true } });
  });

  await page.goto('/');
  await page.getByLabel('作品播放器').hover();
  await expect(page.getByLabel('打开搜索')).toBeVisible();
  await page.getByLabel('更多播放控制').click();
  await page.getByLabel('关闭悬浮').click();
  await expect(page.getByLabel('搜索本地作品')).toBeHidden();
  await expect(page.getByLabel('作品操作')).toBeHidden();
  await expect(page.getByLabel('视频控制')).toHaveCount(0);
  await expect(page.getByLabel('悬浮透明度')).toHaveCount(0);
  await expect(page.locator('.player-control-dock')).toHaveCount(0);
  await expect(page.locator('.minimal-progress-dock')).toBeVisible();
  await expect(page.locator('.minimal-progress-dock button')).toHaveCount(0);
  await expect(page.locator('.minimal-progress-dock input')).toHaveCount(1);
  await expect(page.getByLabel('打开悬浮')).toBeVisible();
  await page.getByLabel('打开悬浮').click();
  await expect(page.locator('.minimal-progress-dock')).toHaveCount(0);
  await expect(page.getByLabel('打开搜索')).toBeVisible();
  await expect(page.getByLabel('作品操作')).toBeVisible();
});

test('duplicate bottom metadata is removed and overlay opacity is configurable', async ({ page }) => {
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) {
      await route.fallback();
      return;
    }
    if (await fulfillCommon(route, url)) return;
    if (url.pathname === '/api/feed') {
      await route.fulfill({
        json: {
          works: [
            {
              id: 1,
              directory_id: 1,
              creator_id: 1,
              creator_name: '阿言',
              aweme_id: '1001',
              title: '第一条',
              description: '第一条描述',
              music_title: '',
              source_url: '',
              tags: [],
              published_at: '',
              media: [{ id: 1, work_id: 1, type: 'video', file_name: 'a.mp4', url: 'data:video/mp4;base64,', ordinal: 0 }],
            },
          ],
        },
      });
      return;
    }
    await route.fulfill({ json: { ok: true } });
  });

  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('作品播放器').hover();
  await expect(page.locator('.creator-meta-card')).toHaveCount(0);
  await expect(page.locator('.top-meta')).toHaveCount(0);
  await expect(page.locator('.bottom-meta')).toContainText('@阿言');
  await expect(page.locator('.creator-info-panel')).toContainText('第一条描述');
  await expect(page.locator('.player-control-dock')).toBeVisible();

  await page.getByLabel('更多播放控制').click();
  await page.getByLabel('悬浮透明度').click();
  await page.getByLabel('悬浮透明度数值').fill('0.14');
  await expect(page.getByText('14%')).toBeVisible();
  const opacity = await page.evaluate(() => {
    return {
      stored: localStorage.getItem('localtwitter.overlayOpacity'),
      styleValue: (document.querySelector('.feed-viewer') as HTMLElement).style.getPropertyValue('--overlay-alpha'),
    };
  });
  expect(opacity.stored).toBe('0.14');
  expect(opacity.styleValue).toBe('0.14');

  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.getByLabel('作品播放器').hover();
  const restored = await page.evaluate(() => {
    const viewerEl = document.querySelector('.feed-viewer') as HTMLElement;
    const viewer = viewerEl.getBoundingClientRect();
    const dock = document.querySelector('.player-control-dock')!.getBoundingClientRect();
    return {
      styleValue: viewerEl.style.getPropertyValue('--overlay-alpha'),
      dockWidthRatio: dock.width / viewer.width,
    };
  });
  expect(restored.styleValue).toBe('0.14');
  expect(restored.dockWidthRatio).toBeLessThan(0.45);
});
