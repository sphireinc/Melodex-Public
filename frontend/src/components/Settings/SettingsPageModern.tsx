import { useEffect, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { pickCookiesFile } from "../../lib/backend";
import { toolReadinessDetail, toolReadinessLabel } from "../../lib/toolReadiness";
import { formatDateTime } from "../../lib/viewHelpers";
import { InfoField, SettingsCard } from "../Common";
import type { AppState, SettingsInput } from "../../types";

export function SettingsPageModern({
  settings,
  setSettings,
  apiKeyDraft,
  setApiKeyDraft,
  state,
  saveStatusMessage,
  saveStatusScope,
  statusMessage,
  busy,
  onChooseRoot,
  onSaveSettings,
  onClearKey,
  onResetSettings,
  onTestYTDLP,
  onTestFFmpeg,
  onTestAI,
  onCheckForUpdates,
  onExportDiagnostics,
  onOpenUpdateDownload,
  onOpenDiagnosticsFolder,
}: {
  settings: SettingsInput;
  setSettings: Dispatch<SetStateAction<SettingsInput>>;
  apiKeyDraft: string;
  setApiKeyDraft: (value: string) => void;
  state: AppState | null;
  saveStatusMessage: string;
  saveStatusScope: string;
  statusMessage: string;
  busy: boolean;
  onChooseRoot: () => Promise<void>;
  onSaveSettings: (scope?: string) => Promise<void>;
  onClearKey: () => Promise<void>;
  onResetSettings: () => Promise<void>;
  onTestYTDLP: () => Promise<void>;
  onTestFFmpeg: () => Promise<void>;
  onTestAI: () => Promise<void>;
  onCheckForUpdates: () => Promise<void>;
  onExportDiagnostics: () => Promise<void>;
  onOpenUpdateDownload: () => Promise<void>;
  onOpenDiagnosticsFolder: () => Promise<void>;
}) {
  const [apiKeyEditing, setApiKeyEditing] = useState(false);
  const apiKeyConfigured = Boolean(state?.settings.apiKeyConfigured);
  const apiKeyDisplay = apiKeyConfigured && !apiKeyEditing && !apiKeyDraft.trim() ? "*****************" : apiKeyDraft;
  const ytDlpStatus = settings.ytDlpPath.trim() ? "Path Set" : state?.toolStatus.ytDlp ? "Detected" : "Not Set";
  const ytDlpReadiness = state?.toolReadiness?.ytDlp;
  const ytDlpReady = toolReadinessLabel(ytDlpReadiness, Boolean(state?.toolStatus.ytDlp));
  const ffmpegStatus = settings.ffmpegPath.trim() ? "Path Set" : state?.toolStatus.ffmpeg ? "Detected" : "Not Set";
  const ffmpegReadiness = state?.toolReadiness?.ffmpeg;
  const ffmpegReady = toolReadinessLabel(ffmpegReadiness, Boolean(state?.toolStatus.ffmpeg));
  const ytDlpReadinessDetail = toolReadinessDetail(ytDlpReadiness);
  const ffmpegReadinessDetail = toolReadinessDetail(ffmpegReadiness);
  const cookieState = settings.ytDlpCookiesPath.trim()
    ? "cookie file set"
    : settings.ytDlpCookiesFromBrowser.trim()
      ? "browser set"
      : "not set";
  const saveStatus = saveStatusMessage.trim();
  const testStatus = statusMessage.trim();
  const savedFor = (scope: string) => saveStatus && saveStatusScope === scope;

  useEffect(() => {
    if (!apiKeyDraft.trim()) {
      setApiKeyEditing(false);
    }
  }, [apiKeyDraft]);

  return (
    <section className="content-stack">
      <div className="settings-grid modern-settings-grid">
        <SettingsCard title="Storage">
          <label className="input-group">
            <span>Library root</span>
            <input
              className="text-input"
              value={settings.libraryRoot}
              onChange={(event) => setSettings((current) => ({ ...current, libraryRoot: event.target.value }))}
            />
          </label>
          <div className="action-row">
            <button className="button button-secondary" onClick={onChooseRoot} disabled={busy}>
              Choose folder
            </button>
            <button className="button button-primary" onClick={() => void onSaveSettings("storage")} disabled={busy}>
              Save settings
            </button>
          </div>
          {savedFor("storage") ? <div className="top-status-line settings-status-note">{saveStatus}</div> : null}
        </SettingsCard>

        <SettingsCard title="Downloader">
          <div className="settings-status-row settings-status-row-4">
            <div className="settings-status-card">
              <span>yt-dlp</span>
              <strong>
                {ytDlpStatus} &amp; {ytDlpReady}
              </strong>
            </div>
            <div className="settings-status-card">
              <span>ffmpeg</span>
              <strong>
                {ffmpegStatus} &amp; {ffmpegReady}
              </strong>
            </div>
            <div className="settings-status-card">
              <span>Cookies</span>
              <strong>{cookieState}</strong>
            </div>
            <div className="settings-status-card">
              <span>YouTube auth</span>
              <strong>Use cookies-from-browser or a cookies file when sign-in is required.</strong>
            </div>
          </div>
          {ytDlpReadinessDetail || ffmpegReadinessDetail ? (
            <div className="detail-note">
              <strong>Tool checks</strong>
              <div>yt-dlp: {ytDlpReadinessDetail}</div>
              <div>ffmpeg: {ffmpegReadinessDetail}</div>
            </div>
          ) : null}
          <label className="input-group">
            <span>
              <strong>Video downloads</strong>
              <small>Choose whether videos stay on demand, are fetched during imports, or are disabled.</small>
            </span>
            <select
              className="text-input"
              value={settings.videoDownloadMode || (settings.downloadMusicVideo ? "during-import" : "on-demand")}
              onChange={(event) =>
                setSettings((current) => ({
                  ...current,
                  videoDownloadMode: event.target.value,
                  downloadMusicVideo: event.target.value === "during-import",
                }))
              }
              aria-label="Video download mode"
            >
              <option value="on-demand">On demand (recommended)</option>
              <option value="during-import">During import</option>
              <option value="off">Disabled</option>
            </select>
          </label>
          <label className="toggle-setting">
            <input
              type="checkbox"
              checked={settings.keepOriginalAudio}
              onChange={(event) => setSettings((current) => ({ ...current, keepOriginalAudio: event.target.checked }))}
            />
            <span>
              <strong>Keep original audio copy</strong>
              <small>
                Store the imported source file alongside the decoded MP3 copy when the source is not already MP3.
              </small>
            </span>
          </label>
          <label className="input-group">
            <span>yt-dlp path</span>
            <input
              className="text-input"
              value={settings.ytDlpPath}
              onChange={(event) => setSettings((current) => ({ ...current, ytDlpPath: event.target.value }))}
            />
          </label>
          <label className="input-group">
            <span>ffmpeg path</span>
            <input
              className="text-input"
              value={settings.ffmpegPath}
              onChange={(event) => setSettings((current) => ({ ...current, ffmpegPath: event.target.value }))}
            />
          </label>
          <label className="input-group">
            <span>Cookies from browser</span>
            <input
              className="text-input"
              list="browser-cookie-options"
              value={settings.ytDlpCookiesFromBrowser}
              onChange={(event) =>
                setSettings((current) => ({ ...current, ytDlpCookiesFromBrowser: event.target.value }))
              }
              placeholder="chrome, firefox, brave..."
            />
            <datalist id="browser-cookie-options">
              {["brave", "chrome", "chromium", "edge", "firefox", "opera", "safari", "vivaldi", "whale"].map(
                (option) => (
                  <option key={option} value={option} />
                ),
              )}
            </datalist>
          </label>
          <label className="input-group">
            <span>Cookies file</span>
            <input
              className="text-input"
              value={settings.ytDlpCookiesPath}
              onChange={(event) => setSettings((current) => ({ ...current, ytDlpCookiesPath: event.target.value }))}
              placeholder="Optional path to exported cookies"
            />
          </label>
          <div className="action-row">
            <button
              className="button button-secondary"
              onClick={async () => {
                const path = await pickCookiesFile();
                if (path) {
                  setSettings((current) => ({ ...current, ytDlpCookiesPath: path }));
                }
              }}
              disabled={busy}
            >
              Choose cookies file
            </button>
          </div>
          <div className="settings-subsection">
            <div className="settings-subsection-title">Throughput</div>
            <div className="field-grid-2">
              <label className="input-group">
                <span>Max concurrent downloads</span>
                <input
                  className="text-input"
                  type="number"
                  min={1}
                  value={settings.maxConcurrentDownloads}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      maxConcurrentDownloads: Math.max(1, Number.parseInt(event.target.value || "1", 10) || 1),
                    }))
                  }
                />
              </label>
              <label className="input-group">
                <span>Max concurrent video downloads</span>
                <input
                  className="text-input"
                  type="number"
                  min={1}
                  value={settings.maxConcurrentVideoDownloads}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      maxConcurrentVideoDownloads: Math.max(1, Number.parseInt(event.target.value || "1", 10) || 1),
                    }))
                  }
                />
              </label>
              <label className="input-group">
                <span>Max concurrent enrichment requests</span>
                <input
                  className="text-input"
                  type="number"
                  min={1}
                  value={settings.maxConcurrentEnrichmentRequests}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      maxConcurrentEnrichmentRequests: Math.max(1, Number.parseInt(event.target.value || "1", 10) || 1),
                    }))
                  }
                />
              </label>
              <label className="input-group">
                <span>Max concurrent lyrics requests</span>
                <input
                  className="text-input"
                  type="number"
                  min={1}
                  value={settings.maxConcurrentLyricsRequests}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      maxConcurrentLyricsRequests: Math.max(1, Number.parseInt(event.target.value || "1", 10) || 1),
                    }))
                  }
                />
              </label>
            </div>
            <label className="toggle-setting">
              <input
                type="checkbox"
                checked={settings.throttleOnYtdlpBotErrors}
                onChange={(event) =>
                  setSettings((current) => ({ ...current, throttleOnYtdlpBotErrors: event.target.checked }))
                }
              />
              <span>
                <strong>Throttle on yt-dlp bot/cookie errors</strong>
                <small>Back off automatically when YouTube starts demanding sign-in or cookies.</small>
              </span>
            </label>
          </div>
          <div className="action-row">
            <button className="button button-secondary" onClick={onTestYTDLP} disabled={busy}>
              Test yt-dlp
            </button>
            <button className="button button-secondary" onClick={onTestFFmpeg} disabled={busy}>
              Test ffmpeg
            </button>
            <button className="button button-secondary" onClick={onTestAI} disabled={busy}>
              Test AI
            </button>
            <button className="button button-primary" onClick={() => void onSaveSettings("downloader")} disabled={busy}>
              Save settings
            </button>
          </div>
          {savedFor("downloader") ? (
            <div className="top-status-line settings-status-note">{saveStatus}</div>
          ) : testStatus ? (
            <div className="top-status-line settings-status-note">{testStatus}</div>
          ) : null}
        </SettingsCard>

        <SettingsCard title="AI Provider">
          <div className="field-grid-2">
            <label className="input-group">
              <span>Provider</span>
              <input
                className="text-input"
                value={settings.provider}
                onChange={(event) => setSettings((current) => ({ ...current, provider: event.target.value }))}
              />
            </label>
            <label className="input-group">
              <span>Model</span>
              <input
                className="text-input"
                value={settings.aiModel}
                onChange={(event) => setSettings((current) => ({ ...current, aiModel: event.target.value }))}
              />
            </label>
            <label className="input-group">
              <span>Base URL</span>
              <input
                className="text-input"
                value={settings.aiBaseUrl}
                onChange={(event) => setSettings((current) => ({ ...current, aiBaseUrl: event.target.value }))}
              />
            </label>
            <label className="input-group">
              <span>API key</span>
              <input
                className="text-input"
                type="text"
                value={apiKeyDisplay}
                onFocus={() => {
                  if (apiKeyConfigured && !apiKeyDraft.trim()) {
                    setApiKeyEditing(true);
                    setApiKeyDraft("");
                  }
                }}
                onBlur={() => {
                  if (!apiKeyDraft.trim()) {
                    setApiKeyEditing(false);
                  }
                }}
                onChange={(event) => {
                  setApiKeyEditing(true);
                  setApiKeyDraft(event.target.value);
                }}
                placeholder={apiKeyConfigured ? "*****************" : "Enter your key"}
              />
            </label>
          </div>
          <div className="action-row">
            <button className="button button-secondary" onClick={onClearKey} disabled={busy}>
              Clear key
            </button>
            <button className="button button-primary" onClick={() => void onSaveSettings("ai")} disabled={busy}>
              Save settings
            </button>
          </div>
          {savedFor("ai") ? <div className="top-status-line settings-status-note">{saveStatus}</div> : null}
        </SettingsCard>

        <SettingsCard title="Import defaults">
          <div className="settings-spec-grid">
            <div className="toggle-setting settings-spec-card">
              <div>
                <strong>Collision behavior</strong>
                <small>Deterministic suffix on all four files</small>
              </div>
            </div>
            <div className="toggle-setting settings-spec-card">
              <div>
                <strong>Lyrics file</strong>
                <small>Always written, empty when unavailable</small>
              </div>
            </div>
            <div className="toggle-setting settings-spec-card">
              <div>
                <strong>Timed lyrics file</strong>
                <small>Always written, empty when unavailable</small>
              </div>
            </div>
            <div className="toggle-setting settings-spec-card">
              <div>
                <strong>Metadata source</strong>
                <small>.metadata.json is the source of truth</small>
              </div>
            </div>
          </div>
          <div className="action-row">
            <button
              className="button button-primary"
              onClick={() => void onSaveSettings("import-defaults")}
              disabled={busy}
            >
              Save settings
            </button>
          </div>
          {savedFor("import-defaults") ? (
            <div className="top-status-line settings-status-note">{saveStatus}</div>
          ) : null}
        </SettingsCard>

        <SettingsCard title="Updates">
          <div className="settings-info-grid">
            <InfoField label="Update manifest URL" value={settings.updateManifestUrl || "Not configured"} />
            <InfoField
              label="Manifest status"
              value={
                state?.updateInfo.status || (state?.updateInfo.available ? "Update Available" : "On newest version")
              }
            />
            <InfoField
              label="Current version"
              value={state?.updateInfo.currentVersion ?? state?.buildInfo.appVersion ?? "1.0.0"}
            />
            <InfoField label="Latest version" value={state?.updateInfo.latestVersion || "—"} />
            <InfoField label="Platform" value={state?.updateInfo.platform || "—"} />
            <InfoField label="Mandatory" value={state?.updateInfo.mandatory ? "Yes" : "No"} />
          </div>
          {state?.updateInfo.releaseNotes ? <div className="detail-note">{state.updateInfo.releaseNotes}</div> : null}
          <div className="action-row">
            <button className="button button-secondary" onClick={onCheckForUpdates} disabled={busy}>
              Check for updates
            </button>
            <button
              className="button button-secondary"
              onClick={onOpenUpdateDownload}
              disabled={busy || !state?.updateInfo.downloadUrl}
            >
              Open download
            </button>
            <button className="button button-primary" onClick={() => void onSaveSettings("updates")} disabled={busy}>
              Save settings
            </button>
          </div>
          <div className="top-status-line settings-status-note">
            {state?.updateInfo.status || (state?.updateInfo.available ? "Update Available" : "On newest version")}
          </div>
        </SettingsCard>

        <SettingsCard title="Diagnostics">
          <div className="settings-info-grid">
            <InfoField label="Log file" value={state?.diagnostics.logPath || "—"} />
            <InfoField label="Last export" value={formatDateTime(state?.diagnostics.lastExportedAt)} />
            <InfoField label="Export status" value={state?.diagnostics.bundleStatus || "Idle"} />
            <InfoField label="Last export path" value={state?.diagnostics.lastExportPath || "—"} />
          </div>
          {state?.diagnostics.bundleMessage ? (
            <div className="detail-note">{state.diagnostics.bundleMessage}</div>
          ) : null}
          <div className="action-row">
            <button className="button button-secondary" onClick={onExportDiagnostics} disabled={busy}>
              Export diagnostics
            </button>
            <button
              className="button button-secondary"
              onClick={onOpenDiagnosticsFolder}
              disabled={busy || !state?.diagnostics.lastExportPath}
            >
              Open export
            </button>
          </div>
          <div className="top-status-line settings-status-note">{state?.diagnostics.bundleStatus || "Idle"}</div>
        </SettingsCard>

        <SettingsCard title="Appearance">
          <div className="settings-info-grid">
            <InfoField label="Theme" value="Dark black + silver" />
            <InfoField label="Accent" value="Deep crimson" />
            <InfoField label="Motion" value="Subtle 120-180ms transitions" />
          </div>
        </SettingsCard>

        <SettingsCard title="Build Info and Versions">
          <div className="settings-info-grid">
            <InfoField label="App version" value={state?.buildInfo.appVersion ?? "1.0.0"} />
            <InfoField label="Build number" value={state?.buildInfo.buildNumber ?? "dev"} />
            <InfoField label="Git commit" value={state?.buildInfo.gitCommit ?? "dev"} />
            <InfoField label="Build time" value={state?.buildInfo.buildTime ?? "unknown"} />
            <InfoField label="Channel" value={state?.buildInfo.releaseChannel ?? "dev"} />
            <InfoField label="Go version" value={state?.buildInfo.goVersion ?? "unknown"} />
            <InfoField label="Settings schema" value={String(state?.buildInfo.settingsSchemaVersion ?? 1)} />
            <InfoField label="Catalog schema" value={String(state?.buildInfo.catalogSchemaVersion ?? 1)} />
            <InfoField label="Cache schema" value={String(state?.buildInfo.libraryCacheSchemaVersion ?? 1)} />
            <InfoField label="Playlist schema" value={String(state?.buildInfo.playlistSchemaVersion ?? 1)} />
            <InfoField label="Import history schema" value={String(state?.buildInfo.importHistorySchemaVersion ?? 1)} />
            <InfoField label="Track metadata schema" value={String(state?.buildInfo.trackMetadataSchemaVersion ?? 1)} />
          </div>
          <div className="action-row">
            <button className="button button-secondary" onClick={onResetSettings} disabled={busy}>
              Reset settings
            </button>
          </div>
        </SettingsCard>
      </div>
    </section>
  );
}
