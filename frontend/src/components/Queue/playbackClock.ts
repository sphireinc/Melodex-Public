export type PlaybackClock = {
  trackID: string;
  baseTime: number;
  startedAt: number;
};

export function playbackTimeAt(now: number, clock: PlaybackClock, duration: number) {
  const elapsed = Math.max(0, now - clock.startedAt) / 1000;
  const nextTime = Math.max(0, clock.baseTime + elapsed);
  return duration > 0 ? Math.min(duration, nextTime) : nextTime;
}
