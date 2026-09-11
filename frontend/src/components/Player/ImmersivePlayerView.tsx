import { VideoStage } from "./VideoStage";
import { LyricsScroller } from "./LyricsScroller";
import { EmptyState, StatusBadge } from "../Common";
import {
  badgeToneFromTrack,
  formatDuration,
  getArtworkClickAction,
  initialsForTrack,
  metadataLabel,
} from "../../lib/viewHelpers";
import type { MutableRefObject } from "react";
import type Player from "video.js/dist/types/player";
import type { PlaybackState, RepeatMode, TrackPreview, TrackRecord } from "../../types";
import type { VideoStageError } from "../../lib/videoDiagnostics";

export type ImmersivePlayerViewProps = {
  track: TrackRecord | null;
  preview: TrackPreview | null;
  previewLoading: boolean;
  artworkDataUrl: string;
  videoSource: string;
  videoOpen: boolean;
  videoFullscreenOpen: boolean;
  videoLoading: boolean;
  videoError: string;
  videoCurrentTime: number;
  videoDuration: number;
  videoIsPlaying: boolean;
  videoVolume: number;
  videoMuted: boolean;
  videoSourceType?: string;
  videoShouldPlay?: boolean;
  videoPlayerRef: MutableRefObject<Player | null>;
  currentTime: number;
  duration: number;
  seekValue: number;
  isPlaying: boolean;
  isLoading: boolean;
  volume: number;
  muted: boolean;
  shuffleEnabled: boolean;
  repeatMode: RepeatMode;
  error: string;
  _player?: PlaybackState;
  onClose: () => void;
  onBack: () => void;
  onPrevious: () => Promise<void>;
  onPlayPause: () => Promise<void>;
  onNext: () => Promise<void>;
  onSeek: (ratio: number) => void;
  onSeekStart: (ratio: number) => void;
  onSeekEnd: (ratio: number) => void;
  onVolume: (value: number) => void;
  onToggleMute: () => void;
  onToggleShuffle: () => void;
  onToggleRepeat: () => void;
  _onOpenLyrics?: () => void;
  onOpenInfo: () => void;
  onOpenVideo: () => Promise<void>;
  onCloseVideo: () => Promise<void>;
  onDownloadVideo: () => Promise<void>;
  onSetArtwork: () => Promise<void>;
  onAddContext: () => void;
  hasVideoDownloaded: boolean;
  onVideoTimeChange: (seconds: number) => void;
  onVideoDurationChange: (seconds: number) => void;
  onVideoPlayingChange: (value: boolean) => void;
  onVideoError: (value: VideoStageError) => void;
  onVideoLoaded: () => void;
  onAddToPlaylist: () => void;
  onRevealFolder: () => void;
};

export function ImmersivePlayerView({
  track,
  preview,
  previewLoading,
  artworkDataUrl,
  videoSource,
  videoOpen,
  videoFullscreenOpen,
  videoLoading,
  videoError,
  videoCurrentTime,
  videoDuration,
  videoIsPlaying,
  videoVolume,
  videoMuted,
  videoSourceType,
  videoShouldPlay,
  videoPlayerRef,
  currentTime,
  duration,
  seekValue,
  isPlaying,
  isLoading,
  volume,
  muted,
  shuffleEnabled,
  repeatMode,
  error,
  _player,
  onClose,
  onBack,
  onPrevious,
  onPlayPause,
  onNext,
  onSeek,
  onSeekStart,
  onSeekEnd,
  onVolume,
  onToggleMute,
  onToggleShuffle,
  onToggleRepeat,
  _onOpenLyrics,
  onOpenInfo,
  onOpenVideo,
  onCloseVideo,
  onDownloadVideo,
  onSetArtwork,
  onAddContext,
  hasVideoDownloaded,
  onVideoTimeChange,
  onVideoDurationChange,
  onVideoPlayingChange,
  onVideoError,
  onVideoLoaded,
  onAddToPlaylist,
  onRevealFolder,
}: ImmersivePlayerViewProps) {
  const artwork = artworkDataUrl || track?.artworkDataUrl || "";
  const artworkAction = getArtworkClickAction(track, hasVideoDownloaded);
  const canDownloadVideo = artworkAction === "download-video";
  const hasArtwork = Boolean(artwork.trim());
  const resolvedVideoSourceType = videoSourceType ?? "video/mp4";
  const resolvedVideoShouldPlay = videoShouldPlay ?? false;
  const activeIsPlaying = videoOpen || videoFullscreenOpen ? videoIsPlaying : isPlaying;
  const activeIsLoading = videoOpen || videoFullscreenOpen ? videoLoading : isLoading;
  const activeVolume = videoOpen || videoFullscreenOpen ? videoVolume : volume;
  const activeMuted = videoOpen || videoFullscreenOpen ? videoMuted : muted;

  return (
    <div className="full-player-screen">
      <div className="full-player-backdrop" />
      <div className="full-player-shell">
        <div className="full-player-top">
          <button className="icon-button" onClick={onBack} aria-label="Back to library">
            ←
          </button>
          <button className="icon-button" onClick={onClose} aria-label="Close player">
            ✕
          </button>
        </div>

        {!track ? (
          <div className="full-player-empty">
            <EmptyState title="Nothing playing" text="Choose a track from your library to open the immersive player." />
          </div>
        ) : (
          <div className="full-player-grid">
            <section className="full-player-left">
              <div className="full-player-art">
                {artwork ? (
                  <button
                    className="full-player-art-button"
                    onClick={() => {
                      if (hasVideoDownloaded) {
                        void onOpenVideo();
                        return;
                      }
                      if (canDownloadVideo) {
                        void onDownloadVideo();
                      }
                    }}
                    disabled={!track}
                    aria-label={
                      hasVideoDownloaded ? "Play music video" : canDownloadVideo ? "Download music video" : "Album art"
                    }
                  >
                    <img src={artwork} alt="" />
                    <span
                      className={`video-art-overlay ${hasVideoDownloaded ? "play" : canDownloadVideo ? "download" : ""}`}
                    >
                      {hasVideoDownloaded ? "▶" : canDownloadVideo ? "⇩" : initialsForTrack(track)}
                    </span>
                  </button>
                ) : (
                  <button
                    className="full-player-art-button"
                    onClick={() => {
                      if (canDownloadVideo) {
                        void onDownloadVideo();
                      }
                    }}
                    disabled={!canDownloadVideo}
                    aria-label={canDownloadVideo ? "Download music video" : "No artwork available"}
                  >
                    <span>{initialsForTrack(track)}</span>
                    {canDownloadVideo ? <span className="video-art-overlay download">⇩</span> : null}
                  </button>
                )}
              </div>
              <div className="full-player-meta">
                <h2 className="full-player-title">{track.title}</h2>
                <div className="full-player-subtitle">
                  {track.artist} · {track.album}
                </div>
                <div className="collection-meta-row">
                  <StatusBadge tone="accent">{activeIsPlaying ? "Playing" : "Paused"}</StatusBadge>
                  <StatusBadge tone={badgeToneFromTrack(track)}>{metadataLabel(track)}</StatusBadge>
                </div>
                <div className="action-row">
                  <button
                    className="button button-primary"
                    onClick={() => void onPlayPause()}
                    disabled={activeIsLoading}
                  >
                    {activeIsPlaying ? "Pause" : "Play"}
                  </button>
                  <button className="button button-secondary" onClick={onOpenInfo} aria-label="Track profile">
                    Info
                  </button>
                  <button className="button button-secondary" onClick={onAddContext}>
                    Add Context
                  </button>
                  <button className="button button-secondary" onClick={() => void onSetArtwork()}>
                    {hasArtwork ? "Change Album Art" : "Add Album Art"}
                  </button>
                  {hasVideoDownloaded ? (
                    <button className="button button-secondary" onClick={() => void onOpenVideo()}>
                      Play Video
                    </button>
                  ) : canDownloadVideo ? (
                    <button className="button button-secondary" onClick={() => void onDownloadVideo()}>
                      Download Video
                    </button>
                  ) : null}
                  <button className="button button-secondary" onClick={onAddToPlaylist}>
                    Add to Playlist
                  </button>
                  <button className="button button-secondary" onClick={onRevealFolder}>
                    Open Folder
                  </button>
                </div>
              </div>

              <div className="full-player-controls">
                <div className="player-controls large">
                  <button
                    className="player-button"
                    onClick={() => void onPrevious()}
                    disabled={isLoading}
                    aria-label="Previous track"
                  >
                    ⏮
                  </button>
                  <button
                    className="player-button player-button-primary player-button-large"
                    onClick={() => void onPlayPause()}
                    aria-label={isPlaying ? "Pause" : "Play"}
                  >
                    {activeIsLoading ? "…" : activeIsPlaying ? "❚❚" : "▶"}
                  </button>
                  <button
                    className="player-button"
                    onClick={() => void onNext()}
                    disabled={isLoading}
                    aria-label="Next track"
                  >
                    ⏭
                  </button>
                  <button
                    className={`player-button ${shuffleEnabled ? "active" : ""}`}
                    onClick={onToggleShuffle}
                    aria-label="Toggle shuffle"
                  >
                    ⇄
                  </button>
                  <button
                    className={`player-button ${repeatMode !== "off" ? "active" : ""}`}
                    onClick={onToggleRepeat}
                    aria-label="Toggle repeat mode"
                  >
                    {repeatMode === "one" ? "1" : "↺"}
                  </button>
                </div>
                <div className="player-progress-row player-progress-row-full">
                  <span>{formatDuration(videoOpen || videoFullscreenOpen ? videoCurrentTime : currentTime)}</span>
                  <input
                    className="player-range"
                    type="range"
                    min={0}
                    max={100}
                    step={0.1}
                    value={Math.max(0, Math.min(100, seekValue * 100))}
                    onChange={(event) => onSeek(Number(event.target.value) / 100)}
                    onPointerDown={(event) =>
                      onSeekStart(Number((event.currentTarget as HTMLInputElement).value) / 100)
                    }
                    onPointerUp={(event) => onSeekEnd(Number((event.currentTarget as HTMLInputElement).value) / 100)}
                    onPointerCancel={(event) =>
                      onSeekEnd(Number((event.currentTarget as HTMLInputElement).value) / 100)
                    }
                    onMouseUp={(event) => onSeekEnd(Number((event.currentTarget as HTMLInputElement).value) / 100)}
                    onTouchEnd={(event) => onSeekEnd(Number((event.currentTarget as HTMLInputElement).value) / 100)}
                    disabled={(videoOpen || videoFullscreenOpen ? videoDuration : duration) <= 0}
                  />
                  <span>{formatDuration(videoOpen || videoFullscreenOpen ? videoDuration : duration)}</span>
                </div>
                <label className="player-volume player-volume-full">
                  <span>Volume</span>
                  <input
                    className="player-range"
                    type="range"
                    min={0}
                    max={1}
                    step={0.01}
                    value={activeVolume}
                    onChange={(event) => onVolume(Number(event.target.value))}
                  />
                  <button className="player-button" onClick={onToggleMute} aria-label={activeMuted ? "Unmute" : "Mute"}>
                    {activeMuted ? "🔇" : "🔊"}
                  </button>
                </label>
                {videoError ? <div className="player-error">{videoError}</div> : null}
                {!videoError && error ? <div className="player-error">{error}</div> : null}
              </div>
            </section>

            <section className="full-player-right">
              <LyricsScroller
                variant="player"
                track={track}
                preview={preview}
                previewLoading={previewLoading}
                previewStatus=""
                currentTime={videoOpen || videoFullscreenOpen ? videoCurrentTime : currentTime}
                onSeekTime={(seconds) => {
                  const activeDuration = videoOpen || videoFullscreenOpen ? videoDuration : duration;
                  if (activeDuration > 0) {
                    onSeekEnd(Math.max(0, Math.min(1, seconds / activeDuration)));
                  }
                }}
              />
            </section>
          </div>
        )}
      </div>
      {videoOpen && videoFullscreenOpen ? (
        <div className="video-fullscreen-modal video-theater-modal">
          <div className="modal-backdrop video-theater-backdrop" />
          <div className="video-fullscreen-shell">
            <div className="video-fullscreen-head">
              <button className="icon-button" onClick={onCloseVideo} aria-label="Exit video fullscreen">
                ✕
              </button>
            </div>
            <div className="video-fullscreen-stage">
              {videoSource ? (
                <VideoStage
                  key={`${videoSource}-fullscreen`}
                  playerRef={videoPlayerRef}
                  source={videoSource}
                  sourceType={resolvedVideoSourceType}
                  poster={artwork}
                  startTime={videoOpen || videoFullscreenOpen ? videoCurrentTime : currentTime}
                  playing={resolvedVideoShouldPlay}
                  volume={videoVolume}
                  muted={videoMuted}
                  loading={videoLoading}
                  errorMessage={videoError}
                  onTimeChange={onVideoTimeChange}
                  onDurationChange={onVideoDurationChange}
                  onPlayingChange={onVideoPlayingChange}
                  onError={onVideoError}
                  onLoaded={onVideoLoaded}
                  onToggleFullscreen={() => undefined}
                  onClose={onCloseVideo}
                  onRetry={onOpenVideo}
                  showFullscreenToggle={false}
                  fullscreen
                />
              ) : (
                <div className="video-fullscreen-fallback">
                  <div className="video-fullscreen-fallback-art">
                    {artwork ? <img src={artwork} alt="" /> : initialsForTrack(track)}
                  </div>
                  <div className="video-fullscreen-fallback-copy">
                    <div className="detail-title">Video unavailable</div>
                    <div className="detail-note">{videoError || "No video file is available for this track yet."}</div>
                    <div className="action-row">
                      {canDownloadVideo ? (
                        <button className="button button-primary" onClick={() => void onDownloadVideo()}>
                          Download video
                        </button>
                      ) : null}
                      <button className="button button-secondary" onClick={onCloseVideo}>
                        Close video
                      </button>
                    </div>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
