import { useEffect, useRef, useState } from "react";
import type { PlaybackState, TrackRecord, AppState } from "../types";
import type Player from "video.js/dist/types/player";
import {
  downloadTrackVideo,
  mediaInfo,
  logVideoEvent,
  pausePlayback,
  seekPlayback as backendSeekPlayback,
  setPlaybackVolume as backendSetPlaybackVolume,
  toggleMute as backendToggleMute,
  togglePlayback as backendTogglePlayback,
} from "../lib/backend";
import { serializeVideoDiagnosticContext, videoSourceType as videoSourceKind } from "../lib/videoDiagnostics";

type UseVideoPlaybackArgs = {
  player: PlaybackState;
  downloadMode: string;
  setPlayer: (next: PlaybackState) => void;
  setState: (next: AppState) => void;
  trackById: (trackID: string) => TrackRecord | null;
};

function logVideoDiagnostic(
  event: string,
  track: TrackRecord | null,
  context: Parameters<typeof serializeVideoDiagnosticContext>[0],
) {
  void logVideoEvent(
    event,
    `track:${track?.id || "unknown"}`,
    serializeVideoDiagnosticContext({
      trackId: track?.id,
      sourcePathAvailable: Boolean(track?.videoPath?.trim()),
      remoteSourceAvailable: Boolean(track?.sourceRef?.trim()),
      ...context,
    }),
  );
}

export function useVideoPlayback({ player, downloadMode, setPlayer, setState, trackById }: UseVideoPlaybackArgs) {
  const [videoOpen, setVideoOpen] = useState(false);
  const [videoFullscreenOpen, setVideoFullscreenOpen] = useState(false);
  const [videoSourceUrl, setVideoSourceUrl] = useState("");
  const [videoSourceType, setVideoSourceType] = useState("video/mp4");
  const [videoCurrentTime, setVideoCurrentTime] = useState(0);
  const [videoDuration, setVideoDuration] = useState(0);
  const [videoIsPlaying, setVideoIsPlaying] = useState(false);
  const [videoShouldPlay, setVideoShouldPlay] = useState(false);
  const [videoLoading, setVideoLoading] = useState(false);
  const [videoError, setVideoError] = useState("");
  const [videoMuted, setVideoMuted] = useState(false);
  const [videoVolume, setVideoVolume] = useState(0.84);
  const [videoTrackAvailable, setVideoTrackAvailable] = useState(false);
  const videoPlayerRef = useRef<Player | null>(null);
  const videoWasPlayingRef = useRef(false);

  const closeVideo = async (resumeAudio = false) => {
    const playerRef = videoPlayerRef.current;
    const currentTime = playerRef?.currentTime?.() ?? videoCurrentTime;
    const duration =
      playerRef?.duration?.() ?? (videoDuration > 0 ? videoDuration : player.duration > 0 ? player.duration : 0);
    const seekRatio = duration > 0 ? Math.max(0, Math.min(1, currentTime / duration)) : 0;
    setVideoFullscreenOpen(false);
    setVideoOpen(false);
    setVideoLoading(false);
    setVideoError("");
    setVideoSourceUrl("");
    setVideoCurrentTime(currentTime);
    setVideoDuration(duration > 0 ? duration : videoDuration);
    setVideoShouldPlay(false);
    setVideoIsPlaying(false);
    if (duration > 0) {
      try {
        const next = await backendSeekPlayback(seekRatio);
        setPlayer(next);
      } catch {
        // ignore
      }
    }
    const shouldResume = resumeAudio && videoWasPlayingRef.current && !(playerRef?.paused?.() ?? !videoIsPlaying);
    videoWasPlayingRef.current = false;
    if (shouldResume) {
      try {
        const next = await backendTogglePlayback();
        setPlayer(next);
      } catch {
        // ignore
      }
    }
  };

  const syncVideoFromAudio = (useAudioPosition = true) => {
    setVideoVolume(player.volume);
    setVideoMuted(player.muted);
    setVideoCurrentTime(useAudioPosition ? player.currentTime : 0);
    setVideoDuration(useAudioPosition ? player.duration : 0);
  };

  const openVideoForTrack = async (track: TrackRecord, fullScreen = false, forceRefresh = false) => {
    const sourcePath = track.videoPath?.trim();
    let activeTrack = track;
    let finalVideoPath = sourcePath;
    if (finalVideoPath && !forceRefresh) {
      try {
        await mediaInfo(finalVideoPath);
      } catch {
        finalVideoPath = "";
      }
    }
    if ((!finalVideoPath || forceRefresh) && track.sourceRef.trim()) {
      setVideoLoading(true);
      setVideoError("");
      try {
        const next = await downloadTrackVideo(track.id);
        setState(next);
        setPlayer(next.playback);
        activeTrack = next.libraryTracks.find((item) => item.id === track.id) ?? trackById(track.id) ?? track;
        finalVideoPath = activeTrack.videoPath?.trim() ?? "";
      } catch (error) {
        setVideoLoading(false);
        setVideoTrackAvailable(false);
        const message = error instanceof Error ? error.message : String(error);
        setVideoError(message);
        logVideoDiagnostic("video_download_failed", track, {
          sourceType: videoSourceKind(track.videoPath ?? "", "", track.sourceRef),
          browserEvent: "download",
          mode: fullScreen ? "fullscreen" : "inline",
          downloadMode,
          message,
        });
        return;
      }
    }

    if (!finalVideoPath) {
      setVideoLoading(false);
      setVideoTrackAvailable(false);
      setVideoError("No video available for this track.");
      logVideoDiagnostic("video_missing_after_download", activeTrack, {
        sourceType: videoSourceKind(activeTrack.videoPath ?? "", "", activeTrack.sourceRef),
        browserEvent: "download-complete",
        mode: fullScreen ? "fullscreen" : "inline",
        downloadMode,
        message: "No video available for this track.",
      });
      return;
    }

    let mediaUrlValue = "";
    let mediaTypeValue = "video/mp4";
    try {
      const media = await mediaInfo(finalVideoPath);
      if (media.reachable === false) {
        const status = media.statusCode ? ` HTTP status ${media.statusCode}.` : "";
        throw new Error(`Media server URL was created but is not reachable.${status}`);
      }
      mediaUrlValue = media.url;
      mediaTypeValue = media.mimeType || mediaTypeValue;
      logVideoDiagnostic("video_media_ready", activeTrack, {
        sourceType: "media-server",
        mimeType: mediaTypeValue,
        browserEvent: "media-info",
        mode: fullScreen ? "fullscreen" : "inline",
        downloadMode,
        message: `reachable=${media.reachable ?? "unknown"} status=${media.statusCode ?? "unknown"} content_length=${media.contentLength ?? "unknown"}`,
      });
    } catch (error) {
      setVideoLoading(false);
      setVideoTrackAvailable(false);
      const message = error instanceof Error ? error.message : String(error);
      setVideoError(message);
      logVideoDiagnostic("video_media_url_failed", activeTrack, {
        sourceType: videoSourceKind(finalVideoPath, "", activeTrack.sourceRef),
        browserEvent: "media-info",
        mode: fullScreen ? "fullscreen" : "inline",
        downloadMode,
        message,
      });
      return;
    }
    if (!mediaUrlValue) {
      setVideoLoading(false);
      setVideoTrackAvailable(false);
      setVideoError("Unable to load the video source.");
      logVideoDiagnostic("video_media_url_missing", activeTrack, {
        sourceType: videoSourceKind(finalVideoPath, "", activeTrack.sourceRef),
        browserEvent: "media-info",
        mode: fullScreen ? "fullscreen" : "inline",
        downloadMode,
        message: "Unable to load the video source.",
      });
      return;
    }

    const videoMatchesCurrentAudio = activeTrack.id === player.currentTrackId;
    videoWasPlayingRef.current = videoMatchesCurrentAudio && player.isPlaying;
    syncVideoFromAudio(videoMatchesCurrentAudio);
    setVideoError("");
    setVideoLoading(true);
    setVideoShouldPlay(true);
    setVideoIsPlaying(false);
    setVideoSourceUrl(mediaUrlValue);
    setVideoSourceType(mediaTypeValue);
    setVideoTrackAvailable(true);
    setVideoOpen(true);
    setVideoFullscreenOpen(fullScreen);
    if (videoMatchesCurrentAudio && player.isPlaying) {
      try {
        await pausePlayback();
      } catch (error) {
        setVideoError(error instanceof Error ? error.message : String(error));
      }
    }
  };

  const handleVideoSeek = async (seconds: number) => {
    const playerRef = videoPlayerRef.current;
    if (!playerRef) return;
    const nextTime = Math.max(0, seconds);
    const source = playerRef.currentSrc?.() || videoSourceUrl || "";
    try {
      playerRef.currentTime(nextTime);
      if (videoFullscreenOpen || videoOpen) {
        setVideoShouldPlay(true);
        const playPromise = playerRef.play?.();
        if (playPromise && typeof playPromise.catch === "function") {
          await playPromise.catch(() => undefined);
        }
      }
      setVideoCurrentTime(nextTime);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setVideoError(message);
      const track = player.currentTrackId ? trackById(player.currentTrackId) : null;
      logVideoDiagnostic("video_seek_failed", track, {
        sourceType: videoSourceKind(track?.videoPath ?? "", source, track?.sourceRef ?? ""),
        browserEvent: "seek",
        mode: videoFullscreenOpen ? "fullscreen" : videoOpen ? "inline" : "closed",
        downloadMode,
        message,
      });
    }
  };

  const handleVideoPlayPause = async () => {
    const playerRef = videoPlayerRef.current;
    if (!playerRef) return;
    const source = playerRef.currentSrc?.() || videoSourceUrl || "";
    try {
      if (playerRef.paused()) {
        setVideoShouldPlay(true);
        await playerRef.play();
      } else {
        setVideoShouldPlay(false);
        playerRef.pause();
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setVideoError(message);
      const track = player.currentTrackId ? trackById(player.currentTrackId) : null;
      logVideoDiagnostic("video_toggle_play_failed", track, {
        sourceType: videoSourceKind(track?.videoPath ?? "", source, track?.sourceRef ?? ""),
        browserEvent: "play",
        mode: videoFullscreenOpen ? "fullscreen" : videoOpen ? "inline" : "closed",
        downloadMode,
        message,
      });
    }
  };

  const handleVideoVolume = async (value: number) => {
    const nextVolume = Math.max(0, Math.min(1, value));
    const playerRef = videoPlayerRef.current;
    const nextMuted = nextVolume <= 0;
    if (playerRef) {
      playerRef.volume(nextVolume);
      playerRef.muted(nextMuted);
    }
    setVideoVolume(nextVolume);
    setVideoMuted(nextMuted);
    const next = await backendSetPlaybackVolume(nextVolume);
    setPlayer(next);
  };

  const handleVideoToggleMute = async () => {
    const playerRef = videoPlayerRef.current;
    const nextMuted = !videoMuted;
    if (playerRef) {
      playerRef.muted(nextMuted);
    }
    setVideoMuted(nextMuted);
    const next = await backendToggleMute();
    setPlayer(next);
    setVideoVolume(next.volume);
  };

  useEffect(() => {
    if (player.currentTrackId) {
      setVideoSourceUrl("");
      setVideoSourceType("video/mp4");
      setVideoShouldPlay(false);
      setVideoError("");
      setVideoLoading(false);
    }
  }, [player.currentTrackId]);

  useEffect(() => {
    if (!player.currentTrackId) {
      setVideoOpen(false);
      setVideoFullscreenOpen(false);
      setVideoError("");
      setVideoLoading(false);
      setVideoSourceUrl("");
      setVideoSourceType("video/mp4");
      setVideoShouldPlay(false);
      setVideoCurrentTime(0);
      setVideoDuration(0);
      setVideoIsPlaying(false);
      videoWasPlayingRef.current = false;
    }
  }, [player.currentTrackId]);

  useEffect(() => {
    let cancelled = false;
    const currentTrack = player.currentTrackId ? trackById(player.currentTrackId) : null;
    const sourcePath = currentTrack?.videoPath?.trim() ?? "";

    if (!currentTrack || !sourcePath) {
      setVideoTrackAvailable(false);
      return;
    }

    void (async () => {
      try {
        await mediaInfo(sourcePath);
        if (!cancelled) {
          setVideoTrackAvailable(true);
        }
      } catch {
        if (!cancelled) {
          setVideoTrackAvailable(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [player.currentTrackId, trackById]);

  return {
    videoOpen,
    videoFullscreenOpen,
    videoSourceUrl,
    videoSourceType,
    videoCurrentTime,
    videoDuration,
    videoIsPlaying,
    videoShouldPlay,
    videoLoading,
    videoError,
    videoMuted,
    videoVolume,
    videoPlayerRef,
    openVideoForTrack,
    closeVideo,
    handleVideoSeek,
    handleVideoPlayPause,
    handleVideoVolume,
    handleVideoToggleMute,
    setVideoCurrentTime,
    setVideoDuration,
    setVideoError,
    setVideoLoading,
    setVideoOpen,
    setVideoFullscreenOpen,
    setVideoSourceUrl,
    setVideoSourceType,
    setVideoShouldPlay,
    setVideoIsPlaying,
    setVideoMuted,
    setVideoVolume,
    videoTrackAvailable,
  };
}
