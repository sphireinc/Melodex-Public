import { useEffect } from "react";
import type { Dispatch, SetStateAction } from "react";

type UseKeyboardShortcutsArgs = {
  currentTrackId: string;
  togglePlayback: () => void | Promise<void>;
  playPrevious: () => void | Promise<void>;
  playNext: () => void | Promise<void>;
  seekBySeconds: (seconds: number) => void;
  toggleMute: () => void | Promise<void>;
  setQueueOpen: Dispatch<SetStateAction<boolean>>;
  setLyricsOpen: Dispatch<SetStateAction<boolean>>;
};

export function useKeyboardShortcuts({
  currentTrackId,
  togglePlayback,
  playPrevious,
  playNext,
  seekBySeconds,
  toggleMute,
  setQueueOpen,
  setLyricsOpen,
}: UseKeyboardShortcutsArgs) {
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (
        target &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT" ||
          target.isContentEditable)
      ) {
        return;
      }
      const hasTrack = Boolean(currentTrackId);
      if (event.code === "Space") {
        event.preventDefault();
        void togglePlayback();
      } else if ((event.metaKey || event.ctrlKey) && event.key === "ArrowLeft") {
        event.preventDefault();
        void playPrevious();
      } else if ((event.metaKey || event.ctrlKey) && event.key === "ArrowRight") {
        event.preventDefault();
        void playNext();
      } else if (hasTrack && event.key === "ArrowLeft") {
        event.preventDefault();
        seekBySeconds(-5);
      } else if (hasTrack && event.key === "ArrowRight") {
        event.preventDefault();
        seekBySeconds(5);
      } else if (event.key.toLowerCase() === "l") {
        event.preventDefault();
        setQueueOpen(false);
        setLyricsOpen((current) => !current);
      } else if (event.key.toLowerCase() === "q") {
        event.preventDefault();
        setLyricsOpen(false);
        setQueueOpen((current) => !current);
      } else if (event.key.toLowerCase() === "m") {
        event.preventDefault();
        toggleMute();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [currentTrackId, togglePlayback, playPrevious, playNext, seekBySeconds, toggleMute, setQueueOpen, setLyricsOpen]);
}
