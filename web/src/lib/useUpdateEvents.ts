import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';

export function useUpdateEvents() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const source = new EventSource('/api/events', { withCredentials: true });
    source.addEventListener('scan', (message) => {
      const event = JSON.parse((message as MessageEvent).data) as { type?: string };
      queryClient.invalidateQueries({ queryKey: ['scan-status'] });
      queryClient.invalidateQueries({ queryKey: ['directories'] });
      if (event.type === 'creator_completed' || event.type === 'completed') {
        queryClient.invalidateQueries({ queryKey: ['creators'] });
        queryClient.invalidateQueries({ queryKey: ['feed'] });
      }
    });
    source.addEventListener('update', () => {
      queryClient.invalidateQueries({ queryKey: ['update-status'] });
      queryClient.invalidateQueries({ queryKey: ['update-jobs'] });
      queryClient.invalidateQueries({ queryKey: ['update-queues'] });
      queryClient.invalidateQueries({ queryKey: ['directories'] });
      queryClient.invalidateQueries({ queryKey: ['creators'] });
      queryClient.invalidateQueries({ queryKey: ['feed'] });
    });
    return () => source.close();
  }, [queryClient]);
}
