import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { FavoriteWorksPage } from './FavoritesPage';

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

beforeEach(() => {
  vi.spyOn(window, 'fetch').mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/favorite-folders?type=work') {
      return jsonResponse({
        folders: [{ id: 7, user_id: 1, type: 'work', name: '默认作品收藏夹', is_default: true, created_at: '', updated_at: '' }],
      });
    }
    if (url === '/api/favorites/works?folder_id=7') {
      return jsonResponse({
        works: [
          {
            id: 11,
            directory_id: 1,
            creator_id: 1,
            creator_name: 'dahuoluowan',
            aweme_id: 'a',
            title: '有封面作品',
            description: 'desc',
            music_title: '',
            source_url: '',
            cover_url: '/api/works/11/cover',
            tags: [],
            published_at: '',
            media: [{ id: 1, work_id: 11, type: 'video', file_name: 'a.mp4', url: '/api/media/1', ordinal: 0 }],
          },
          {
            id: 12,
            directory_id: 1,
            creator_id: 2,
            creator_name: 'DamiDamie233',
            aweme_id: 'b',
            title: '无封面作品',
            description: 'desc',
            music_title: '',
            source_url: '',
            tags: [],
            published_at: '',
            media: [{ id: 2, work_id: 12, type: 'video', file_name: 'b.mp4', url: '/api/media/2', ordinal: 0 }],
          },
        ],
      });
    }
    return jsonResponse({}, { status: 404 });
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

function renderFavoriteWorks() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/favorite-works/7']}>
        <Routes>
          <Route path="/favorite-works/:folderId" element={<FavoriteWorksPage />} />
          <Route path="/favorite-works/:favoriteFolderId/play" element={<div>收藏夹播放器</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

it('renders favorite works as a cover grid and opens the folder player from the clicked work', async () => {
  const user = userEvent.setup();
  renderFavoriteWorks();

  const cover = await screen.findByAltText('有封面作品');
  expect(cover).toHaveAttribute('src', '/api/works/11/cover');
  expect(screen.getByText('无封面')).toBeInTheDocument();
  expect(screen.getByText('有封面作品').closest('.work-card')).toHaveAttribute('href', '/favorite-works/7/play?work_id=11');

  await user.click(screen.getByText('有封面作品'));
  expect(await screen.findByText('收藏夹播放器')).toBeInTheDocument();
});
