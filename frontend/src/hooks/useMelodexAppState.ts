import { useCallback, useRef, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { getState } from "../lib/backend";
import type { AppState, PlaybackState, SettingsInput } from "../types";

type UseMelodexAppStateArgs = {
  initialSettings: SettingsInput;
  normalizeAppState: (input: Partial<AppState> | null | undefined) => AppState;
  setPlayer: Dispatch<SetStateAction<PlaybackState>>;
};

export function useMelodexAppState({ initialSettings, normalizeAppState, setPlayer }: UseMelodexAppStateArgs) {
  const [state, setState] = useState<AppState | null>(null);
  const [settings, setSettings] = useState<SettingsInput>(initialSettings);
  const [status, setStatus] = useState("Ready");
  const [busy, setBusy] = useState(false);
  const hasLoadedRef = useRef(false);

  const syncSettingsFromAppState = useCallback((next: AppState) => {
    setState(next);
    setSettings({
      libraryRoot: next.settings.libraryRoot,
      aiBaseUrl: next.settings.aiBaseUrl,
      aiModel: next.settings.aiModel,
      provider: next.settings.provider,
      apiKey: "",
      updateManifestUrl: next.settings.updateManifestUrl ?? "",
      ytDlpPath: next.settings.ytDlpPath,
      ffmpegPath: next.settings.ffmpegPath,
      ytDlpCookiesPath: next.settings.ytDlpCookiesPath,
      ytDlpCookiesFromBrowser: next.settings.ytDlpCookiesFromBrowser,
      videoDownloadMode:
        next.settings.videoDownloadMode ?? (next.settings.downloadMusicVideo ? "during-import" : "on-demand"),
      downloadMusicVideo: next.settings.downloadMusicVideo,
      keepOriginalAudio: next.settings.keepOriginalAudio,
      maxConcurrentDownloads: next.settings.maxConcurrentDownloads,
      maxConcurrentVideoDownloads: next.settings.maxConcurrentVideoDownloads,
      maxConcurrentEnrichmentRequests: next.settings.maxConcurrentEnrichmentRequests,
      maxConcurrentLyricsRequests: next.settings.maxConcurrentLyricsRequests,
      throttleOnYtdlpBotErrors: next.settings.throttleOnYtdlpBotErrors,
    });
  }, []);

  const refresh = useCallback(async () => {
    try {
      const next = normalizeAppState(await getState());
      if (!hasLoadedRef.current) {
        syncSettingsFromAppState(next);
        hasLoadedRef.current = true;
      } else {
        setState(next);
      }
      setPlayer(next.playback);
    } catch (error) {
      setStatus(error instanceof Error ? error.message : String(error));
      console.error("Melodex refresh failed:", error);
    }
  }, [normalizeAppState, setPlayer, syncSettingsFromAppState]);

  return {
    state,
    setState,
    settings,
    setSettings,
    status,
    setStatus,
    busy,
    setBusy,
    hasLoadedRef,
    syncSettingsFromAppState,
    refresh,
  };
}
