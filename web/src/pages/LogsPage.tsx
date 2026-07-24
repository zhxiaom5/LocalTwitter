import { useQuery } from '@tanstack/react-query';
import { FileText, Search } from 'lucide-react';
import { useState } from 'react';
import { api } from '../lib/api';
import type { LogEvent } from '../lib/types';

type LogTab = 'audit' | 'system';

export function LogsPage() {
  const [tab, setTab] = useState<LogTab>('audit');
  const [page, setPage] = useState(1);
  const [keyword, setKeyword] = useState('');
  const [eventType, setEventType] = useState('');
  const params = { page, pageSize: 50, keyword, eventType };
  const logs = useQuery({
    queryKey: ['logs', tab, params],
    queryFn: () => (tab === 'audit' ? api.auditLogs(params) : api.systemLogs(params)),
  });
  const data = logs.data;

  return (
    <div className="page logs-page">
      <header className="page-header">
        <h1>日志记录</h1>
        <p>仅超级管理员可见，记录用户操作和系统运行事件。</p>
      </header>

      <section className="settings-card">
        <div className="toolbar log-toolbar">
          <div className="segmented">
            <button className={tab === 'audit' ? 'active' : ''} type="button" onClick={() => { setTab('audit'); setPage(1); }}>
              操作日志
            </button>
            <button className={tab === 'system' ? 'active' : ''} type="button" onClick={() => { setTab('system'); setPage(1); }}>
              系统日志
            </button>
          </div>
          <label className="inline-search">
            <Search size={16} />
            <input value={keyword} onChange={(event) => { setKeyword(event.target.value); setPage(1); }} placeholder="搜索日志" />
          </label>
          <input className="compact-input" value={eventType} onChange={(event) => { setEventType(event.target.value); setPage(1); }} placeholder="事件类型" />
        </div>

        {logs.isLoading && <div className="empty-state">加载日志中...</div>}
        {logs.error && <p className="error-line">{logs.error.message}</p>}
        {!logs.isLoading && data?.events.length === 0 && <div className="empty-state">暂无日志。</div>}
        <div className="log-list">
          {data?.events.map((event) => (
            <LogRow key={`${tab}-${event.id}`} event={event} system={tab === 'system'} />
          ))}
        </div>
        <div className="pagination-bar">
          <button type="button" disabled={page <= 1} onClick={() => setPage((current) => Math.max(1, current - 1))}>
            上一页
          </button>
          <span>
            第 {data?.page ?? page} / {data?.total_pages || 1} 页 · 共 {data?.total ?? 0} 条
          </span>
          <button type="button" disabled={!data || page >= data.total_pages} onClick={() => setPage((current) => current + 1)}>
            下一页
          </button>
        </div>
      </section>
    </div>
  );
}

function LogRow({ event, system }: { event: LogEvent; system: boolean }) {
  const playerSummary = playerLogSummary(event);
  const title = playerSummary?.title || event.message || event.detail_json || event.event_type;
  return (
    <details className={`log-row ${playerSummary ? 'player-log-row' : ''}`}>
      <summary title={title}>
        <FileText size={16} />
        {playerSummary ? (
          <>
            <time>{formatDate(event.created_at)}</time>
            <strong className="log-main">
              <span className="log-file-name">{playerSummary.fileName}</span>
              <span className="log-event-type">{event.event_type}</span>
            </strong>
            <span>{event.actor_name || (system ? event.severity || 'info' : `用户 ${event.actor_user_id || '-'}`)}</span>
            <span className="log-metrics">{playerSummary.metrics}</span>
          </>
        ) : (
          <>
            <strong>{event.event_type}</strong>
            {!system && <span>{event.actor_name || `用户 ${event.actor_user_id || '-'}`}</span>}
            {system && <span>{event.severity || 'info'}</span>}
            <time>{formatDate(event.created_at)}</time>
          </>
        )}
      </summary>
      <div className="log-detail">
        {event.message && <p>{event.message}</p>}
        {event.target_type && (
          <p>
            目标：{event.target_type} #{event.target_id || '-'}
          </p>
        )}
        {event.detail_json && <pre>{event.detail_json}</pre>}
      </div>
    </details>
  );
}

function playerLogSummary(event: LogEvent) {
  if (event.event_type !== 'player_load' && event.event_type !== 'video_stream' && event.event_type !== 'feed_load') return null;
  const detail = parseDetail(event.detail_json);
  const fileName = String(detail.file_name || (event.event_type === 'feed_load' ? 'Feed 列表' : '未知视频'));
  const metrics: string[] = [];
  pushMetric(metrics, '总', detail.total_ms ?? detail.load_time_ms);
  pushMetric(metrics, '查询', detail.query_ms);
  pushMetric(metrics, 'JSON', detail.json_ms);
  pushMetric(metrics, 'metadata', detail.metadata_load_ms);
  pushMetric(metrics, 'loadeddata', detail.loaded_data_ms);
  pushMetric(metrics, 'canplay', detail.can_play_ms);
  pushMetric(metrics, 'playing', detail.playing_ms);
  pushMetric(metrics, '首帧', detail.first_frame_ms);
  pushMetric(metrics, '首字节', detail.first_write_ms);
  pushMetric(metrics, '读盘', detail.disk_read_ms);
  pushMetric(metrics, '写出', detail.write_ms);
  pushMetric(metrics, '最大写等', detail.write_wait_max_ms);
  pushMetric(metrics, '最大读等', detail.read_wait_max_ms);
  if (Number(detail.waiting_count) > 0) metrics.push(`waiting ${Number(detail.waiting_count)}`);
  if (Number(detail.stalled_count) > 0) metrics.push(`stalled ${Number(detail.stalled_count)}`);
  if (Number(detail.works_count) > 0) metrics.push(`${Number(detail.works_count)} 个作品`);
  if (detail.bytes_sent) metrics.push(`发送 ${formatBytes(Number(detail.bytes_sent))}`);
  if (detail.client_aborted) metrics.push('客户端中断');
  if (detail.play_session_id) metrics.push(`会话 ${shortSession(String(detail.play_session_id))}`);
  return {
    fileName,
    metrics: metrics.join(' · ') || event.message,
    title: `${fileName}\n${event.message}\n${event.detail_json || ''}`.trim(),
  };
}

function shortSession(value: string) {
  return value.length > 10 ? value.slice(0, 8) : value;
}

function parseDetail(value?: string) {
  if (!value) return {} as Record<string, unknown>;
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === 'object' ? parsed as Record<string, unknown> : {};
  } catch {
    return {};
  }
}

function pushMetric(metrics: string[], label: string, value: unknown) {
  const numeric = Number(value);
  if (Number.isFinite(numeric) && numeric > 0) {
    metrics.push(`${label} ${Math.round(numeric)}ms`);
  }
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  if (value < 1024) return `${Math.round(value)} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`;
  return `${(value / 1024 / 1024 / 1024).toFixed(1)} GB`;
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}
