import { useEffect, useRef } from "react";
import type { MutableRefObject } from "react";
import videojs from "video.js";
import type Player from "video.js/dist/types/player";
import "video.js/dist/video-js.css";
import type { VideoStageError } from "../../lib/videoDiagnostics";

type VideoStageProps = {
  source: string;
  sourceType: string;
  poster: string;
  startTime: number;
  playing: boolean;
  volume: number;
  muted: boolean;
  loading: boolean;
  errorMessage: string;
  fullscreen: boolean;
  playerRef: MutableRefObject<Player | null>;
  onTimeChange: (seconds: number) => void;
  onDurationChange: (seconds: number) => void;
  onPlayingChange: (value: boolean) => void;
  onError: (value: VideoStageError) => void;
  onLoaded: () => void;
  onToggleFullscreen: () => void;
  onClose: () => void;
  onRetry: () => void | Promise<void>;
  showFullscreenToggle?: boolean;
};

export function VideoStage({
  source,
  sourceType,
  poster,
  startTime,
  playing,
  volume,
  muted,
  loading,
  errorMessage,
  fullscreen,
  playerRef,
  onTimeChange,
  onDurationChange,
  onPlayingChange,
  onError,
  onLoaded,
  onToggleFullscreen,
  onClose,
  onRetry,
  showFullscreenToggle = true,
}: VideoStageProps) {
  const videoElementRef = useRef<HTMLVideoElement | null>(null);
  const seekAppliedRef = useRef(false);
  const loadedNotifiedRef = useRef(false);
  const initialMediaStateRef = useRef({ startTime, volume, muted });
  const callbacksRef = useRef({
    onTimeChange,
    onDurationChange,
    onPlayingChange,
    onError,
    onLoaded,
  });

  useEffect(() => {
    callbacksRef.current = {
      onTimeChange,
      onDurationChange,
      onPlayingChange,
      onError,
      onLoaded,
    };
  }, [onTimeChange, onDurationChange, onPlayingChange, onError, onLoaded]);

  useEffect(() => {
    initialMediaStateRef.current = { startTime, volume, muted };
  }, [muted, startTime, volume]);

  useEffect(() => {
    const element = videoElementRef.current;
    if (!element || !source) {
      return;
    }

    seekAppliedRef.current = false;
    loadedNotifiedRef.current = false;

    const player = videojs(element, {
      controls: true,
      autoplay: false,
      preload: "auto",
      fluid: true,
      responsive: true,
      playsinline: true,
      poster,
      playbackRates: [0.75, 1, 1.25, 1.5, 2],
      controlBar: {
        pictureInPictureToggle: false,
      },
    });

    playerRef.current = player;
    player.src({ src: source, type: sourceType || "video/mp4" });
    player.volume(initialMediaStateRef.current.volume);
    player.muted(initialMediaStateRef.current.muted);

    const syncState = () => {
      callbacksRef.current.onTimeChange(player.currentTime() || 0);
      callbacksRef.current.onDurationChange(player.duration() || 0);
      callbacksRef.current.onPlayingChange(!player.paused());
    };

    const applyInitialSeek = () => {
      const initialStartTime = initialMediaStateRef.current.startTime;
      if (!seekAppliedRef.current && initialStartTime > 0) {
        try {
          player.currentTime(initialStartTime);
        } catch {
          // ignore seek failures on initial load
        }
      }
      seekAppliedRef.current = true;
    };

    const reportLoaded = () => {
      if (loadedNotifiedRef.current) {
        return;
      }
      loadedNotifiedRef.current = true;
      callbacksRef.current.onLoaded();
    };

    const handleLoadedMetadata = () => {
      applyInitialSeek();
      callbacksRef.current.onDurationChange(player.duration() || 0);
      syncState();
    };

    const handleLoadedData = () => {
      applyInitialSeek();
      reportLoaded();
      syncState();
    };

    const handleCanPlay = () => {
      applyInitialSeek();
      reportLoaded();
      syncState();
    };

    player.on("loadedmetadata", handleLoadedMetadata);
    player.one("loadeddata", handleLoadedData);
    player.one("canplay", handleCanPlay);
    player.on("timeupdate", syncState);
    player.on("durationchange", () => callbacksRef.current.onDurationChange(player.duration() || 0));
    player.on("play", () => callbacksRef.current.onPlayingChange(true));
    player.on("pause", () => callbacksRef.current.onPlayingChange(false));
    player.on("ended", () => callbacksRef.current.onPlayingChange(false));
    player.on("error", () => {
      const error = player.error();
      console.error("videojs_error", { code: error?.code, message: error?.message, source });
      callbacksRef.current.onError({
        browserEvent: "error",
        playerErrorCode: error?.code ?? null,
        playerErrorMessage: error?.message ?? "",
        mimeType: player.currentType?.() || sourceType,
      });
    });
    player.on("stalled", () => {
      syncState();
    });
    player.on("abort", () => {
      syncState();
    });
    player.on("emptied", () => {
      syncState();
    });

    return () => {
      try {
        player.pause();
      } catch {
        // ignore cleanup failures
      }
      player.dispose();
      if (playerRef.current === player) {
        playerRef.current = null;
      }
    };
  }, [playerRef, poster, source, sourceType]);

  useEffect(() => {
    const player = playerRef.current;
    if (!player) return;
    player.muted(muted);
  }, [muted, playerRef]);

  useEffect(() => {
    const player = playerRef.current;
    if (!player) return;
    player.volume(volume);
  }, [playerRef, volume]);

  useEffect(() => {
    const player = playerRef.current;
    if (!player) return;
    if (playing && player.paused()) {
      const playPromise = player.play?.();
      if (playPromise && typeof playPromise.catch === "function") {
        void playPromise.catch(() => undefined);
      }
    } else if (!playing && !player.paused()) {
      player.pause();
    }
  }, [playing, playerRef]);

  return (
    <div className={`video-stage-shell ${fullscreen ? "video-stage-fullscreen" : "video-stage-inline"}`}>
      <video ref={videoElementRef} className="video-js vjs-default-skin video-stage-element" playsInline />
      <div className="video-stage-overlay">
        <div className="video-stage-status">
          {loading ? <div className="video-stage-loading">Loading video…</div> : null}
          {!loading && errorMessage ? <div className="video-stage-error">{errorMessage}</div> : null}
        </div>
        <div className="video-stage-controls">
          <button className="icon-button" onClick={onClose} aria-label="Close video">
            ✕
          </button>
          {showFullscreenToggle ? (
            <button
              className="icon-button"
              onClick={onToggleFullscreen}
              aria-label={fullscreen ? "Exit fullscreen" : "Fullscreen video"}
            >
              {fullscreen ? "⤡" : "⤢"}
            </button>
          ) : null}
        </div>
      </div>
      {!loading && errorMessage ? (
        <div className="video-stage-fallback">
          <div className="video-stage-fallback-title">Video unavailable</div>
          <div className="video-stage-fallback-text">{errorMessage}</div>
          <div className="video-stage-fallback-actions">
            <button className="button button-primary" onClick={() => void onRetry()}>
              Retry video
            </button>
            <button className="button button-secondary" onClick={onClose}>
              Back to artwork
            </button>
            <button
              className="button button-secondary"
              onClick={onToggleFullscreen}
              disabled={fullscreen}
              aria-label="Fullscreen video"
            >
              {fullscreen ? "Fullscreen" : "Expand"}
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
