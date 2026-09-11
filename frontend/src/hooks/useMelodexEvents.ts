import { useEffect } from "react";
import type { Dispatch, MutableRefObject, SetStateAction } from "react";
import { Events } from "@wailsio/runtime";
import type { AppState, JobProgressEvent, PlaybackState } from "../types";
import { patchJobProgress } from "../lib/viewHelpers";
import { shouldUseStatePolling, targetedEventPolicy } from "./eventSyncPolicy";

type UseMelodexEventsArgs = {
  refresh: () => Promise<void>;
  setState: Dispatch<SetStateAction<AppState | null>>;
  setPlayer: Dispatch<SetStateAction<PlaybackState>>;
  syncSettingsFromAppState: (next: AppState) => void;
  hasLoadedRef: MutableRefObject<boolean>;
  normalizeAppState: (input: Partial<AppState> | null | undefined) => AppState;
  setStatus: Dispatch<SetStateAction<string>>;
};

function hasRuntimeEvents() {
  if (typeof window === "undefined") return false;
  const wailsWindow = window as typeof window & { _wails?: { environment?: unknown } };
  return Boolean(wailsWindow._wails?.environment);
}

export function useMelodexEvents({
  refresh,
  setState,
  setPlayer,
  syncSettingsFromAppState,
  hasLoadedRef,
  normalizeAppState,
  setStatus,
}: UseMelodexEventsArgs) {
  useEffect(() => {
    let disposed = false;
    void refresh();

    const applyInitialState = (next: AppState) => {
      if (!hasLoadedRef.current) {
        syncSettingsFromAppState(next);
        hasLoadedRef.current = true;
        return;
      }
      setState(next);
    };

    if (shouldUseStatePolling(hasRuntimeEvents())) {
      const timer = window.setInterval(() => {
        void refresh().catch((error) => {
          setStatus(error instanceof Error ? error.message : String(error));
        });
      }, 1000);
      return () => {
        disposed = true;
        window.clearInterval(timer);
      };
    }

    const offState = Events.On("melodex:state", (event) => {
      if (disposed) return;
      const next = event.data;
      if (!targetedEventPolicy("structural", next).apply) return;
      const normalized = normalizeAppState(next);
      applyInitialState(normalized);
      setPlayer(normalized.playback);
    });

    const offPlayback = Events.On("melodex:playback", (event) => {
      if (disposed || !targetedEventPolicy("playback", event.data).apply) return;
      setPlayer(event.data as PlaybackState);
    });

    const offJobProgress = Events.On("melodex:job-progress", (event) => {
      const next = event.data;
      if (disposed) return;
      setState((current) => {
        const knownJobIds = new Set((current?.jobs ?? []).map((job) => job.id));
        if (!targetedEventPolicy("job-progress", next, knownJobIds).apply) return current;
        return patchJobProgress(current, next as JobProgressEvent);
      });
    });

    return () => {
      disposed = true;
      offState();
      offPlayback();
      offJobProgress();
    };
  }, [refresh, setState, setPlayer, syncSettingsFromAppState, hasLoadedRef, normalizeAppState, setStatus]);
}
