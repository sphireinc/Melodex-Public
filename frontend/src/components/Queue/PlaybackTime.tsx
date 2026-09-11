import { useEffect, useRef, useState, type MutableRefObject, type ReactNode } from "react";
import type { PlaybackState } from "../../types";
import { playbackTimeAt, type PlaybackClock } from "./playbackClock";

export type PlaybackTimeValues = {
  audioCurrentTime: number;
  seekDisplayCurrentTime: number;
  seekDisplayRatio: number;
};

export function PlaybackTime({
  player,
  videoOpen,
  videoFullscreenOpen,
  videoCurrentTime,
  videoDuration,
  seekPreviewRatio,
  liveAudioTimeRef,
  children,
}: {
  player: PlaybackState;
  videoOpen: boolean;
  videoFullscreenOpen: boolean;
  videoCurrentTime: number;
  videoDuration: number;
  seekPreviewRatio: number | null;
  liveAudioTimeRef: MutableRefObject<number>;
  children: (values: PlaybackTimeValues) => ReactNode;
}) {
  const [liveAudioTime, setLiveAudioTime] = useState(() => player.currentTime);
  const liveAudioClockRef = useRef<PlaybackClock | null>(null);

  useEffect(() => {
    const publish = (nextTime: number) => {
      liveAudioTimeRef.current = nextTime;
      setLiveAudioTime(nextTime);
    };
    const shouldAnimateAudio = Boolean(player.currentTrackId) && player.isPlaying && !videoOpen && !videoFullscreenOpen;
    if (!shouldAnimateAudio) {
      liveAudioClockRef.current = null;
      publish(player.currentTime);
      return;
    }

    const nextBaseTime = Math.max(0, player.currentTime);
    const existing = liveAudioClockRef.current;
    if (!existing || existing.trackID !== player.currentTrackId || Math.abs(existing.baseTime - nextBaseTime) > 0.75) {
      liveAudioClockRef.current = {
        trackID: player.currentTrackId,
        baseTime: nextBaseTime,
        startedAt: performance.now(),
      };
      publish(nextBaseTime);
    }

    let frame = 0;
    const tick = () => {
      const clock = liveAudioClockRef.current;
      if (!clock || clock.trackID !== player.currentTrackId || !player.isPlaying || videoOpen || videoFullscreenOpen) {
        return;
      }
      publish(playbackTimeAt(performance.now(), clock, player.duration));
      frame = window.requestAnimationFrame(tick);
    };

    frame = window.requestAnimationFrame(tick);
    return () => window.cancelAnimationFrame(frame);
  }, [
    liveAudioTimeRef,
    player.currentTrackId,
    player.currentTime,
    player.duration,
    player.isPlaying,
    videoOpen,
    videoFullscreenOpen,
  ]);

  const seekDuration =
    videoOpen || videoFullscreenOpen
      ? videoDuration > 0
        ? videoDuration
        : player.duration > 0
          ? player.duration
          : 0
      : player.duration > 0
        ? player.duration
        : 0;
  const audioCurrentTime = videoOpen || videoFullscreenOpen ? videoCurrentTime : liveAudioTime;
  const seekDisplayRatio =
    videoOpen || videoFullscreenOpen
      ? seekDuration > 0
        ? videoCurrentTime / seekDuration
        : 0
      : (seekPreviewRatio ?? (seekDuration > 0 ? audioCurrentTime / seekDuration : 0));
  const seekDisplayCurrentTime =
    videoOpen || videoFullscreenOpen
      ? videoCurrentTime
      : seekPreviewRatio != null && seekDuration > 0
        ? seekPreviewRatio * seekDuration
        : audioCurrentTime;

  return children({ audioCurrentTime, seekDisplayCurrentTime, seekDisplayRatio });
}
