import type { CSSProperties, KeyboardEvent } from "react";
import { StatusBadge } from "../Common";
import {
  capitalize,
  formatDateTime,
  isPlaylistRootJob,
  jobPlaylistSummary,
  jobResultLabel,
  jobStageKey,
  jobStageProgressItems,
  jobStageStatusItems,
  jobStageTone,
  jobTone,
  splitJobDetail,
} from "../../lib/viewHelpers";
import type { Job } from "../../types";

export function JobCard({
  job,
  position,
  active,
  onSelect,
  onStart,
  onStop,
  onDelete,
  onRetryFailedPlaylist,
}: {
  job: Job;
  position?: number;
  active: boolean;
  onSelect: () => void;
  onStart: () => void;
  onStop: () => void;
  onDelete: () => void;
  onRetryFailedPlaylist: () => void;
}) {
  const [primaryDetail, secondaryDetail] = splitJobDetail(job.detail);
  const canStart = jobCanStart(job.status);
  const canStop = jobCanStop(job.status);
  const resultLabel = jobResultLabel(job);
  const playlistSummary = jobPlaylistSummary(job);
  const playlistRoot = isPlaylistRootJob(job);
  const playlistFailed = (job.playlistFailedItems ?? 0) > 0;
  const handleCardKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.target !== event.currentTarget || (event.key !== "Enter" && event.key !== " ")) {
      return;
    }
    event.preventDefault();
    onSelect();
  };

  return (
    <div
      className={`job-card ${active ? "active" : ""}`}
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={handleCardKeyDown}
      aria-label={`${capitalize(job.kind)} job: ${job.input}`}
      aria-pressed={active}
    >
      <div className="job-top">
        <div>
          <div className="job-title">{position ? `Job #${position}` : capitalize(job.kind)}</div>
          <div className="job-subtitle">
            {job.input}
            {resultLabel ? <span className="job-result-label">{resultLabel}</span> : null}
          </div>
          {playlistSummary ? <div className="job-note job-playlist-note">{playlistSummary}</div> : null}
        </div>
        <StatusBadge tone={jobTone(job.status)}>{capitalize(job.status)}</StatusBadge>
      </div>
      {!playlistRoot && secondaryDetail ? <div className="job-note">{secondaryDetail}</div> : null}
      <div className="job-detail">{primaryDetail}</div>
      <JobStageChipRow job={job} />
      <div className="job-footer">
        <span>{formatDateTime(job.createdAt)}</span>
        {playlistRoot ? (
          <div className="playlist-job-metrics">
            <div className="playlist-job-metric">
              <span>Total</span>
              <strong>{job.playlistTotalItems ?? 0}</strong>
            </div>
            <div className="playlist-job-metric">
              <span>Processed</span>
              <strong>{job.playlistProcessedItems ?? 0}</strong>
            </div>
            <div className="playlist-job-metric">
              <span>Failed</span>
              <strong>{job.playlistFailedItems ?? 0}</strong>
            </div>
          </div>
        ) : (
          <div className="job-stage-progress">
            {jobStageProgressItems(job).map((item) => (
              <JobStageProgress key={item.label} label={item.label} value={item.value} />
            ))}
          </div>
        )}
        <div className="job-actions">
          {playlistRoot && playlistFailed ? (
            <button
              className="text-action"
              onClick={(event) => {
                event.stopPropagation();
                onRetryFailedPlaylist();
              }}
            >
              Retry failed
            </button>
          ) : null}
          {canStart ? (
            <button
              className="text-action"
              onClick={(event) => {
                event.stopPropagation();
                onStart();
              }}
            >
              {job.parentJobId && job.status === "failed" ? "Retry" : "Start"}
            </button>
          ) : null}
          {canStop ? (
            <button
              className="text-action"
              onClick={(event) => {
                event.stopPropagation();
                onStop();
              }}
            >
              Stop
            </button>
          ) : null}
          <button
            className="text-action job-action-danger"
            onClick={(event) => {
              event.stopPropagation();
              onDelete();
            }}
          >
            Delete
          </button>
        </div>
      </div>
    </div>
  );
}

function JobStageProgress({ label, value }: { label: string; value: number }) {
  const safeValue = Math.max(0, Math.min(100, value));
  const tone = jobStageTone(safeValue);
  const stage = jobStageKey(label);
  const style = {
    "--stage-color": stage.color,
    "--stage-soft": stage.soft,
  } as CSSProperties;
  return (
    <div className={`job-stage-progress-item tone-${tone} stage-${stage.key}`} style={style}>
      <div className="job-stage-progress-head">
        <span>{label}</span>
        <span>{safeValue}/100</span>
      </div>
      <ProgressBar value={safeValue} />
    </div>
  );
}

function JobStageChipRow({ job }: { job: Job }) {
  const items = jobStageStatusItems(job);
  return (
    <div className="job-stage-chip-row">
      {items.map((item) => (
        <div key={item.label} className={`job-stage-chip tone-${item.tone}`}>
          <span>{item.label}</span>
          <strong>{item.value}</strong>
        </div>
      ))}
    </div>
  );
}

function ProgressBar({ value }: { value: number }) {
  const safeValue = Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0));
  return (
    <div className="progress" aria-hidden="true">
      <div className="progress-fill" style={{ width: `${safeValue}%` }} />
    </div>
  );
}

function jobCanStart(status: string) {
  return status === "stopped" || status === "failed";
}

function jobCanStop(status: string) {
  return status === "queued" || status === "running";
}
