import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { useParams, useSearchParams } from 'react-router-dom';
import { api } from '../lib/api';
import { FeedViewer } from '../components/FeedViewer';

export function FeedPage() {
  const params = useParams();
  const [searchParams] = useSearchParams();
  const creatorId = params.creatorId ? Number(params.creatorId) : undefined;
  const favoriteFolderId = params.favoriteFolderId ? Number(params.favoriteFolderId) : undefined;
  const initialWorkId = searchParams.get('work_id') ? Number(searchParams.get('work_id')) : undefined;
  const [search, setSearch] = useState('');
  const feed = useQuery({
    queryKey: ['feed', creatorId ?? 'random', favoriteFolderId ?? 'all', search],
    queryFn: () => api.feed({ creatorId, favoriteFolderId, search, limit: 40 }),
  });

  return (
    <div className="page feed-page">
      {feed.isLoading ? (
        <div className="empty-state">加载中...</div>
      ) : (
        <FeedViewer works={feed.data?.works ?? []} search={search} onSearch={setSearch} enableFavorites initialWorkId={initialWorkId} />
      )}
    </div>
  );
}
