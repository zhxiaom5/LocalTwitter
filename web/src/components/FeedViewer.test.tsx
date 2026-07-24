import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { FeedViewer } from './FeedViewer';
import type { Work } from '../lib/types';
import type { ComponentProps } from 'react';

const works: Work[] = [
  {
    id: 1,
    directory_id: 1,
    creator_id: 1,
    creator_name: '阿言',
    creator_avatar_url: '/api/creators/1/avatar',
    linked_creators: [{ id: 2, name: '新阿言', avatar_url: '/api/creators/2/avatar' }],
    aweme_id: '1001',
    title: '第一条',
    description: '第一条描述 https://pbs.twimg.com/amplify_video_thumb/1/img/a.jpg',
    music_title: '音乐',
    source_url: 'https://x.com/ayan/status/1001',
    cover_url: '/api/works/1/cover',
    tags: ['测试'],
    published_at: '',
    media: [
      { id: 1, work_id: 1, type: 'video', file_name: 'a.mp4', url: '/a.mp4', ordinal: 0 },
      { id: 2, work_id: 1, type: 'video', file_name: 'b.mp4', url: '/b.mp4', ordinal: 1 },
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
    cover_url: '/api/works/2/cover',
    tags: [],
    published_at: '',
    media: [{ id: 3, work_id: 2, type: 'video', file_name: 'c.mp4', url: '/c.mp4', ordinal: 0 }],
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
    cover_url: '/api/works/3/cover',
    tags: [],
    published_at: '',
    media: [{ id: 4, work_id: 3, type: 'video', file_name: 'd.mp4', url: '/d.mp4', ordinal: 0 }],
  },
];
let viewRequests: string[] = [];
let playerLogRequests: Array<Record<string, unknown>> = [];

function installLocalStorageMock() {
  const values = new Map<string, string>();
  Object.defineProperty(window, 'localStorage', {
    configurable: true,
    value: {
      getItem: vi.fn((key: string) => values.get(key) ?? null),
      setItem: vi.fn((key: string, value: string) => values.set(key, value)),
      removeItem: vi.fn((key: string) => values.delete(key)),
      clear: vi.fn(() => values.clear()),
    },
  });
}

beforeEach(() => {
  installLocalStorageMock();
  viewRequests = [];
  playerLogRequests = [];
  Object.defineProperty(navigator, 'maxTouchPoints', { configurable: true, value: 0 });
  Object.defineProperty(HTMLMediaElement.prototype, 'play', { configurable: true, value: vi.fn().mockResolvedValue(undefined) });
  Object.defineProperty(HTMLMediaElement.prototype, 'pause', { configurable: true, value: vi.fn() });
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.startsWith('/api/favorites/status')) {
      return jsonResponse({ status: { work_folder_ids: [], creator_folder_ids: [], work_favorited: false, creator_favorited: false } });
    }
    if (url.startsWith('/api/works/') && url.endsWith('/view') && init?.method === 'POST') {
      viewRequests.push(url);
      return jsonResponse({ ok: true });
    }
    if (url === '/api/logs/player' && init?.method === 'POST') {
      playerLogRequests.push(JSON.parse(String(init.body ?? '{}')) as Record<string, unknown>);
      return jsonResponse({ ok: true });
    }
    if (url === '/api/favorite-folders?type=work') {
      return jsonResponse({ folders: [{ id: 1, type: 'work', name: '默认作品收藏夹', is_default: true, created_at: '', updated_at: '' }] });
    }
    if (url === '/api/favorite-folders?type=creator') {
      return jsonResponse({ folders: [{ id: 2, type: 'creator', name: '默认作者收藏夹', is_default: true, created_at: '', updated_at: '' }] });
    }
    if (url === '/api/favorites/works' && init?.method === 'POST') {
      return jsonResponse({ ok: true, status: { work_folder_ids: [1], creator_folder_ids: [], work_favorited: true, creator_favorited: false } });
    }
    if (url === '/api/favorites/creators' && init?.method === 'POST') {
      return jsonResponse({ ok: true, status: { work_folder_ids: [], creator_folder_ids: [2], work_favorited: false, creator_favorited: true } });
    }
    return jsonResponse({}, { status: 404 });
  });
});

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

function renderFeedViewer(uiWorks: Work[] = works, props: Partial<ComponentProps<typeof FeedViewer>> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <FeedViewer works={uiWorks} {...props} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  window.localStorage?.clear();
  document.cookie = 'localtwitter_overlayOpacity=; max-age=0; path=/';
  vi.useRealTimers();
  vi.restoreAllMocks();
});

it('switches media with arrow buttons and works with keyboard and wheel', async () => {
  const user = userEvent.setup();
  renderFeedViewer();
  expect(screen.getByText('第一条描述')).toBeInTheDocument();
  expect(screen.getByText('1/2')).toBeInTheDocument();
  await user.click(screen.getByTitle('下一张'));
  expect(screen.getByText('2/2')).toBeInTheDocument();
  await user.keyboard('{ArrowDown}');
  expect(screen.getByText('第二条描述')).toBeInTheDocument();
  fireEvent.wheel(screen.getByLabelText('作品播放器'), { deltaY: -100 });
  expect(screen.getByText('第一条描述')).toBeInTheDocument();
});

it('does not jump multiple works during one wheel gesture', async () => {
  renderFeedViewer();
  const viewer = screen.getByLabelText('作品播放器').closest('.feed-viewer')!;
  fireEvent.wheel(viewer, { deltaY: 120 });
  fireEvent.wheel(viewer, { deltaY: 120 });
  fireEvent.wheel(viewer, { deltaY: 120 });
  expect(screen.getByText('第二条描述')).toBeInTheDocument();
  expect(screen.queryByText('第三条描述')).not.toBeInTheDocument();
  await new Promise((resolve) => window.setTimeout(resolve, 460));
  fireEvent.wheel(viewer, { deltaY: 120 });
  expect(screen.getByText('第三条描述')).toBeInTheDocument();
});

it('switches works with vertical touch swipes', () => {
  vi.useFakeTimers();
  renderFeedViewer();
  const viewer = screen.getByLabelText('作品播放器').closest('.feed-viewer')!;
  fireEvent.touchStart(viewer, { touches: [{ clientX: 160, clientY: 560 }] });
  fireEvent.touchEnd(viewer, { changedTouches: [{ clientX: 152, clientY: 430 }] });
  expect(screen.getByText('第二条描述')).toBeInTheDocument();
  vi.advanceTimersByTime(451);
  fireEvent.touchStart(viewer, { touches: [{ clientX: 160, clientY: 430 }] });
  fireEvent.touchEnd(viewer, { changedTouches: [{ clientX: 152, clientY: 560 }] });
  expect(screen.getByText('第一条描述')).toBeInTheDocument();
});

it('does not switch works when touch starts on interactive controls', () => {
  renderFeedViewer();
  const moreButton = screen.getByLabelText('更多播放控制');
  fireEvent.touchStart(moreButton, { touches: [{ clientX: 160, clientY: 560 }] });
  fireEvent.touchEnd(moreButton, { changedTouches: [{ clientX: 152, clientY: 430 }] });
  expect(screen.getByText('第一条描述')).toBeInTheDocument();
});

it('uses non-stretching media content class for videos', async () => {
  const user = userEvent.setup();
  renderFeedViewer();
  const firstVideo = document.querySelector('video');
  expect(firstVideo).toHaveClass('media-content');
  await user.keyboard('{ArrowDown}');
  const video = document.querySelector('video');
  expect(video).toHaveClass('media-content');
});

it('keeps media arrows clickable and bounded', async () => {
  const user = userEvent.setup();
  renderFeedViewer();
  expect(screen.getByTitle('上一张')).toBeDisabled();
  await user.click(screen.getByTitle('下一张'));
  expect(screen.getByText('2/2')).toBeInTheDocument();
  expect(screen.getByTitle('下一张')).toBeDisabled();
  await user.click(screen.getByTitle('上一张'));
  expect(screen.getByText('1/2')).toBeInTheDocument();
});

it('activates overlay on hover and submits search', async () => {
  const user = userEvent.setup();
  const onSearch = vi.fn();
  const { container } = renderFeedViewer(works, { onSearch });
  const viewer = container.querySelector('.feed-viewer');
  expect(viewer).not.toHaveClass('overlay-active');
  fireEvent.mouseMove(viewer!);
  expect(viewer).toHaveClass('overlay-active');
  expect(screen.getByLabelText('打开搜索')).toBeInTheDocument();
  await user.click(screen.getByLabelText('打开搜索'));
  await user.type(screen.getByLabelText('搜索本地作品'), '夏天');
  await user.click(screen.getByTitle('搜索'));
  expect(onSearch).toHaveBeenCalledWith('夏天');
});

it('records a work view when video playback starts', async () => {
  renderFeedViewer();
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(viewRequests).toEqual([]);

  let video = document.querySelector('video') as HTMLVideoElement;
  fireEvent.playing(video);
  expect(viewRequests).toEqual(['/api/works/1/view']);
  fireEvent.playing(video);
  expect(viewRequests).toEqual(['/api/works/1/view']);

  fireEvent.wheel(screen.getByLabelText('作品播放器').closest('.feed-viewer')!, { deltaY: 120 });
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(viewRequests).toEqual(['/api/works/1/view']);
  video = document.querySelector('video') as HTMLVideoElement;
  fireEvent.playing(video);
  await waitFor(() => expect(viewRequests).toEqual(['/api/works/1/view', '/api/works/2/view']));
});

it('logs video lifecycle timing with the current media file name once per play session', () => {
  let now = 100;
  vi.spyOn(performance, 'now').mockImplementation(() => now);
  renderFeedViewer();
  const video = document.querySelector('video') as HTMLVideoElement;
  Object.defineProperty(video, 'duration', { configurable: true, value: 90 });
  now = 340;
  fireEvent.loadedMetadata(video);
  now = 410;
  fireEvent.loadedData(video);
  fireEvent.waiting(video);
  fireEvent.stalled(video);
  now = 500;
  fireEvent.seeking(video);
  now = 560;
  fireEvent.seeked(video);
  now = 740;
  fireEvent.canPlay(video);
  fireEvent.canPlay(video);
  expect(playerLogRequests).toHaveLength(0);
  now = 780;
  fireEvent.playing(video);

  expect(playerLogRequests).toHaveLength(1);
  expect(playerLogRequests[0]).toMatchObject({
    action: 'playing',
    work_id: 1,
    media_id: 1,
    file_name: 'a.mp4',
    metadata_load_ms: 240,
    loaded_data_ms: 310,
    can_play_ms: 640,
    playing_ms: 680,
    waiting_count: 1,
    stalled_count: 1,
    seeking_count: 1,
    last_seek_ms: 60,
  });
  expect(playerLogRequests[0].play_session_id).toEqual(expect.any(String));

  now = 900;
  video.currentTime = 10;
  fireEvent.play(video);
  now = 1120;
  fireEvent.playing(video);

  expect(playerLogRequests).toHaveLength(2);
  expect(playerLogRequests[1]).toMatchObject({
    action: 'playing',
    work_id: 1,
    media_id: 1,
    file_name: 'a.mp4',
    playing_ms: 220,
  });
});

it('keeps the video src stable when playback events fire', () => {
  renderFeedViewer();
  const video = document.querySelector('video') as HTMLVideoElement;
  const initialSrc = video.getAttribute('src');

  video.currentTime = 10;
  fireEvent.play(video);
  fireEvent.playing(video);
  fireEvent.pause(video);

  expect(video.getAttribute('src')).toBe(initialSrc);
});

it('prebuffers only the next desktop video with the same URL used for playback', async () => {
  const user = userEvent.setup();
  renderFeedViewer();
  const prebuffers = document.querySelectorAll('.video-prebuffer video');
  expect(prebuffers).toHaveLength(1);
  const prebuffer = document.querySelector('.video-prebuffer video[title="c.mp4"]') as HTMLVideoElement;
  expect(prebuffer).toBeInTheDocument();
  expect(prebuffer.getAttribute('preload')).toBe('auto');
  expect(document.querySelector('.video-prebuffer video[title="d.mp4"]')).not.toBeInTheDocument();
  const prebufferedSrc = prebuffer.getAttribute('src');
  expect(prebufferedSrc).toContain('/c.mp4?play_session_id=');

  await user.keyboard('{ArrowDown}');
  const activeVideo = document.querySelector('.immersive-stage video') as HTMLVideoElement;

  expect(activeVideo.getAttribute('src')).toBe(prebufferedSrc);
});

it('disables hidden video prebuffering on touch devices', () => {
  Object.defineProperty(navigator, 'maxTouchPoints', { configurable: true, value: 5 });

  renderFeedViewer();

  expect(document.querySelectorAll('.video-prebuffer video')).toHaveLength(0);
});

it('does not insert document-level video preload links', () => {
  renderFeedViewer();

  expect(document.head.querySelector('link[rel="preload"][as="video"]')).not.toBeInTheDocument();
});

it('retries autoplay when the new video becomes playable', async () => {
  const play = vi.fn().mockRejectedValueOnce(new Error('not ready')).mockResolvedValue(undefined);
  Object.defineProperty(HTMLMediaElement.prototype, 'play', { configurable: true, value: play });
  renderFeedViewer();
  const video = document.querySelector('video') as HTMLVideoElement;

  expect(play).toHaveBeenCalledTimes(1);
  fireEvent.canPlay(video);

  await waitFor(() => expect(play).toHaveBeenCalledTimes(2));
});

it('renders URLs as short external links without showing raw URLs', () => {
  renderFeedViewer();
  expect(screen.getByRole('link', { name: '@阿言' })).toHaveAttribute('href', '/creator/1');
  expect(screen.getAllByRole('link', { name: '查看封面' })[0]).toHaveAttribute('href', 'https://pbs.twimg.com/amplify_video_thumb/1/img/a.jpg');
  expect(screen.getByRole('link', { name: '查看原推文' })).toHaveAttribute('href', 'https://x.com/ayan/status/1001');
  expect(screen.queryByText(/pbs\.twimg\.com/)).not.toBeInTheDocument();
  expect(screen.queryByText(/x\.com\/ayan\/status/)).not.toBeInTheDocument();
});

it('shows creator avatar and linked creator entries in the bottom metadata', async () => {
  const user = userEvent.setup();
  renderFeedViewer();

  expect(screen.getByAltText('阿言 头像')).toHaveAttribute('src', '/api/creators/1/avatar');
  expect(document.querySelector('.bottom-meta')).toBeInTheDocument();
  expect(document.querySelector('.creator-info-panel')).toBeInTheDocument();
  expect(document.querySelector('.top-meta')).not.toBeInTheDocument();
  await user.click(screen.getByText('展开'));
  expect(screen.getByRole('link', { name: '@新阿言' })).toHaveAttribute('href', '/creator/2');
});

it('does not show media switcher for a single video work', async () => {
  const user = userEvent.setup();
  renderFeedViewer();
  await user.keyboard('{ArrowDown}');
  expect(screen.queryByText('1/2')).not.toBeInTheDocument();
  expect(screen.queryByTitle('下一张')).not.toBeInTheDocument();
});

it('updates video timeline and volume controls', async () => {
  const user = userEvent.setup();
  renderFeedViewer();
  await user.keyboard('{ArrowDown}');
  const video = document.querySelector('video') as HTMLVideoElement;
  Object.defineProperty(video, 'duration', { configurable: true, value: 120 });
  fireEvent.loadedMetadata(video);
  const timeline = screen.getByLabelText('视频进度');
  fireEvent.change(timeline, { target: { value: '42' } });
  expect(video.currentTime).toBe(42);
  expect(document.querySelector('.full-video-timeline')).toBeInTheDocument();
  await user.click(screen.getByLabelText('更多播放控制'));
  fireEvent.change(screen.getByLabelText('音量'), { target: { value: '0.35' } });
  expect(video.volume).toBe(0.35);
  await user.click(within(screen.getByLabelText('高级播放控制')).getByTitle('静音'));
  expect(video.muted).toBe(true);
});

it('toggles mute from the right action rail and reflects the current state', async () => {
  const user = userEvent.setup();
  renderFeedViewer();
  const video = document.querySelector('video') as HTMLVideoElement;

  expect(video.muted).toBe(true);
  expect(screen.getByLabelText('取消静音')).toBeInTheDocument();
  await user.click(screen.getByLabelText('取消静音'));
  expect(video.muted).toBe(false);
  expect(screen.getByLabelText('静音')).toBeInTheDocument();
  await user.click(screen.getByLabelText('静音'));
  expect(video.muted).toBe(true);
});

it('renders the advanced controls as a structured volume, playback, image, and window panel', async () => {
  const user = userEvent.setup();
  const { container } = renderFeedViewer();

  await user.click(screen.getByLabelText('更多播放控制'));

  const panel = screen.getByLabelText('高级播放控制');
  expect(panel).toBeInTheDocument();
  expect(container.querySelector('.volume-control')).toContainElement(screen.getByLabelText('音量'));
  expect(screen.getByLabelText('播放设置')).toBeInTheDocument();
  expect(screen.getByLabelText('画面设置')).toBeInTheDocument();
  expect(screen.getByLabelText('窗口设置')).toBeInTheDocument();
  expect(screen.getByLabelText('悬浮透明度')).toBeInTheDocument();
  expect(screen.getByLabelText('网页全屏')).toBeInTheDocument();
});

it('updates duration when metadata arrives after the first metadata event', async () => {
  const user = userEvent.setup();
  renderFeedViewer();
  await user.keyboard('{ArrowDown}');
  const video = document.querySelector('video') as HTMLVideoElement;
  Object.defineProperty(video, 'duration', { configurable: true, value: Number.NaN });
  fireEvent.loadedMetadata(video);
  expect(screen.getByText('00:00 / 00:00')).toBeInTheDocument();

  Object.defineProperty(video, 'duration', { configurable: true, value: 93 });
  fireEvent.durationChange(video);

  expect(screen.getByText('00:00 / 01:33')).toBeInTheDocument();
});

it('seeks with the known duration even when the media element duration is temporarily unavailable', async () => {
  const user = userEvent.setup();
  renderFeedViewer();
  await user.keyboard('{ArrowDown}');
  const video = document.querySelector('video') as HTMLVideoElement;
  Object.defineProperty(video, 'duration', { configurable: true, value: 90 });
  fireEvent.loadedMetadata(video);
  Object.defineProperty(video, 'duration', { configurable: true, value: Number.NaN });

  fireEvent.change(screen.getByLabelText('视频进度'), { target: { value: '31' } });

  expect(video.currentTime).toBe(31);
});

it('uses the Douyin-style bottom metadata and compact control dock', () => {
  const { container } = renderFeedViewer();
  const controlDock = container.querySelector('.player-control-dock');
  expect(container.querySelector('.creator-meta-card')).not.toBeInTheDocument();
  expect(container.querySelector('.top-meta')).not.toBeInTheDocument();
  expect(container.querySelector('.bottom-meta')).toHaveTextContent('@阿言');
  expect(container.querySelector('.creator-info-panel')).toHaveTextContent('第一条描述');
  expect(container.querySelector('.full-video-timeline')).toBeInTheDocument();
  expect(controlDock).toBeInTheDocument();
  expect(controlDock).toContainElement(screen.getByLabelText('视频控制'));
  expect(screen.getByLabelText('更多播放控制')).toBeInTheDocument();
});

it('configures overlay opacity and persists it locally', async () => {
  const user = userEvent.setup();
  const { container, unmount } = renderFeedViewer();
  const viewer = container.querySelector('.feed-viewer') as HTMLElement;
  expect(viewer.style.getPropertyValue('--overlay-alpha')).toBe('0.22');
  await user.click(screen.getByLabelText('更多播放控制'));
  await user.click(screen.getByLabelText('悬浮透明度'));
  const slider = screen.getByLabelText('悬浮透明度数值');
  fireEvent.change(slider, { target: { value: '0.36' } });
  expect(viewer.style.getPropertyValue('--overlay-alpha')).toBe('0.36');
  expect(window.localStorage.getItem('localtwitter.overlayOpacity')).toBe('0.36');
  expect(screen.getByText('36%')).toBeInTheDocument();

  unmount();
  const { container: restoredContainer } = renderFeedViewer();
  expect((restoredContainer.querySelector('.feed-viewer') as HTMLElement).style.getPropertyValue('--overlay-alpha')).toBe('0.36');
});

it('toggles page fullscreen and calls native fullscreen api', async () => {
  const user = userEvent.setup();
  const requestFullscreen = vi.fn().mockResolvedValue(undefined);
  const exitFullscreen = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(HTMLElement.prototype, 'requestFullscreen', { configurable: true, value: requestFullscreen });
  Object.defineProperty(document, 'exitFullscreen', { configurable: true, value: exitFullscreen });
  const { container } = renderFeedViewer();
  const viewer = container.querySelector('.feed-viewer')!;
  await user.click(screen.getByLabelText('更多播放控制'));
  await user.click(screen.getByLabelText('网页全屏'));
  expect(viewer).toHaveClass('page-fullscreen');
  await user.click(screen.getByLabelText('退出网页全屏'));
  expect(viewer).not.toHaveClass('page-fullscreen');
  await user.click(screen.getByLabelText('完整全屏'));
  expect(requestFullscreen).toHaveBeenCalledTimes(1);
});

it('keeps a minimal progress bar when player overlays are hidden', async () => {
  const user = userEvent.setup();
  const { container } = renderFeedViewer();
  await user.keyboard('{ArrowDown}');
  const video = document.querySelector('video') as HTMLVideoElement;
  Object.defineProperty(video, 'duration', { configurable: true, value: 60 });
  fireEvent.loadedMetadata(video);
  expect(screen.getByLabelText('打开搜索')).toBeInTheDocument();
  await user.click(screen.getByLabelText('更多播放控制'));
  await user.click(screen.getByLabelText('关闭悬浮'));
  expect(screen.queryByLabelText('搜索本地作品')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('作品操作')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('视频控制')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('悬浮透明度')).not.toBeInTheDocument();
  expect(container.querySelector('.player-control-dock')).not.toBeInTheDocument();
  const minimalDock = container.querySelector('.minimal-progress-dock')!;
  expect(minimalDock).toBeInTheDocument();
  expect(minimalDock.querySelectorAll('input, button')).toHaveLength(1);
  fireEvent.change(screen.getByLabelText('视频进度'), { target: { value: '12' } });
  expect(video.currentTime).toBe(12);
  expect(screen.getByLabelText('打开悬浮')).toBeInTheDocument();
  await user.click(screen.getByLabelText('打开悬浮'));
  expect(container.querySelector('.minimal-progress-dock')).not.toBeInTheDocument();
  expect(screen.getByLabelText('打开搜索')).toBeInTheDocument();
  expect(screen.getByLabelText('作品操作')).toBeInTheDocument();
  expect(screen.getByLabelText('视频控制')).toBeInTheDocument();
  expect(screen.getByLabelText('更多播放控制')).toBeInTheDocument();
  expect(container.querySelector('.player-control-dock')).toBeInTheDocument();
});

it('keeps overlays visible while paused unless manually hidden', async () => {
  const user = userEvent.setup();
  const { container } = renderFeedViewer();
  await user.keyboard('{ArrowDown}');
  const viewer = container.querySelector('.feed-viewer')!;
  const video = document.querySelector('video') as HTMLVideoElement;
  fireEvent.pause(video);
  expect(viewer).toHaveClass('paused');
  await user.click(screen.getByLabelText('更多播放控制'));
  await user.click(screen.getByLabelText('关闭悬浮'));
  expect(viewer).toHaveClass('overlays-hidden');
  fireEvent.pause(video);
  expect(screen.queryByLabelText('视频控制')).not.toBeInTheDocument();
  expect(container.querySelector('.minimal-progress-dock')).toBeInTheDocument();
  expect(screen.getByLabelText('打开悬浮')).toBeInTheDocument();
});

it('keeps only work and creator favorite actions on the right rail with folder menus', async () => {
  const user = userEvent.setup();
  renderFeedViewer(works, { enableFavorites: true });
  const rail = screen.getByLabelText('作品操作');

  expect(await screen.findByTitle('收藏该作品')).toBeInTheDocument();
  expect(screen.getByTitle('收藏该作者')).toBeInTheDocument();
  expect(rail).not.toHaveTextContent('评论');
  expect(rail).not.toHaveTextContent('分享');
  expect(rail).not.toHaveTextContent('来源');
  expect(rail).not.toHaveTextContent('星标');

  await user.click(screen.getAllByTitle('选择收藏夹')[0]);
  expect(await screen.findByText(/默认作品收藏夹/)).toBeInTheDocument();
  await user.click(screen.getAllByTitle('选择收藏夹')[1]);
  expect(await screen.findByText(/默认作者收藏夹/)).toBeInTheDocument();
});

it('keeps the current work when favorite mutations refresh surrounding data', async () => {
  const user = userEvent.setup();
  const { rerender } = renderFeedViewer(works, { enableFavorites: true, initialWorkId: 2 });

  expect(await screen.findByText('第二条描述')).toBeInTheDocument();
  await user.click(await screen.findByTitle('收藏该作品'));

  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  rerender(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <FeedViewer works={[works[2], works[1], works[0]]} enableFavorites initialWorkId={2} />
      </MemoryRouter>
    </QueryClientProvider>,
  );

  expect(screen.getByText('第二条描述')).toBeInTheDocument();
  expect(screen.queryByText('第三条描述')).not.toBeInTheDocument();
});

it('shows empty state without works', () => {
  renderFeedViewer([]);
  expect(screen.getByText(/还没有作品/)).toBeInTheDocument();
});
