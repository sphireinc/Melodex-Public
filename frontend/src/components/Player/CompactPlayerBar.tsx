import { type KeyboardEvent } from "react";
import { StatusBadge } from "../Common";
import { formatDuration, initialsForTrack, getArtworkClickAction } from "../../lib/viewHelpers";
import type { RepeatMode, TrackRecord } from "../../types";

export type CompactPlayerBarProps = {
  stacked: boolean;
  track: TrackRecord | null;
  artworkDataUrl: string;
  hasVideoDownloaded: boolean;
  currentTime: number;
  duration: number;
  seekValue: number;
  isPlaying: boolean;
  isLoading: boolean;
  muted: boolean;
  volume: number;
  shuffleEnabled: boolean;
  repeatMode: RepeatMode;
  error: string;
  queueLength: number;
  lyricsOpen: boolean;
  onOpenPlayer: () => void;
  onOpenCurrentTrack: () => void;
  onPrevious: () => Promise<void>;
  onPlayPause: () => Promise<void>;
  onNext: () => Promise<void>;
  onSeek: (ratio: number) => void;
  onSeekStart: (ratio: number) => void;
  onSeekEnd: (ratio: number) => void;
  onVolume: (value: number) => void;
  onToggleMute: () => void;
  onToggleLyrics: () => void;
  onToggleQueue: () => void;
  onToggleShuffle: () => void;
  onToggleRepeat: () => void;
  onOpenPlaylistPicker: () => void;
};

export function CompactPlayerBar({
  stacked,
  track,
  artworkDataUrl,
  hasVideoDownloaded,
  currentTime,
  duration,
  seekValue,
  isPlaying,
  isLoading,
  muted,
  volume,
  shuffleEnabled,
  repeatMode,
  error,
  queueLength,
  lyricsOpen,
  onOpenPlayer,
  onOpenCurrentTrack,
  onPrevious,
  onPlayPause,
  onNext,
  onSeek,
  onSeekStart,
  onSeekEnd,
  onVolume,
  onToggleMute,
  onToggleLyrics,
  onToggleQueue,
  onToggleShuffle,
  onToggleRepeat,
  onOpenPlaylistPicker,
}: CompactPlayerBarProps) {
  const safeDuration = duration > 0 ? duration : 0;
  const progress = Math.max(0, Math.min(100, seekValue * 100));
  const artwork = artworkDataUrl || track?.artworkDataUrl || "";
  const artworkAction = getArtworkClickAction(track, hasVideoDownloaded);
  const showDownloadOverlay = artworkAction === "download-video";

  const utilityControls = (
    <>
      <button
        className={`player-button ${lyricsOpen ? "active" : ""}`}
        onClick={onToggleLyrics}
        aria-label="Toggle lyrics"
      >
        ✦
      </button>
      <button className="player-button player-queue-button" onClick={onToggleQueue} aria-label="Toggle queue">
        ☰{queueLength > 0 ? <span className="player-count">{queueLength}</span> : null}
      </button>
      <button className="player-button" onClick={onOpenPlaylistPicker} disabled={!track} aria-label="Add to playlist">
        ＋
      </button>
      <button
        className={`player-button ${muted ? "active" : ""}`}
        onClick={onToggleMute}
        disabled={!track}
        aria-label={muted ? "Unmute" : "Mute"}
      >
        {muted ? "🔇" : "🔊"}
      </button>
      <label className="player-volume">
        <input
          className="player-range"
          type="range"
          min={0}
          max={1}
          step={0.01}
          value={volume}
          onChange={(event) => onVolume(Number(event.target.value))}
          aria-label="Volume"
        />
      </label>
      <button className="player-button" onClick={onOpenPlayer} disabled={!track} aria-label="Open player">
        ⤢
      </button>
    </>
  );

  const handleTopKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.target !== event.currentTarget || (event.key !== "Enter" && event.key !== " ")) {
      return;
    }
    event.preventDefault();
    if (track) {
      onOpenCurrentTrack();
    } else {
      onOpenPlayer();
    }
  };

  return (
    <footer className={`player-bar compact-player-bar ${stacked ? "stacked" : ""}`}>
      <div
        className="player-top"
        role="button"
        tabIndex={0}
        onClick={track ? onOpenCurrentTrack : onOpenPlayer}
        onKeyDown={handleTopKeyDown}
        aria-label={track ? `Open ${track.title} track details` : "Open player"}
      >
        <div className="player-top-art">
          {artwork ? <img src={artwork} alt="" /> : track ? initialsForTrack(track) : "♪"}
          {showDownloadOverlay ? (
            <span className="video-art-overlay download" aria-hidden="true">
              ⇩
            </span>
          ) : null}
        </div>
        <div className="player-top-copy">
          <div className="player-title-row">
            <div className="player-title">{track?.title ?? "Nothing playing"}</div>
            {track ? (
              <StatusBadge tone={isPlaying ? "accent" : "neutral"}>{isPlaying ? "Playing" : "Paused"}</StatusBadge>
            ) : null}
          </div>
          <div className="player-subtitle">
            {track ? `${track.artist} · ${track.album}` : "Choose a track from your library"}
          </div>
          {error ? <div className="player-error">{error}</div> : null}
        </div>
      </div>

      <div className="player-middle">
        <div className="player-middle-top">
          <div className="player-controls">
            <button
              className="player-button"
              onClick={() => void onPrevious()}
              disabled={!track || queueLength === 0}
              aria-label="Previous track"
            >
              ⏮
            </button>
            <button
              className="player-button player-button-primary"
              onClick={() => void onPlayPause()}
              disabled={!track && queueLength === 0}
              aria-label={isLoading ? "Loading player" : isPlaying ? "Pause" : "Play"}
            >
              {isLoading ? "…" : isPlaying ? "❚❚" : "▶"}
            </button>
            <button
              className="player-button"
              onClick={() => void onNext()}
              disabled={!track || queueLength === 0}
              aria-label="Next track"
            >
              ⏭
            </button>
            <button
              className={`player-button ${shuffleEnabled ? "active" : ""}`}
              onClick={onToggleShuffle}
              disabled={queueLength === 0}
              aria-label="Toggle shuffle"
              aria-pressed={shuffleEnabled}
            >
              ⇄
            </button>
            <button
              className={`player-button ${repeatMode !== "off" ? "active" : ""}`}
              onClick={onToggleRepeat}
              disabled={queueLength === 0}
              aria-label={`Toggle repeat${repeatMode === "one" ? " one" : ""}`}
              aria-pressed={repeatMode !== "off"}
            >
              {repeatMode === "one" ? "1" : "↺"}
            </button>
          </div>
          {stacked ? <div className="player-utility-row">{utilityControls}</div> : null}
        </div>
        <div className="player-progress-row">
          <span>{formatDuration(currentTime)}</span>
          <input
            className="player-range"
            type="range"
            min={0}
            max={100}
            step={0.1}
            value={progress}
            onChange={(event) => onSeek(Number(event.target.value) / 100)}
            onPointerDown={(event) => onSeekStart(Number((event.currentTarget as HTMLInputElement).value) / 100)}
            onPointerUp={(event) => onSeekEnd(Number((event.currentTarget as HTMLInputElement).value) / 100)}
            onPointerCancel={(event) => onSeekEnd(Number((event.currentTarget as HTMLInputElement).value) / 100)}
            onMouseUp={(event) => onSeekEnd(Number((event.currentTarget as HTMLInputElement).value) / 100)}
            onTouchEnd={(event) => onSeekEnd(Number((event.currentTarget as HTMLInputElement).value) / 100)}
            disabled={!track || safeDuration <= 0}
            aria-label="Seek through track"
          />
          <span>{formatDuration(safeDuration)}</span>
        </div>
      </div>

      {!stacked ? <div className="player-bottom">{utilityControls}</div> : null}
    </footer>
  );
}
