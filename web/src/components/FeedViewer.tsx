import {
  ChevronLeft,
  ChevronRight,
  Eye,
  EyeOff,
  Expand,
  Heart,
  Maximize2,
  Minimize2,
  MoreHorizontal,
  Pause,
  Play,
  Search,
  SlidersHorizontal,
  Star,
  Volume2,
  VolumeX,
} from 'lucide-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { CSSProperties, Key } from 'react';
import type { FormEvent, MouseEvent } from 'react';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../lib/api';
import type { Work } from '../lib/types';

const OVERLAY_OPACITY_KEY = 'localtwitter.overlayOpacity';
const OVERLAY_OPACITY_COOKIE = 'localtwitter_overlayOpacity';
const DEFAULT_OVERLAY_OPACITY = 0.22;
const MIN_OVERLAY_OPACITY = 0.08;
const MAX_OVERLAY_OPACITY = 0.6;
const TOUCH_SWITCH_DISTANCE = 64;
const TOUCH_AXIS_RATIO = 1.4;

type FeedViewerProps = {
  works: Work[];
  search?: string;
  onSearch?: (keyword: string) => void;
  enableFavorites?: boolean;
  initialWorkId?: number;
};

export function FeedViewer({ works, search = '', onSearch, enableFavorites = false, initialWorkId }: FeedViewerProps) {
  const [workIndex, setWorkIndex] = useState(0);
  const [mediaIndex, setMediaIndex] = useState(0);
  const [query, setQuery] = useState(search);
  const [overlayActive, setOverlayActive] = useState(false);
  const [overlaysHidden, setOverlaysHidden] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [infoExpanded, setInfoExpanded] = useState(false);
  const [isPlaying, setIsPlaying] = useState(true);
  const [muted, setMuted] = useState(true);
  const [volume, setVolume] = useState(0.7);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isPageFullscreen, setIsPageFullscreen] = useState(false);
  const [isNativeFullscreen, setIsNativeFullscreen] = useState(false);
  const [overlayOpacity, setOverlayOpacity] = useState(readOverlayOpacity);
  const overlayTimer = useRef<number | undefined>(undefined);
  const loadStartTimeRef = useRef(0);
  const playSessionIDsRef = useRef(new Map<number, string>());
  const playSessionRef = useRef(0);
  const loggedPlaySessionRef = useRef(0);
  const wheelLockRef = useRef(false);
  const wheelAccumRef = useRef(0);
  const wheelUnlockTimerRef = useRef<number | undefined>(undefined);
  const touchStartRef = useRef<{ x: number; y: number } | null>(null);
  const touchIgnoredRef = useRef(false);
  const touchLockRef = useRef(false);
  const lastInitialWorkIdRef = useRef<number | undefined>(undefined);
  const recordedViewSessionRef = useRef(0);
  const viewerRef = useRef<HTMLDivElement>(null);
  const stageRef = useRef<HTMLElement>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const [stageSize, setStageSize] = useState({ width: 0, height: 0 });
  const [mediaSize, setMediaSize] = useState({ width: 0, height: 0 });
  const metadataLoadMsRef = useRef(0);
  const loadedDataMsRef = useRef(0);
  const canPlayMsRef = useRef(0);
  const playingMsRef = useRef(0);
  const firstFrameMsRef = useRef(0);
  const waitingCountRef = useRef(0);
  const stalledCountRef = useRef(0);
  const seekingCountRef = useRef(0);
  const seekStartRef = useRef(0);
  const lastSeekMsRef = useRef(0);
  const errorCodeRef = useRef(0);
  const work = works[workIndex];
  const media = work?.media[mediaIndex];
  const isVideo = media?.type === 'video';
  const infoSummary = work ? workInfoSummary(work) : '';
  const hasExpandableInfo = Boolean(work && (infoSummary.length > 72 || work.linked_creators?.length || work.tags?.length || work.music_title || work.source_url));
  const workTags = work?.tags ?? [];
  const mediaSessionID = useCallback((mediaID: number) => {
    let sessionID = playSessionIDsRef.current.get(mediaID);
    if (!sessionID) {
      sessionID = createPlaySessionID();
      playSessionIDsRef.current.set(mediaID, sessionID);
    }
    return sessionID;
  }, []);
  const mediaURL = useCallback(
    (item?: { id: number; type: string; url: string }) => {
      if (!item?.url || item.type !== 'video') return item?.url ?? '';
      const separator = item.url.includes('?') ? '&' : '?';
      return `${item.url}${separator}play_session_id=${encodeURIComponent(mediaSessionID(item.id))}`;
    },
    [mediaSessionID],
  );
  const mediaSrc = useMemo(() => mediaURL(media), [media, mediaURL]);
  const playSessionID = media ? mediaSessionID(media.id) : '';
  const prebufferVideos = useMemo(() => {
    if (shouldDisableVideoPrebuffer()) return [];
    const candidates = [workIndex + 1];
    const seen = new Set<number>();
    const items: Array<{ id: number; url: string; file_name: string }> = [];
    for (const idx of candidates) {
      if (idx < 0 || idx >= works.length) continue;
      const candidate = works[idx]?.media.find((item) => item.type === 'video');
      if (!candidate || candidate.id === media?.id || seen.has(candidate.id)) continue;
      seen.add(candidate.id);
      items.push({ id: candidate.id, url: mediaURL(candidate), file_name: candidate.file_name });
    }
    return items;
  }, [media?.id, mediaURL, workIndex, works]);

  useEffect(() => {
    setQuery(search);
  }, [search]);

  useEffect(() => {
    setWorkIndex((current) => {
      if (initialWorkId && initialWorkId !== lastInitialWorkIdRef.current) {
        lastInitialWorkIdRef.current = initialWorkId;
        const initialIndex = works.findIndex((item) => item.id === initialWorkId);
        if (initialIndex >= 0) {
          setMediaIndex(0);
          return initialIndex;
        }
      }
      const currentWorkID = works[current]?.id;
      if (currentWorkID) {
        const nextIndex = works.findIndex((item) => item.id === currentWorkID);
        if (nextIndex >= 0) {
          return nextIndex;
        }
      }
      setMediaIndex(0);
      return 0;
    });
  }, [initialWorkId, works]);

  useEffect(() => {
    document.body.classList.toggle('feed-page-fullscreen', isPageFullscreen);
    return () => document.body.classList.remove('feed-page-fullscreen');
  }, [isPageFullscreen]);

  useEffect(() => {
    const onFullscreenChange = () => setIsNativeFullscreen(document.fullscreenElement === viewerRef.current);
    document.addEventListener('fullscreenchange', onFullscreenChange);
    return () => document.removeEventListener('fullscreenchange', onFullscreenChange);
  }, []);

  useEffect(() => {
    return () => {
      window.clearTimeout(overlayTimer.current);
      window.clearTimeout(wheelUnlockTimerRef.current);
    };
  }, []);

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(([entry]) => {
      const box = entry.contentRect;
      setStageSize({ width: box.width, height: box.height });
    });
    observer.observe(stage);
    const box = stage.getBoundingClientRect();
    setStageSize({ width: box.width, height: box.height });
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    setMediaSize({ width: 0, height: 0 });
    loadStartTimeRef.current = performance.now();
    metadataLoadMsRef.current = 0;
    loadedDataMsRef.current = 0;
    canPlayMsRef.current = 0;
    playingMsRef.current = 0;
    firstFrameMsRef.current = 0;
    waitingCountRef.current = 0;
    stalledCountRef.current = 0;
    seekingCountRef.current = 0;
    seekStartRef.current = 0;
    lastSeekMsRef.current = 0;
    errorCodeRef.current = 0;
    playSessionRef.current += 1;
    loggedPlaySessionRef.current = 0;
    recordedViewSessionRef.current = 0;
    setInfoExpanded(false);
    setSearchOpen(false);
  }, [media?.id]);

  const logPlayerEvent = useCallback((action: string, loadTimeMs: number) => {
    if (!work || !media) return;
    void api.logPlayerAction({
      action,
      play_session_id: playSessionID,
      work_id: work.id,
      media_id: media.id,
      file_name: media.file_name,
      load_time_ms: loadTimeMs,
      metadata_load_ms: metadataLoadMsRef.current,
      loaded_data_ms: loadedDataMsRef.current,
      can_play_ms: canPlayMsRef.current,
      playing_ms: playingMsRef.current,
      first_frame_ms: firstFrameMsRef.current,
      waiting_count: waitingCountRef.current,
      stalled_count: stalledCountRef.current,
      seeking_count: seekingCountRef.current,
      last_seek_ms: lastSeekMsRef.current,
      error_code: errorCodeRef.current,
      preloaded: prebufferVideos.length > 0,
    });
  }, [work, media, playSessionID, prebufferVideos.length]);

  const logCurrentPlaySession = useCallback((action: string) => {
    if (!media || !loadStartTimeRef.current || loggedPlaySessionRef.current === playSessionRef.current) return;
    loggedPlaySessionRef.current = playSessionRef.current;
    const loadTimeMs = Math.max(0, Math.round(performance.now() - loadStartTimeRef.current));
    logPlayerEvent(action, loadTimeMs);
  }, [logPlayerEvent, media]);

  const recordCurrentWorkView = useCallback(() => {
    if (!work?.id || recordedViewSessionRef.current === playSessionRef.current) return;
    recordedViewSessionRef.current = playSessionRef.current;
    void api.recordWorkView(work.id).catch(() => undefined);
  }, [work?.id]);

  const startPlaySession = useCallback(() => {
    playSessionRef.current += 1;
    loggedPlaySessionRef.current = 0;
    recordedViewSessionRef.current = 0;
    loadStartTimeRef.current = performance.now();
    metadataLoadMsRef.current = 0;
    loadedDataMsRef.current = 0;
    canPlayMsRef.current = 0;
    playingMsRef.current = 0;
    firstFrameMsRef.current = 0;
    waitingCountRef.current = 0;
    stalledCountRef.current = 0;
    seekingCountRef.current = 0;
    seekStartRef.current = 0;
    lastSeekMsRef.current = 0;
    errorCodeRef.current = 0;
  }, []);

  const activateOverlay = useCallback(() => {
    if (overlaysHidden) return;
    setOverlayActive(true);
    window.clearTimeout(overlayTimer.current);
    overlayTimer.current = window.setTimeout(() => setOverlayActive(false), 2200);
  }, [overlaysHidden]);

  const moveWork = useCallback(
    (delta: number) => {
      activateOverlay();
      loadStartTimeRef.current = performance.now();
      setWorkIndex((current) => {
        const next = Math.max(0, Math.min(works.length - 1, current + delta));
        if (next !== current) setMediaIndex(0);
        return next;
      });
    },
    [activateOverlay, works.length],
  );

  const moveMedia = useCallback(
    (delta: number) => {
      activateOverlay();
      const count = work?.media.length ?? 0;
      setMediaIndex((current) => Math.max(0, Math.min(count - 1, current + delta)));
    },
    [activateOverlay, work],
  );

  const nativeFullscreenAvailable = typeof HTMLElement.prototype.requestFullscreen === 'function' && typeof document.exitFullscreen === 'function';

  const attemptAutoplay = useCallback(async () => {
    const video = videoRef.current;
    if (!video || media?.type !== 'video') return;
    video.volume = volume;
    video.muted = muted;
    if (!video.paused && !video.ended) {
      setIsPlaying(true);
      return;
    }
    try {
      await video.play();
      setIsPlaying(true);
    } catch {
      setIsPlaying(false);
    }
  }, [media?.type, muted, volume]);

  const handleMediaClick = (event: MouseEvent<HTMLButtonElement>, delta: number) => {
    event.preventDefault();
    event.stopPropagation();
    moveMedia(delta);
  };

  const handleWheel = useCallback((event: WheelEvent) => {
    if (isInteractiveGestureTarget(event.target)) {
      return;
    }
    event.preventDefault();
    activateOverlay();
    if (wheelLockRef.current) {
      return;
    }
    wheelAccumRef.current += event.deltaY;
    if (Math.abs(wheelAccumRef.current) < 40) {
      return;
    }
    moveWork(wheelAccumRef.current > 0 ? 1 : -1);
    wheelAccumRef.current = 0;
    wheelLockRef.current = true;
    window.clearTimeout(wheelUnlockTimerRef.current);
    wheelUnlockTimerRef.current = window.setTimeout(() => {
      wheelLockRef.current = false;
      wheelAccumRef.current = 0;
    }, 450);
  }, [activateOverlay, moveWork]);

  const handleTouchStart = useCallback((event: TouchEvent) => {
    const touch = event.touches[0];
    if (!touch) return;
    touchIgnoredRef.current = isInteractiveGestureTarget(event.target);
    touchStartRef.current = { x: touch.clientX, y: touch.clientY };
  }, []);

  const handleTouchEnd = useCallback((event: TouchEvent) => {
    const start = touchStartRef.current;
    touchStartRef.current = null;
    if (!start || touchIgnoredRef.current || touchLockRef.current) {
      touchIgnoredRef.current = false;
      return;
    }
    const touch = event.changedTouches[0];
    if (!touch) return;
    loadStartTimeRef.current = performance.now();
    const deltaY = touch.clientY - start.y;
    const deltaX = touch.clientX - start.x;
    if (Math.abs(deltaY) < TOUCH_SWITCH_DISTANCE || Math.abs(deltaY) < Math.abs(deltaX) * TOUCH_AXIS_RATIO) {
      return;
    }
    event.preventDefault();
    activateOverlay();
    moveWork(deltaY < 0 ? 1 : -1);
    touchLockRef.current = true;
    window.setTimeout(() => {
      touchLockRef.current = false;
    }, 450);
  }, [activateOverlay, moveWork]);

  const togglePageFullscreen = () => {
    setIsPageFullscreen((current) => !current);
    activateOverlay();
  };

  const toggleNativeFullscreen = async () => {
    activateOverlay();
    if (!document.fullscreenElement) {
      await viewerRef.current?.requestFullscreen?.();
      return;
    }
    await document.exitFullscreen?.();
  };

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.key === 'ArrowDown') { loadStartTimeRef.current = performance.now(); moveWork(1); }
      if (event.key === 'ArrowUp') { loadStartTimeRef.current = performance.now(); moveWork(-1); }
      if (event.key === 'ArrowRight') moveMedia(1);
      if (event.key === 'ArrowLeft') moveMedia(-1);
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [moveMedia, moveWork]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    video.volume = volume;
    video.muted = muted;
    setCurrentTime(0);
    setDuration(0);
    setIsPlaying(true);
    void attemptAutoplay();
  }, [attemptAutoplay, media?.id]);

  useEffect(() => {
    const viewer = viewerRef.current;
    if (!viewer) return;
    viewer.addEventListener('wheel', handleWheel, { passive: false });
    viewer.addEventListener('touchstart', handleTouchStart, { passive: false });
    viewer.addEventListener('touchend', handleTouchEnd, { passive: false });
    return () => {
      viewer.removeEventListener('wheel', handleWheel);
      viewer.removeEventListener('touchstart', handleTouchStart);
      viewer.removeEventListener('touchend', handleTouchEnd);
    };
  }, [handleTouchEnd, handleTouchStart, handleWheel]);

  const submitSearch = (event: FormEvent) => {
    event.preventDefault();
    onSearch?.(query.trim());
    setSearchOpen(false);
    activateOverlay();
  };

  const togglePlay = () => {
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) {
      void video.play();
      setIsPlaying(true);
    } else {
      video.pause();
      setIsPlaying(false);
    }
  };

  const updateVideoDuration = (video: HTMLVideoElement) => {
    const nextDuration = normalizeMediaDuration(video.duration);
    if (nextDuration > 0) {
      setDuration(nextDuration);
    }
    setMediaSize({ width: video.videoWidth || 0, height: video.videoHeight || 0 });
  };

  const seek = (value: number) => {
    const video = videoRef.current;
    const knownDuration = normalizeMediaDuration(video?.duration ?? 0) || duration;
    if (!video || knownDuration <= 0) return;
    const next = Math.max(0, Math.min(value, knownDuration));
    video.currentTime = next;
    setCurrentTime(next);
  };

  const mediaStyle = useMemo<CSSProperties>(() => {
    if (!stageSize.width || !stageSize.height || !mediaSize.width || !mediaSize.height) {
      return {};
    }
    const stageRatio = stageSize.width / stageSize.height;
    const mediaRatio = mediaSize.width / mediaSize.height;
    if (stageRatio > mediaRatio) {
      return { width: `${stageSize.height * mediaRatio}px`, height: `${stageSize.height}px` };
    }
    return { width: `${stageSize.width}px`, height: `${stageSize.width / mediaRatio}px` };
  }, [mediaSize.height, mediaSize.width, stageSize.height, stageSize.width]);

  const viewerStyle = useMemo(() => ({ '--overlay-alpha': overlayOpacity.toFixed(2) }) as CSSProperties, [overlayOpacity]);

  const updateOverlayOpacity = (value: number) => {
    const next = normalizeOverlayOpacity(value);
    setOverlayOpacity(next);
    writeOverlayOpacity(next);
  };

  if (!work || !media) {
    return <div className="empty-state">还没有作品。先去设置里保存目录并扫描。</div>;
  }

  return (
    <div
      ref={viewerRef}
      className={`feed-viewer immersive ${overlayActive ? 'overlay-active' : ''} ${!isPlaying ? 'paused' : ''} ${overlaysHidden ? 'overlays-hidden' : ''} ${isPageFullscreen ? 'page-fullscreen' : ''}`}
      style={viewerStyle}
      tabIndex={0}
      onFocus={activateOverlay}
      onMouseMove={activateOverlay}
    >
      <section
        ref={stageRef}
        className="stage immersive-stage"
        aria-label="作品播放器"
        onClick={(event) => {
          if (!isVideo || isInteractiveGestureTarget(event.target)) return;
          togglePlay();
          activateOverlay();
        }}
      >
        {media.type === 'video' && (
          <video
            className="media-content"
            style={mediaStyle}
            ref={videoRef}
            src={mediaSrc}
            autoPlay
            muted={muted}
            loop
            playsInline
            preload="metadata"
            onLoadedMetadata={(event) => {
              updateVideoDuration(event.currentTarget);
              metadataLoadMsRef.current = Math.round(performance.now() - loadStartTimeRef.current);
            }}
            onDurationChange={(event) => updateVideoDuration(event.currentTarget)}
            onCanPlay={(event) => {
              updateVideoDuration(event.currentTarget);
              canPlayMsRef.current = Math.round(performance.now() - loadStartTimeRef.current);
              void attemptAutoplay();
            }}
            onLoadedData={(event) => {
              updateVideoDuration(event.currentTarget);
              loadedDataMsRef.current = Math.round(performance.now() - loadStartTimeRef.current);
              void attemptAutoplay();
            }}
            onTimeUpdate={(event) => setCurrentTime(event.currentTarget.currentTime)}
            onPlay={(event) => {
              setIsPlaying(true);
              if (event.currentTarget.currentTime > 0 || loggedPlaySessionRef.current === playSessionRef.current) {
                startPlaySession();
              }
            }}
            onPlaying={(event) => {
              recordCurrentWorkView();
              playingMsRef.current = Math.round(performance.now() - loadStartTimeRef.current);
              const video = event.currentTarget as HTMLVideoElement & {
                requestVideoFrameCallback?: (callback: () => void) => number;
              };
              if (typeof video.requestVideoFrameCallback === 'function') {
                video.requestVideoFrameCallback(() => {
                  firstFrameMsRef.current = Math.round(performance.now() - loadStartTimeRef.current);
                  logCurrentPlaySession('first_frame');
                });
                return;
              }
              logCurrentPlaySession('playing');
            }}
            onPause={() => setIsPlaying(false)}
            onWaiting={() => {
              waitingCountRef.current += 1;
            }}
            onStalled={() => {
              stalledCountRef.current += 1;
            }}
            onSeeking={() => {
              seekingCountRef.current += 1;
              seekStartRef.current = performance.now();
            }}
            onSeeked={() => {
              if (seekStartRef.current > 0) {
                lastSeekMsRef.current = Math.round(performance.now() - seekStartRef.current);
                seekStartRef.current = 0;
              }
            }}
            onError={(event) => {
              errorCodeRef.current = event.currentTarget.error?.code ?? 0;
              logCurrentPlaySession('error');
            }}
          />
        )}
        {media.type === 'image' && (
          <img
            className="media-content"
            style={mediaStyle}
            src={media.url}
            alt={work.title}
            onLoad={(event) => setMediaSize({ width: event.currentTarget.naturalWidth || 0, height: event.currentTarget.naturalHeight || 0 })}
          />
        )}
        {media.type === 'audio' && <audio src={media.url} controls autoPlay />}
        {isVideo && !isPlaying && !overlaysHidden && (
          <div className="center-pause-indicator" aria-label="当前已暂停">
            <Pause size={46} fill="currentColor" strokeWidth={2.4} />
          </div>
        )}
      </section>

      <div className="video-prebuffer" aria-hidden="true">
        {prebufferVideos.map((item) => (
          <video key={item.id} src={item.url} preload="auto" muted playsInline tabIndex={-1} title={item.file_name} />
        ))}
      </div>

      {overlaysHidden && (
        <button className="overlay-restore-button icon-button" type="button" onClick={() => setOverlaysHidden(false)} title="打开悬浮" aria-label="打开悬浮">
          <Eye size={20} />
        </button>
      )}

      {overlaysHidden && isVideo && (
        <MinimalVideoProgress currentTime={currentTime} duration={duration} onSeek={seek} />
      )}

      {!overlaysHidden && (
        <div className="player-overlay">
          <div className={`feed-top-actions ${searchOpen ? 'search-open' : ''}`}>
            {!searchOpen && (
              <button className="feed-search-toggle" type="button" title="搜索" aria-label="打开搜索" onClick={() => setSearchOpen(true)}>
                <Search size={28} />
              </button>
            )}
            {searchOpen && (
              <form className="feed-search" onSubmit={submitSearch}>
                <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索本地作品" aria-label="搜索本地作品" autoFocus />
                <button type="submit" title="搜索">
                  <Search size={20} />
                  <span>搜索</span>
                </button>
              </form>
            )}
          </div>

        <aside className="action-rail" aria-label="作品操作">
          {isVideo && (
            <MuteRailButton
              muted={muted}
              onToggle={() => {
                const next = !muted;
                setMuted(next);
                if (videoRef.current) videoRef.current.muted = next;
              }}
            />
          )}
          <FavoriteRailButtons work={work} enabled={enableFavorites} />
        </aside>

        {work.media.length > 1 && (
          <div className="media-controls floating-media-controls">
            <button className="icon-button" type="button" onClick={(event) => handleMediaClick(event, -1)} title="上一张" disabled={mediaIndex === 0}>
              <ChevronLeft size={20} />
            </button>
            <span>
              {mediaIndex + 1}/{work.media.length}
            </span>
            <button className="icon-button" type="button" onClick={(event) => handleMediaClick(event, 1)} title="下一张" disabled={mediaIndex === work.media.length - 1}>
              <ChevronRight size={20} />
            </button>
          </div>
        )}

        {isVideo && <VideoTimeline currentTime={currentTime} duration={duration} onSeek={seek} />}

        <footer className="bottom-meta">
          <section className={`creator-info-panel ${infoExpanded ? 'expanded' : ''}`} aria-label="作品作者信息">
            <Link className="bottom-creator-link" to={`/creator/${work.creator_id}`} title={`查看 @${work.creator_name}`} aria-label={`@${work.creator_name}`}>
              {work.creator_avatar_url ? <img className="bottom-meta-avatar" src={work.creator_avatar_url} alt={`${work.creator_name} 头像`} /> : <span className="bottom-meta-avatar avatar-fallback">{work.creator_name.slice(0, 1).toUpperCase()}</span>}
              <span>@{work.creator_name}</span>
            </Link>
            <p className="bottom-summary">
              {formatDate(work.published_at) && <span>{formatDate(work.published_at)} </span>}
              {renderLinkedText(work.description || work.title)}
              {work.source_url && <> {renderSingleLink(work.source_url)}</>}
              {work.music_title && <> <span className="music-inline">♪ {work.music_title}</span></>}
            </p>
            {hasExpandableInfo && (
              <button className="feed-info-expand" type="button" onClick={() => setInfoExpanded((current) => !current)}>
                {infoExpanded ? '收起' : '展开'}
              </button>
            )}
            {infoExpanded && work.linked_creators && work.linked_creators.length > 0 && (
              <div className="bottom-meta-linked">
                <small>关联推主</small>
                <div className="bottom-meta-linked-list">
                  {work.linked_creators.map((creator) => (
                    <Link key={creator.id} to={`/creator/${creator.id}`}>
                      @{creator.name}
                    </Link>
                  ))}
                </div>
              </div>
            )}
            {infoExpanded && workTags.length > 0 && (
              <div className="tags">
                {workTags.map((tag) => (
                  <span key={tag}>#{tag}</span>
                ))}
              </div>
            )}
          </section>
        </footer>

        <footer className="player-control-dock" aria-label="播放器控制条">
          {isVideo && (
            <VideoControls
              isPlaying={isPlaying}
              muted={muted}
              volume={volume}
              currentTime={currentTime}
              duration={duration}
              onTogglePlay={togglePlay}
              onVolume={(next) => {
                setVolume(next);
                setMuted(next === 0);
                if (videoRef.current) videoRef.current.volume = next;
              }}
              onToggleMute={() => {
                const next = !muted;
                setMuted(next);
                if (videoRef.current) videoRef.current.muted = next;
              }}
              fullscreenControls={
                <FullscreenControls
                  isPageFullscreen={isPageFullscreen}
                  isNativeFullscreen={isNativeFullscreen}
                  nativeFullscreenAvailable={nativeFullscreenAvailable}
                  onTogglePageFullscreen={togglePageFullscreen}
                  onToggleNativeFullscreen={toggleNativeFullscreen}
                />
              }
              overlayToggle={
                <OverlayToggle hidden={overlaysHidden} onToggle={() => setOverlaysHidden((current) => !current)} />
              }
              overlayOpacity={overlayOpacity}
              onOverlayOpacity={updateOverlayOpacity}
              closeKey={`${media.id}-${work.id}`}
            />
          )}
          {!isVideo && (
            <FullscreenControls
              isPageFullscreen={isPageFullscreen}
              isNativeFullscreen={isNativeFullscreen}
              nativeFullscreenAvailable={nativeFullscreenAvailable}
              onTogglePageFullscreen={togglePageFullscreen}
              onToggleNativeFullscreen={toggleNativeFullscreen}
            />
          )}
        </footer>
      </div>
      )}
    </div>
  );
}

function MinimalVideoProgress({ currentTime, duration, onSeek }: { currentTime: number; duration: number; onSeek: (value: number) => void }) {
  return (
    <div className="minimal-progress-dock" aria-label="极简播放进度">
      <input
        className="minimal-timeline feed-range-input"
        aria-label="视频进度"
        type="range"
        min={0}
        max={duration || 0}
        step="0.1"
        value={Math.min(currentTime, duration || 0)}
        onChange={(event) => onSeek(Number(event.currentTarget.value))}
      />
    </div>
  );
}

function FavoriteRailButtons({ work, enabled }: { work: Work; enabled: boolean }) {
  const queryClient = useQueryClient();
  const [openMenu, setOpenMenu] = useState<'work' | 'creator' | null>(null);
  const workFolders = useQuery({
    queryKey: ['favorite-folders', 'work'],
    queryFn: () => api.favoriteFolders('work'),
    enabled,
  });
  const creatorFolders = useQuery({
    queryKey: ['favorite-folders', 'creator'],
    queryFn: () => api.favoriteFolders('creator'),
    enabled,
  });
  const favoriteStatus = useQuery({
    queryKey: ['favorite-status', work.id, work.creator_id],
    queryFn: () => api.favoriteStatus({ workId: work.id, creatorId: work.creator_id }),
    enabled,
  });
  const status = favoriteStatus.data?.status;
  const workFolderIds = status?.work_folder_ids ?? [];
  const creatorFolderIds = status?.creator_folder_ids ?? [];
  const workFavorited = Boolean(status?.work_favorited || workFolderIds.length);
  const creatorFavorited = Boolean(status?.creator_favorited || creatorFolderIds.length);
  const workFolderList = workFolders.data?.folders ?? [];
  const creatorFolderList = creatorFolders.data?.folders ?? [];
  const defaultWorkFolder = workFolderList.find((folder) => folder.is_default);
  const defaultCreatorFolder = creatorFolderList.find((folder) => folder.is_default);

  const invalidateFavorites = () => {
    void queryClient.invalidateQueries({ queryKey: ['favorite-status', work.id, work.creator_id] });
    void queryClient.invalidateQueries({ queryKey: ['favorite-works'] });
    void queryClient.invalidateQueries({ queryKey: ['favorite-creators'] });
    void queryClient.invalidateQueries({ queryKey: ['favorite-folders'] });
    void queryClient.invalidateQueries({ queryKey: ['creators'] });
  };

  const toggleWorkFavorite = useMutation({
    mutationFn: (folderId?: number) => {
      if (folderId == null) {
        return workFavorited ? api.removeWorkFavorite(work.id) : api.addWorkFavorite(work.id);
      }
      return workFolderIds.includes(folderId) ? api.removeWorkFavorite(work.id, folderId) : api.addWorkFavorite(work.id, folderId);
    },
    onSuccess: () => {
      invalidateFavorites();
    },
  });
  const toggleCreatorFavorite = useMutation({
    mutationFn: (folderId?: number) => {
      if (folderId == null) {
        return creatorFavorited ? api.removeCreatorFavorite(work.creator_id) : api.addCreatorFavorite(work.creator_id);
      }
      return creatorFolderIds.includes(folderId) ? api.removeCreatorFavorite(work.creator_id, folderId) : api.addCreatorFavorite(work.creator_id, folderId);
    },
    onSuccess: () => {
      invalidateFavorites();
    },
  });

  return (
    <>
      <FavoriteRailButton
        active={workFavorited}
        icon={<Heart size={30} fill={workFavorited ? 'currentColor' : 'none'} />}
        label={workFavorited ? '已收藏作品' : '收藏作品'}
        title={workFavorited ? '取消收藏该作品' : '收藏该作品'}
        disabled={!enabled || toggleWorkFavorite.isPending}
        onClick={() => toggleWorkFavorite.mutate(defaultWorkFolder?.id)}
        onExpand={() => setOpenMenu(openMenu === 'work' ? null : 'work')}
      />
      {openMenu === 'work' && (
        <FavoriteFolderMenu
          folders={workFolderList}
          selectedIds={workFolderIds}
          onSelect={(folderId) => toggleWorkFavorite.mutate(folderId)}
        />
      )}
      <FavoriteRailButton
        active={creatorFavorited}
        icon={<Star size={30} fill={creatorFavorited ? 'currentColor' : 'none'} />}
        label={creatorFavorited ? '已收藏作者' : '收藏作者'}
        title={creatorFavorited ? '取消收藏该作者' : '收藏该作者'}
        disabled={!enabled || toggleCreatorFavorite.isPending}
        onClick={() => toggleCreatorFavorite.mutate(defaultCreatorFolder?.id)}
        onExpand={() => setOpenMenu(openMenu === 'creator' ? null : 'creator')}
      />
      {openMenu === 'creator' && (
        <FavoriteFolderMenu
          folders={creatorFolderList}
          selectedIds={creatorFolderIds}
          onSelect={(folderId) => toggleCreatorFavorite.mutate(folderId)}
        />
      )}
    </>
  );
}

function isInteractiveGestureTarget(target: EventTarget | null) {
  if (!(target instanceof Element)) return false;
  return Boolean(target.closest('a, button, input, textarea, select, .feed-range-input, .media-controls, .video-controls, .feed-search, .feed-top-actions, .bottom-meta, .opacity-popover, .favorite-folder-menu, .favorite-action'));
}

function MuteRailButton({ muted, onToggle }: { muted: boolean; onToggle: () => void }) {
  const label = muted ? '取消静音' : '静音';
  return (
    <button className="rail-button mute-rail-button" title={label} aria-label={label} onClick={onToggle} type="button">
      {muted ? <VolumeX size={30} /> : <Volume2 size={30} />}
      <span>{label}</span>
    </button>
  );
}

function shouldDisableVideoPrebuffer() {
  const nav = navigator as Navigator & {
    connection?: { saveData?: boolean };
  };
  if (nav.maxTouchPoints > 0) return true;
  if (nav.connection?.saveData) return true;
  if (typeof window.matchMedia === 'function' && window.matchMedia('(pointer: coarse)').matches) return true;
  return false;
}

function FavoriteRailButton({
  icon,
  label,
  title,
  active,
  disabled,
  onClick,
  onExpand,
}: {
  icon: ReactNode;
  label: string;
  title: string;
  active?: boolean;
  disabled?: boolean;
  onClick?: () => void;
  onExpand: () => void;
}) {
  return (
    <div className="favorite-action">
      <button className={`rail-button ${active ? 'favorited active' : ''}`} title={title} onClick={onClick} disabled={disabled} type="button">
        {icon}
        <span>{label}</span>
      </button>
      <button className="favorite-expand" type="button" title="选择收藏夹" aria-label="选择收藏夹" onClick={onExpand} disabled={disabled}>
        目录
      </button>
    </div>
  );
}

function FavoriteFolderMenu({
  folders,
  selectedIds,
  onSelect,
}: {
  folders: { id: number; name: string; is_default: boolean }[];
  selectedIds: number[];
  onSelect: (id: number) => void;
}) {
  return (
    <div className="favorite-folder-menu">
      {folders.map((folder) => (
        <button key={folder.id} type="button" onClick={() => onSelect(folder.id)}>
          {selectedIds.includes(folder.id) ? '✓ ' : ''}
          {folder.name}
          {folder.is_default ? ' 默认' : ''}
        </button>
      ))}
      {folders.length === 0 && <span>暂无收藏夹</span>}
    </div>
  );
}

function VideoControls({
  isPlaying,
  muted,
  volume,
  currentTime,
  duration,
  onTogglePlay,
  onVolume,
  onToggleMute,
  fullscreenControls,
  overlayToggle,
  overlayOpacity,
  onOverlayOpacity,
  closeKey,
}: {
  isPlaying: boolean;
  muted: boolean;
  volume: number;
  currentTime: number;
  duration: number;
  onTogglePlay: () => void;
  onVolume: (value: number) => void;
  onToggleMute: () => void;
  fullscreenControls: ReactNode;
  overlayToggle: ReactNode;
  overlayOpacity: number;
  onOverlayOpacity: (value: number) => void;
  closeKey: string;
}) {
  const [opacityOpen, setOpacityOpen] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const controlsRef = useRef<HTMLDivElement | null>(null);
  const opacityPercent = Math.round(overlayOpacity * 100);

  useEffect(() => {
    setAdvancedOpen(false);
    setOpacityOpen(false);
  }, [closeKey]);

  useEffect(() => {
    if (!advancedOpen) return;
    const handlePointerDown = (event: globalThis.PointerEvent) => {
      if (controlsRef.current?.contains(event.target as Node)) return;
      setAdvancedOpen(false);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setAdvancedOpen(false);
    };
    document.addEventListener('pointerdown', handlePointerDown);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [advancedOpen]);

  return (
    <div ref={controlsRef} className={`video-controls ${advancedOpen ? 'mobile-controls-expanded' : ''}`} aria-label="视频控制">
      <div className="control-row">
        <span className="timecode timeline-time-badge">
          {formatTime(currentTime)} / {formatTime(duration)}
        </span>
        <button
          className={`icon-button more-controls-button ${advancedOpen ? 'active-toggle' : ''}`}
          type="button"
          title="更多播放控制"
          aria-label="更多播放控制"
          aria-expanded={advancedOpen}
          onClick={() => setAdvancedOpen((current) => !current)}
        >
          <MoreHorizontal size={22} />
        </button>
        {advancedOpen && (
          <div className="advanced-control-panel" aria-label="高级播放控制">
            <label className="volume-control" title="音量">
              <button className="icon-button panel-mute-button" type="button" onClick={onToggleMute} title={muted ? '取消静音' : '静音'} aria-label={muted ? '取消静音' : '静音'}>
                {muted ? <VolumeX size={22} /> : <Volume2 size={22} />}
              </button>
              <input className="feed-range-input" aria-label="音量" type="range" min={0} max={1} step={0.05} value={muted ? 0 : volume} onChange={(event) => onVolume(Number(event.currentTarget.value))} />
            </label>
            <section className="advanced-panel-section" aria-label="播放设置">
              <span className="advanced-panel-heading">播放</span>
              <div className="advanced-action-grid advanced-action-grid-single">
                <button className="icon-button mode-icon-button" type="button" onClick={onTogglePlay} title={isPlaying ? '暂停' : '播放'} aria-label={isPlaying ? '暂停' : '播放'}>
                  {isPlaying ? <Pause size={22} fill="currentColor" /> : <Play size={22} fill="currentColor" />}
                  <span>{isPlaying ? '暂停' : '播放'}</span>
                </button>
              </div>
            </section>
            <section className="advanced-panel-section" aria-label="画面设置">
              <span className="advanced-panel-heading">画面</span>
              <div className="advanced-action-grid">
                <div className="opacity-control advanced-action-cell">
                  <button className="icon-button mode-icon-button" type="button" onClick={() => setOpacityOpen((current) => !current)} title="悬浮透明度" aria-label="悬浮透明度">
                    <SlidersHorizontal size={22} />
                    <span>透明度</span>
                  </button>
                  {opacityOpen && (
                    <div className="opacity-popover" role="group" aria-label="悬浮透明度设置">
                      <input
                        className="feed-range-input"
                        aria-label="悬浮透明度数值"
                        type="range"
                        min={MIN_OVERLAY_OPACITY}
                        max={MAX_OVERLAY_OPACITY}
                        step="0.02"
                        value={overlayOpacity}
                        onChange={(event) => onOverlayOpacity(Number(event.currentTarget.value))}
                      />
                      <span>{opacityPercent}%</span>
                    </div>
                  )}
                </div>
                <div className="advanced-action-cell">{overlayToggle}</div>
              </div>
            </section>
            <section className="advanced-panel-section advanced-panel-section-compact" aria-label="窗口设置">
              <span className="advanced-panel-heading">窗口</span>
              <div className="advanced-window-actions">
                {fullscreenControls}
              </div>
            </section>
          </div>
        )}
      </div>
    </div>
  );
}

function VideoTimeline({ currentTime, duration, onSeek }: { currentTime: number; duration: number; onSeek: (value: number) => void }) {
  return (
    <div className="full-video-timeline" aria-label="视频底部进度条">
      <input
        className="timeline feed-range-input"
        aria-label="视频进度"
        type="range"
        min={0}
        max={duration || 0}
        step="0.1"
        value={Math.min(currentTime, duration || 0)}
        onChange={(event) => onSeek(Number(event.currentTarget.value))}
      />
    </div>
  );
}

function OverlayToggle({ hidden, onToggle }: { hidden: boolean; onToggle: () => void }) {
  return (
    <button className="icon-button overlay-toggle" type="button" onClick={onToggle} title={hidden ? '打开悬浮' : '关闭悬浮'} aria-label={hidden ? '打开悬浮' : '关闭悬浮'}>
      {hidden ? <Eye size={21} /> : <EyeOff size={21} />}
    </button>
  );
}

function FullscreenControls({
  isPageFullscreen,
  isNativeFullscreen,
  nativeFullscreenAvailable,
  onTogglePageFullscreen,
  onToggleNativeFullscreen,
}: {
  isPageFullscreen: boolean;
  isNativeFullscreen: boolean;
  nativeFullscreenAvailable: boolean;
  onTogglePageFullscreen: () => void;
  onToggleNativeFullscreen: () => void;
}) {
  return (
    <div className="fullscreen-controls" aria-label="全屏控制">
      <button className="icon-button" type="button" onClick={onTogglePageFullscreen} title={isPageFullscreen ? '退出网页全屏' : '网页全屏'} aria-label={isPageFullscreen ? '退出网页全屏' : '网页全屏'}>
        {isPageFullscreen ? <Minimize2 size={21} /> : <Maximize2 size={21} />}
      </button>
      {nativeFullscreenAvailable && (
        <button className="icon-button" type="button" onClick={onToggleNativeFullscreen} title={isNativeFullscreen ? '退出完整全屏' : '完整全屏'} aria-label={isNativeFullscreen ? '退出完整全屏' : '完整全屏'}>
          {isNativeFullscreen ? <Minimize2 size={21} /> : <Expand size={21} />}
        </button>
      )}
    </div>
  );
}

function formatTime(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '00:00';
  const minutes = Math.floor(value / 60);
  const seconds = Math.floor(value % 60);
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

function normalizeMediaDuration(value: number) {
  return Number.isFinite(value) && value > 0 ? value : 0;
}

function formatDate(value: string) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' });
}

function workInfoSummary(work: Work) {
  return [formatDate(work.published_at), work.description || work.title, work.source_url, work.music_title ? `♪ ${work.music_title}` : ''].filter(Boolean).join(' ');
}

function readOverlayOpacity() {
  try {
    if (typeof window === 'undefined') return DEFAULT_OVERLAY_OPACITY;
    const stored = window.localStorage?.getItem(OVERLAY_OPACITY_KEY) ?? readCookie(OVERLAY_OPACITY_COOKIE);
    if (stored === null) return DEFAULT_OVERLAY_OPACITY;
    return normalizeOverlayOpacity(Number(stored));
  } catch {
    return DEFAULT_OVERLAY_OPACITY;
  }
}

function writeOverlayOpacity(value: number) {
  const normalized = value.toFixed(2);
  try {
    if (typeof window === 'undefined') return;
    window.localStorage?.setItem(OVERLAY_OPACITY_KEY, normalized);
  } catch {
    // Browsers can disable localStorage; the live UI state still applies.
  }
  writeCookie(OVERLAY_OPACITY_COOKIE, normalized);
}

function normalizeOverlayOpacity(value: number) {
  if (!Number.isFinite(value)) return DEFAULT_OVERLAY_OPACITY;
  return Math.min(MAX_OVERLAY_OPACITY, Math.max(MIN_OVERLAY_OPACITY, value));
}

function createPlaySessionID() {
  const cryptoAPI = typeof crypto !== 'undefined' ? crypto : undefined;
  if (cryptoAPI?.randomUUID) {
    return cryptoAPI.randomUUID();
  }
  return `play-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

function readCookie(name: string) {
  if (typeof document === 'undefined') return null;
  const prefix = `${name}=`;
  const cookie = document.cookie.split('; ').find((part) => part.startsWith(prefix));
  return cookie ? decodeURIComponent(cookie.slice(prefix.length)) : null;
}

function writeCookie(name: string, value: string) {
  if (typeof document === 'undefined') return;
  document.cookie = `${name}=${encodeURIComponent(value)}; max-age=31536000; path=/; SameSite=Lax`;
}

const urlPattern = /https?:\/\/[^\s<>"']+/g;

function renderLinkedText(text: string) {
  if (!text) return null;
  const nodes: ReactNode[] = [];
  let cursor = 0;
  for (const match of text.matchAll(urlPattern)) {
    const url = trimTrailingPunctuation(match[0]);
    const index = match.index ?? 0;
    if (index > cursor) {
      nodes.push(text.slice(cursor, index));
    }
    nodes.push(renderSingleLink(url, nodes.length));
    cursor = index + match[0].length;
    if (url.length < match[0].length) {
      nodes.push(match[0].slice(url.length));
    }
  }
  if (cursor < text.length) {
    nodes.push(text.slice(cursor));
  }
  return nodes;
}

function renderSingleLink(url: string, key?: Key) {
  return (
    <a key={key} className="inline-link" href={url} target="_blank" rel="noreferrer">
      {linkLabel(url)}
    </a>
  );
}

function linkLabel(url: string) {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return '打开链接';
  }
  const host = parsed.hostname.toLowerCase();
  const path = parsed.pathname.toLowerCase();
  if ((host === 'x.com' || host.endsWith('.x.com') || host === 'twitter.com' || host.endsWith('.twitter.com')) && path.includes('/status/')) {
    return '查看原推文';
  }
  if (host === 'pbs.twimg.com' || /\.(jpg|jpeg|png|webp|gif)$/.test(path)) {
    return '查看封面';
  }
  return '打开链接';
}

function trimTrailingPunctuation(url: string) {
  return url.replace(/[，。！？、；：,.!?;:)）]+$/g, '');
}
