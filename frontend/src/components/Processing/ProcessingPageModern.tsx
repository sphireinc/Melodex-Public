import { EmptyState, SectionHeader, VirtualList } from "../Common";
import { JobCard } from "../Queue/JobCard";
import type { AppState, Job } from "../../types";
import {
  currentImportQueueJob,
  importCurrentLabel,
  queueSummaryCounts,
  workerSummaryLabel,
} from "../../lib/viewHelpers";

export function ProcessingPageModern({
  jobs,
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
  state,
}: {
  jobs: Job[];
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
  state: AppState | null;
}) {
  const currentImportJob = currentImportQueueJob(jobs);
  const queueCounts = queueSummaryCounts(jobs);
  const queuedJobs = jobs.filter((job) => job.status === "queued");
  const activeJobs = jobs.filter((job) => job.status === "queued" || job.status === "running");
  const completedJobs = jobs.filter((job) => job.status !== "queued" && job.status !== "running");
  return (
    <section className="content-stack">
      <section className="hero-card import-hero processing-hero-card">
        <div className="processing-hero-layout">
          <div className="processing-hero-copy">
            <h2 className="collection-title collection-title-queue">Queue and Job Progress</h2>
            <p className="hero-description">View Queued Imports, Retries, and Failures</p>
          </div>
          <div className="import-summary-box processing-summary-box">
            <div className="import-current-label">Currently processing</div>
            <div className={`import-current-value ${currentImportJob ? "" : "empty"}`}>
              {importCurrentLabel(currentImportJob)}
            </div>
            <div className="import-current-counts">
              {workerSummaryLabel(state)} · {queueCounts.activelyProcessing} Actively Processing, {queueCounts.done}{" "}
              Done, {queueCounts.todo} To Do
            </div>
          </div>
        </div>
      </section>
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
        <EmptyState title="No active jobs" text="Queued imports will appear here with their current status." />
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
  );
}
