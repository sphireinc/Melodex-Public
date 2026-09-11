import { useEffect, useMemo, useRef, useState } from "react";
import { EmptyState, StatusBadge } from "../Common";
import { formatDuration } from "../../lib/viewHelpers";
import type { TrackPreview, TrackRecord } from "../../types";

type LyricsScrollerProps = {
  variant: "panel" | "player";
  track: TrackRecord | null;
  preview: TrackPreview | null;
  previewLoading: boolean;
  previewStatus: string;
  currentTime: number;
  onSeekTime?: (seconds: number) => void;
};

type TimedLyricLine = {
  time: number;
  text: string;
};

export function LyricsScroller({
  variant,
  track,
  preview,
  previewLoading,
  previewStatus,
  currentTime,
  onSeekTime,
}: LyricsScrollerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const lineRefs = useRef<Array<HTMLElement | null>>([]);
  const autoScrollingRef = useRef(false);
  const scrollFrameRef = useRef<number | null>(null);
  const lastCenteredIndexRef = useRef(-1);
  const [followLive, setFollowLive] = useState(true);

  const timedLines = useMemo(() => parseLrc(preview?.timedLyricsText ?? ""), [preview?.timedLyricsText]);
  const hasTimed = timedLines.length > 0;
  const plainLines = useMemo(
    () => (preview?.lyricsText ?? "").split(/\r?\n/).map((line) => line.trimEnd()),
    [preview?.lyricsText],
  );
  const activeIndex = useMemo(
    () => (hasTimed ? activeLyricIndex(timedLines, currentTime) : -1),
    [hasTimed, timedLines, currentTime],
  );
  const autoFollow = variant === "player" ? true : followLive;

  useEffect(() => {
    if (!autoFollow || activeIndex < 0 || activeIndex === lastCenteredIndexRef.current) return;
    const container = containerRef.current;
    const element = lineRefs.current[activeIndex];
    if (!container || !element) return;
    lastCenteredIndexRef.current = activeIndex;
    smoothScrollLine(container, element, scrollFrameRef, autoScrollingRef);
  }, [activeIndex, autoFollow]);

  useEffect(() => {
    lastCenteredIndexRef.current = -1;
    setFollowLive(true);
    if (scrollFrameRef.current !== null) {
      window.cancelAnimationFrame(scrollFrameRef.current);
      scrollFrameRef.current = null;
    }
    autoScrollingRef.current = false;
  }, [preview?.timedLyricsText, preview?.lyricsText, track?.id]);

  useEffect(
    () => () => {
      if (scrollFrameRef.current !== null) {
        window.cancelAnimationFrame(scrollFrameRef.current);
      }
    },
    [],
  );

  function handleScroll() {
    if (autoScrollingRef.current) return;
    if (variant !== "player" && hasTimed) setFollowLive(false);
  }

  if (!track) {
    return <EmptyState title="No lyrics available" text="Timed lyrics will appear here when an LRC file exists." />;
  }

  if (previewLoading && !preview) {
    return <div className="lyrics-loading">{previewStatus || "Loading lyrics..."}</div>;
  }

  if (!hasTimed && plainLines.every((line) => !line.trim())) {
    return (
      <EmptyState
        title="No lyrics saved for this track."
        text="Melodex can save plain lyrics when provided or safely transcribed."
      />
    );
  }

  return (
    <div
      className={`lyrics-scroller ${variant === "player" ? "lyrics-scroller-player" : "lyrics-scroller-panel"}`}
      ref={containerRef}
      onScroll={handleScroll}
    >
      {hasTimed ? (
        <>
          {variant === "panel" ? (
            <div className="lyrics-scroller-toolbar">
              <StatusBadge tone="accent">Timed lyrics</StatusBadge>
              <button
                className="text-action"
                onClick={() => {
                  lastCenteredIndexRef.current = -1;
                  setFollowLive(true);
                }}
              >
                Follow live
              </button>
            </div>
          ) : null}
          <div className="lyrics-lines">
            {timedLines.map((line, index) => {
              const distance = activeIndex < 0 ? 999 : Math.abs(index - activeIndex);
              const active = index === activeIndex;
              const lineClass = `lyric-line ${active ? "active" : ""} distance-${Math.min(distance, 4)}`;
              if (variant === "player") {
                return (
                  <button
                    key={`${line.time}-${index}`}
                    ref={(element) => {
                      lineRefs.current[index] = element;
                    }}
                    className={`${lineClass} lyric-line-button`}
                    onClick={() => {
                      if (typeof onSeekTime === "function") {
                        onSeekTime(line.time);
                      }
                    }}
                    type="button"
                    aria-label={`Seek to ${formatDuration(line.time)}`}
                  >
                    <span className="lyric-text">{line.text || " "}</span>
                  </button>
                );
              }
              return (
                <div
                  key={`${line.time}-${index}`}
                  ref={(element) => {
                    lineRefs.current[index] = element;
                  }}
                  className={lineClass}
                >
                  <span className="lyric-text">{line.text || " "}</span>
                </div>
              );
            })}
          </div>
        </>
      ) : (
        <>
          <div className="lyrics-scroller-toolbar">
            <StatusBadge tone="neutral">Plain lyrics</StatusBadge>
            <span className="lyrics-note">No timing data available.</span>
          </div>
          <div className="lyrics-lines plain-lines">
            {plainLines.length === 0 ? (
              <div className="lyrics-note">No lyrics available.</div>
            ) : (
              plainLines.map((line, index) => (
                <div key={`${line}-${index}`} className="plain-lyric-line">
                  {line || "\u00A0"}
                </div>
              ))
            )}
          </div>
        </>
      )}
    </div>
  );
}

function smoothScrollLine(
  container: HTMLElement,
  element: HTMLElement,
  scrollFrameRef: { current: number | null },
  autoScrollingRef: { current: boolean },
) {
  const start = container.scrollTop;
  const maxScroll = Math.max(0, container.scrollHeight - container.clientHeight);
  const containerRect = container.getBoundingClientRect();
  const elementRect = element.getBoundingClientRect();
  const elementCenter = elementRect.top - containerRect.top + container.scrollTop + elementRect.height / 2;
  const target = Math.max(0, Math.min(maxScroll, elementCenter - container.clientHeight / 2));
  const distance = target - start;
  if (Math.abs(distance) < 1) {
    container.scrollTop = target;
    return;
  }

  const duration = 240;
  const startedAt = performance.now();
  autoScrollingRef.current = true;

  if (scrollFrameRef.current !== null) {
    window.cancelAnimationFrame(scrollFrameRef.current);
  }

  const easeInOutCubic = (value: number) =>
    value < 0.5 ? 4 * value * value * value : 1 - Math.pow(-2 * value + 2, 3) / 2;

  const tick = (now: number) => {
    const progress = Math.min(1, (now - startedAt) / duration);
    const eased = easeInOutCubic(progress);
    container.scrollTop = start + distance * eased;
    if (progress < 1) {
      scrollFrameRef.current = window.requestAnimationFrame(tick);
      return;
    }
    autoScrollingRef.current = false;
    scrollFrameRef.current = null;
  };

  scrollFrameRef.current = window.requestAnimationFrame(tick);
}

function parseLrc(text: string): TimedLyricLine[] {
  const lines: TimedLyricLine[] = [];
  for (const rawLine of text.split(/\r?\n/)) {
    const timestamps = Array.from(rawLine.matchAll(/\[(\d{1,2}):(\d{2})(?:[.:](\d{1,3}))?\]/g));
    if (timestamps.length === 0) continue;
    const lyricText = rawLine.replace(/\[(\d{1,2}):(\d{2})(?:[.:](\d{1,3}))?\]/g, "").trim();
    for (const match of timestamps) {
      const minutes = Number(match[1]);
      const seconds = Number(match[2]);
      const fraction = Number((match[3] ?? "0").padEnd(3, "0")) / 1000;
      lines.push({ time: minutes * 60 + seconds + fraction, text: lyricText });
    }
  }
  return lines.sort((left, right) => left.time - right.time);
}

function activeLyricIndex(lines: TimedLyricLine[], currentTime: number) {
  let index = -1;
  for (let item = 0; item < lines.length; item++) {
    const line = lines[item];
    if (line && line.time <= currentTime + 0.05) {
      index = item;
    } else {
      break;
    }
  }
  return index;
}
