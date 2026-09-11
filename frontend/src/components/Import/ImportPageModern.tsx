import { useMemo } from "react";
import { EmptyState, InfoField, SectionHeader, StatusBadge, VirtualList } from "../Common";
import { JobCard } from "../Queue/JobCard";
import type { AppState, ImportHistoryEntry, Job } from "../../types";
import { toolReadinessDetail, toolReadinessLabel } from "../../lib/toolReadiness";
import {
  currentImportQueueJob,
  importCurrentLabel,
  queueSummaryCounts,
  safeSplitImportUrls,
  workerSummaryLabel,
} from "../../lib/viewHelpers";

export function ImportPageModern({
  url,
  setUrl,
  busy,
  onQueueURL,
  onChooseFiles,
  onChooseFolder,
  onChooseRoot,
  onRescan,
  onOpenSettings,
  onToggleWindowMaximize,
  state,
  jobs,
  importHistory,
  selectedJobId,
  onSelectJob,
  onStartJob,
  onStopJob,
  onDeleteJob,
  onRetryFailedPlaylist,
  onPauseAllJobs,
  onDeleteAllJobs,
  onDeleteDoneJobs,
  onDeleteQueuedJobs,
  onClearImportHistory,
  advancedOpen,
  setAdvancedOpen,
}: {
  url: string;
  setUrl: (value: string) => void;
  busy: boolean;
  onQueueURL: () => Promise<void>;
  onChooseFiles: () => Promise<void>;
  onChooseFolder: () => Promise<void>;
  onChooseRoot: () => Promise<void>;
  onRescan: () => Promise<void>;
  onOpenSettings: () => void;
  onToggleWindowMaximize: () => Promise<void>;
  state: AppState | null;
  jobs: Job[];
  importHistory: ImportHistoryEntry[];
  selectedJobId: string;
  onSelectJob: (id: string) => void;
  onStartJob: (job: Job) => Promise<void>;
  onStopJob: (job: Job) => Promise<void>;
  onDeleteJob: (job: Job) => Promise<void>;
  onRetryFailedPlaylist: (job: Job) => Promise<void>;
  onPauseAllJobs: () => Promise<void>;
  onDeleteAllJobs: () => Promise<void>;
  onDeleteDoneJobs: () => Promise<void>;
  onDeleteQueuedJobs: () => Promise<void>;
  onClearImportHistory: () => Promise<void>;
  advancedOpen: boolean;
  setAdvancedOpen: (value: boolean) => void;
}) {
  const currentImportJob = currentImportQueueJob(jobs);
  const queueCounts = queueSummaryCounts(jobs);
  const outputFormat =
    state?.settings.videoDownloadMode === "during-import" || state?.settings.downloadMusicVideo
      ? "MP3 + MP4 + TXT + LRC + metadata.json"
      : "MP3 + TXT + LRC + metadata.json";
  const doneLines = useMemo(() => importHistory.map((entry) => entry.url).filter(Boolean), [importHistory]);
  const doneText = useMemo(() => doneLines.join("\n"), [doneLines]);
  const queuedJobs = jobs.filter((job) => job.status === "queued");
  const activeJobs = jobs.filter((job) => job.status === "queued" || job.status === "running");
  const completedJobs = jobs.filter((job) => job.status !== "queued" && job.status !== "running");
  const ytDlpReadiness = state?.toolReadiness?.ytDlp;
  const ffmpegReadiness = state?.toolReadiness?.ffmpeg;
  const toolReadinessWarning = [
    {
      name: "yt-dlp",
      label: toolReadinessLabel(ytDlpReadiness, Boolean(state?.toolStatus.ytDlp)),
      detail: toolReadinessDetail(ytDlpReadiness),
    },
    {
      name: "ffmpeg",
      label: toolReadinessLabel(ffmpegReadiness, Boolean(state?.toolStatus.ffmpeg)),
      detail: toolReadinessDetail(ffmpegReadiness),
    },
  ].filter(({ label }) => label !== "Ready");
  return (
    <section className="content-stack">
      <section className="hero-card import-hero import-diff-shell">
        <div className="import-diff-top">
          <div className="import-diff-label">Entry</div>
          <div className="import-current">
            <div className="import-current-label">Currently processing</div>
            <div className={`import-current-value ${currentImportJob ? "" : "empty"}`}>
              {importCurrentLabel(currentImportJob)}
            </div>
            <div className="import-current-counts">
              {workerSummaryLabel(state)} · {queueCounts.activelyProcessing} Actively Processing, {queueCounts.done}{" "}
              Done, {queueCounts.todo} To Do
            </div>
          </div>
          <div className="import-diff-label import-diff-label-right">Done</div>
        </div>

        <div className="import-diff-grid">
          <section className="import-diff-pane">
            <div className="import-pane-actions">
              <button className="button button-secondary" onClick={() => setUrl("")} disabled={url.length === 0}>
                Clear
              </button>
              <div className="import-pane-actions-right">
                <button className="button button-secondary" onClick={onChooseFiles} disabled={busy}>
                  Load File
                </button>
                <button className="button button-secondary" onClick={onChooseFolder} disabled={busy}>
                  Load Folder
                </button>
                <button
                  className="button button-primary"
                  onClick={onQueueURL}
                  disabled={busy || safeSplitImportUrls(url).length === 0}
                >
                  Import
                </button>
              </div>
            </div>
            <div className="import-summary-box import-summary-box-inline">
              <div className="import-current-label">Currently processing</div>
              <div className={`import-current-value ${currentImportJob ? "" : "empty"}`}>
                {importCurrentLabel(currentImportJob)}
              </div>
              <div className="import-current-counts">
                {workerSummaryLabel(state)} · {queueCounts.activelyProcessing} Actively Processing, {queueCounts.done}{" "}
                Done, {queueCounts.todo} To Do
              </div>
            </div>
            {!state?.settings.apiKeyConfigured ? (
              <div className="import-ai-callout missing">
                <div className="import-ai-callout-top">
                  <StatusBadge tone="warning">AI recommended</StatusBadge>
                  <button className="text-action" onClick={onOpenSettings} disabled={busy}>
                    Open settings
                  </button>
                </div>
                <div className="detail-note">
                  AI is strongly recommended because it gives Melodex richer metadata, trivia, and song meaning. You can
                  still import music without it.
                </div>
              </div>
            ) : null}
            {toolReadinessWarning.length > 0 ? (
              <div className="import-ai-callout missing">
                <div className="import-ai-callout-top">
                  <StatusBadge tone="warning">Tool check</StatusBadge>
                  <button className="text-action" onClick={onOpenSettings} disabled={busy}>
                    Open settings
                  </button>
                </div>
                <div className="detail-note">
                  {toolReadinessWarning.map(({ name, label, detail }) => (
                    <div key={name}>
                      <strong>
                        {name}: {label}
                      </strong>
                      {detail ? ` — ${detail}` : null}
                    </div>
                  ))}
                </div>
              </div>
            ) : null}

            <textarea
              className="text-area import-textarea"
              placeholder="Paste one URL per line"
              value={url}
              onChange={(event) => setUrl(event.target.value)}
              spellCheck={false}
            />
          </section>

          <section className="import-diff-pane">
            <div className="import-pane-footer">
              <button
                className="button button-secondary"
                onClick={onClearImportHistory}
                disabled={doneLines.length === 0}
              >
                Clear
              </button>
            </div>
            <textarea
              className="text-area import-textarea import-textarea-done"
              value={doneText}
              placeholder="Completed URLs appear here after imports finish."
              readOnly
              disabled
            />
          </section>
        </div>

        <details
          className="advanced"
          open={advancedOpen}
          onToggle={(event) => setAdvancedOpen(event.currentTarget.open)}
        >
          <summary>Advanced</summary>
          <div className="advanced-grid">
            <InfoField label="Save location" value={state?.rootInfo.libraryRoot ?? "~/Music/Melodex Music"} />
            <InfoField label="Output format" value={outputFormat} />
            <InfoField label="Storage structure" value="Artist / Album / Song" />
            <InfoField label="Collision behavior" value="Deterministic suffix on all files" />
          </div>
          <div className="advanced-actions">
            <button className="button button-secondary" onClick={onChooseRoot} disabled={busy}>
              Choose folder
            </button>
            <button className="button button-secondary" onClick={onToggleWindowMaximize} disabled={busy}>
              {state?.windowFullscreen ? "Exit Fullscreen" : "Fullscreen"}
            </button>
            <button className="button button-secondary" onClick={onRescan} disabled={busy}>
              Rescan library
            </button>
          </div>
        </details>
      </section>

      <section className="section-block">
        <div className="queue-header-row">
          <SectionHeader title="Queue" />
          <div className="queue-header-actions">
            <button
              className="button button-secondary"
              onClick={() => void onPauseAllJobs()}
              disabled={activeJobs.length === 0}
            >
              Pause All
            </button>
            <button
              className="button button-secondary"
              onClick={() => void onDeleteAllJobs()}
              disabled={jobs.length === 0}
            >
              Delete All
            </button>
            <button
              className="button button-secondary"
              onClick={() => void onDeleteDoneJobs()}
              disabled={completedJobs.length === 0}
            >
              Delete Done
            </button>
            <button
              className="button button-secondary"
              onClick={() => void onDeleteQueuedJobs()}
              disabled={queuedJobs.length === 0}
            >
              Delete Not Done
            </button>
          </div>
        </div>
        {jobs.length === 0 ? (
          <EmptyState
            title="No jobs yet"
            text="Imports you start here will appear in the queue with progress and status."
          />
        ) : (
          <VirtualList
            className="virtual-job-stack"
            style={{ maxHeight: "min(72vh, 940px)" }}
            items={jobs}
            itemHeight={390}
            getKey={(job) => job.id}
            renderItem={(job, index) => (
              <JobCard
                job={job}
                position={index + 1}
                active={job.id === selectedJobId}
                onSelect={() => onSelectJob(job.id)}
                onStart={() => void onStartJob(job)}
                onStop={() => void onStopJob(job)}
                onDelete={() => void onDeleteJob(job)}
                onRetryFailedPlaylist={() => void onRetryFailedPlaylist(job)}
              />
            )}
          />
        )}
      </section>
    </section>
  );
}
