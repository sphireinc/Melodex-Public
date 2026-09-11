import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { EmptyState, KeyValue, SectionDivider, StatusBadge, StatusLine } from "../Common";
import type { TrackPreview, TrackRecord } from "../../types";
import {
  badgeToneFromConfidence,
  confidenceLabel,
  formatDateTime,
  formatMaybeNumber,
  formatYesNoUnknown,
  initialsForTrack,
  hasAIContext,
  normalizeAIContext,
  relativeOrAbsolute,
} from "../../lib/viewHelpers";

export function TrackDetailPanel({
  track,
  preview,
  previewLoading,
  previewStatus,
  lyricsView,
  setLyricsView,
  onOpenFolder,
  onCopyPath,
  onPlayTrack,
  onTogglePlayback,
  onAddToPlaylist,
  onAddToQueue,
  currentTrackId,
  isPlaying,
}: {
  track: TrackRecord | null;
  preview: TrackPreview | null;
  previewLoading: boolean;
  previewStatus: string;
  lyricsView: "plain" | "timed";
  setLyricsView: (value: "plain" | "timed") => void;
  onOpenFolder: (path: string) => Promise<void>;
  onCopyPath: (path: string) => Promise<void>;
  onPlayTrack: (track: TrackRecord, queueTracks?: TrackRecord[], source?: string) => Promise<void>;
  onTogglePlayback: () => Promise<void>;
  onAddToPlaylist: () => void;
  onAddToQueue: () => void;
  currentTrackId: string;
  isPlaying: boolean;
}) {
  const [showAIContext, setShowAIContext] = useState(false);
  const [showSignals, setShowSignals] = useState(false);

  useEffect(() => {
    setShowAIContext(false);
    setShowSignals(false);
  }, [track?.id]);

  if (!track) {
    return (
      <PanelShell title="Track details" subtitle="Select a track to inspect its files and metadata.">
        <EmptyState title="No track selected" text="Choose a track from the library to reveal its files and lyrics." />
      </PanelShell>
    );
  }

  const p = preview ?? null;
  const metadataConfidence = p?.metadataConfidence || track.metadataConfidence || "low";
  const aiContext = normalizeAIContext(p?.aiContext ?? track.aiContext);

  return (
    <PanelShell
      title={track.title}
      subtitle={`${track.artist} · ${track.album}`}
      extra={
        <div className="panel-inline">
          {track.id === currentTrackId ? (
            <StatusBadge tone="accent">{isPlaying ? "Now playing" : "Ready"}</StatusBadge>
          ) : null}
          <StatusBadge tone={badgeToneFromConfidence(metadataConfidence)}>
            {confidenceLabel(metadataConfidence)}
          </StatusBadge>
        </div>
      }
    >
      <div className="panel-actions">
        <button
          className="button button-primary"
          onClick={() => {
            if (track.id === currentTrackId && isPlaying) {
              void onTogglePlayback();
              return;
            }
            void onPlayTrack(track, [track], "manual");
          }}
        >
          {track.id === currentTrackId && isPlaying ? "Pause" : "Play"}
        </button>
        <button className="button button-secondary" onClick={onAddToQueue}>
          Add to queue
        </button>
        <button className="button button-secondary" onClick={onAddToPlaylist}>
          Add to playlist
        </button>
        <button className="button button-secondary" onClick={() => onOpenFolder(track.storageDir)}>
          Open folder
        </button>
        <button className="button button-secondary" onClick={() => onCopyPath(track.metadataPath)}>
          Copy metadata path
        </button>
        <button className="button button-secondary" onClick={() => onCopyPath(track.audioPath)}>
          Copy audio path
        </button>
      </div>

      <SectionDivider />

      <div className="detail-group">
        <div className="detail-artwork">
          {p?.artworkDataUrl || track.artworkDataUrl ? (
            <img src={p?.artworkDataUrl ?? track.artworkDataUrl ?? ""} alt="" />
          ) : (
            <div className="detail-artwork-placeholder">{initialsForTrack(track)}</div>
          )}
        </div>
      </div>

      <SectionDivider />

      <div className="detail-group">
        <div className="detail-title">File outputs</div>
        {p?.fileStates?.length ? (
          <div className="file-list">
            {p.fileStates.map((file) => (
              <div key={file.label} className="file-row">
                <div>
                  <div className="file-name">{file.label}</div>
                  <div className="file-path">{relativeOrAbsolute(file.path)}</div>
                </div>
                <div className="file-actions">
                  <StatusBadge tone={file.exists ? "success" : "danger"}>
                    {file.exists ? "Exists" : "Missing"}
                  </StatusBadge>
                  <button className="icon-button" onClick={() => onCopyPath(file.path)}>
                    Copy
                  </button>
                  <button className="icon-button" onClick={() => onOpenFolder(file.path)}>
                    Open
                  </button>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="detail-note">{previewLoading ? "Loading files..." : previewStatus}</div>
        )}
      </div>

      <SectionDivider />

      <div className="detail-group">
        <div className="detail-title">Metadata</div>
        <KeyValue label="Title" value={p?.title ?? track.title} />
        <KeyValue label="Artist" value={p?.artist ?? track.artist} />
        <KeyValue label="Album" value={p?.album ?? track.album} />
        <KeyValue label="Track number" value={formatMaybeNumber(p?.trackNumber)} />
        <KeyValue label="Year" value={formatMaybeNumber(p?.year)} />
        <KeyValue label="Genre" value={p?.genre || track.genre || "Unknown"} />
        <KeyValue label="Source URL" value={p?.sourceUrl ?? track.sourceRef} />
        <KeyValue label="Source title" value={p?.sourceTitle ?? track.sourceTitle ?? "Unknown"} />
        <KeyValue label="AI provider" value={p?.aiProvider ?? track.aiProvider ?? "unknown"} />
        <KeyValue label="AI model" value={p?.aiModel ?? track.aiModel ?? "unknown"} />
        <KeyValue label="AI ran" value={formatYesNoUnknown(p?.aiRan ?? track.aiRan)} />
        <KeyValue label="AI status" value={p?.aiStatus ?? track.aiStatus ?? "unknown"} />
        <KeyValue label="AI message" value={p?.aiMessage ?? track.aiMessage ?? "None"} />
        <KeyValue label="Generated at" value={formatDateTime(p?.generatedAt ?? track.generatedAt)} />
        <KeyValue label="Metadata notes" value={p?.metadataNotes ?? track.metadataNotes ?? "None"} />
      </div>

      {hasAIContext(aiContext) ? (
        <>
          <SectionDivider />
          <div className="detail-group">
            <div className="detail-toggle-row">
              <div className="detail-title">AI context</div>
              <button className="text-action" onClick={() => setShowAIContext((value) => !value)}>
                {showAIContext ? "Hide" : "Show"}
              </button>
            </div>
            {showAIContext ? (
              <div className="detail-group">
                <KeyValue label="File name" value={aiContext?.fileName ?? "Unknown"} />
                <KeyValue label="Source ref" value={aiContext?.sourceRef ?? track.sourceRef} />
                <KeyValue label="Source URL" value={aiContext?.sourceUrl ?? p?.sourceUrl ?? track.sourceRef} />
                <KeyValue label="Source kind" value={aiContext?.sourceKind ?? track.sourceKind} />
                <KeyValue label="Library root" value={relativeOrAbsolute(aiContext?.libraryRoot ?? "")} />
                <KeyValue label="Source title" value={aiContext?.sourceTitle ?? "Unknown"} />
                <KeyValue label="Source uploader" value={aiContext?.sourceUploader ?? "Unknown"} />
                <KeyValue label="Source channel" value={aiContext?.sourceChannel ?? "Unknown"} />
                <KeyValue label="Source description" value={aiContext?.sourceDescriptionHint || "—"} />
                <KeyValue label="Duration hint" value={aiContext?.durationHint || "—"} />
                <KeyValue label="Transcript available" value={aiContext?.hasTranscript || "No"} />
                <KeyValue label="Timestamped transcript" value={aiContext?.hasTimestampedTranscript || "No"} />
                <KeyValue label="User context" value={aiContext?.userContext || "—"} />
                <KeyValue label="Artist hint" value={aiContext?.artistHint ?? "—"} />
                <KeyValue label="Album hint" value={aiContext?.albumHint ?? "—"} />
                <KeyValue label="Title hint" value={aiContext?.titleHint ?? "—"} />
                <KeyValue label="Year hint" value={aiContext?.yearHint || "—"} />
                <KeyValue label="Genre hint" value={aiContext?.genreHint || "—"} />
                <KeyValue label="Track number hint" value={aiContext?.trackNumberHint || "—"} />
                <KeyValue label="Resolved title" value={aiContext?.resolvedTitle ?? "—"} />
                <KeyValue label="Resolved artist" value={aiContext?.resolvedArtist ?? "—"} />
                <KeyValue label="Resolved album" value={aiContext?.resolvedAlbum ?? "—"} />
                <KeyValue label="Resolved track number" value={aiContext?.resolvedTrackNumber || "—"} />
                <KeyValue label="Resolved year" value={aiContext?.resolvedYear || "—"} />
                <KeyValue label="Resolved genre" value={aiContext?.resolvedGenre || "—"} />
                <KeyValue label="Resolved confidence" value={aiContext?.resolvedConfidence ?? "—"} />
                <KeyValue label="Target artist dir" value={relativeOrAbsolute(aiContext?.targetArtistDir ?? "")} />
                <KeyValue label="Target album dir" value={relativeOrAbsolute(aiContext?.targetAlbumDir ?? "")} />
                <KeyValue label="Target base name" value={aiContext?.targetBaseName ?? "—"} />
                <KeyValue label="Target audio path" value={relativeOrAbsolute(aiContext?.targetAudioPath ?? "")} />
                <KeyValue label="Target lyrics path" value={relativeOrAbsolute(aiContext?.targetLyricsPath ?? "")} />
                <KeyValue
                  label="Target timed lyrics path"
                  value={relativeOrAbsolute(aiContext?.targetTimedLyricsPath ?? "")}
                />
                <KeyValue
                  label="Target metadata path"
                  value={relativeOrAbsolute(aiContext?.targetMetadataPath ?? "")}
                />
                <KeyValue label="Resolved notes" value={aiContext?.resolvedNotes || "—"} />
                {aiContext?.notes ? <div className="detail-note">{aiContext.notes}</div> : null}
              </div>
            ) : null}
          </div>
        </>
      ) : null}

      <SectionDivider />

      <div className="detail-group">
        <div className="detail-title">Lyrics preview</div>
        {p?.lyricsText?.trim() ? (
          <>
            {p?.timedLyricsText?.trim() ? (
              <div className="segmented">
                <button
                  className={`segment ${lyricsView === "plain" ? "active" : ""}`}
                  onClick={() => setLyricsView("plain")}
                >
                  Plain
                </button>
                <button
                  className={`segment ${lyricsView === "timed" ? "active" : ""}`}
                  onClick={() => setLyricsView("timed")}
                >
                  Timed
                </button>
              </div>
            ) : null}
            <pre className="lyrics-box">
              {lyricsView === "timed"
                ? p?.timedLyricsText || "No timed lyrics available."
                : p?.lyricsText || "No lyrics available."}
            </pre>
          </>
        ) : (
          <EmptyState
            title="No lyrics saved for this track."
            text="Try re-running lyrics detection if you have transcript data available."
          />
        )}
      </div>

      <SectionDivider />

      <div className="detail-group">
        <div className="detail-toggle-row">
          <div className="detail-title">Signals</div>
          <button className="text-action" onClick={() => setShowSignals((value) => !value)}>
            {showSignals ? "Hide" : "Show"}
          </button>
        </div>
        {showSignals ? (
          <div className="detail-group">
            <StatusLine
              label="Lyrics"
              value={track.lyricsIncluded || Boolean(p?.lyricsText?.trim()) ? "Lyrics available" : "No lyrics"}
            />
            <StatusLine
              label="Timed lyrics"
              value={
                track.hasTimedLyrics || Boolean(p?.timedLyricsText?.trim())
                  ? "Timed lyrics available"
                  : "No timed lyrics"
              }
            />
            <StatusLine label="Confidence" value={confidenceLabel(metadataConfidence)} />
            <StatusLine label="Source" value={track.sourceKind} />
            <StatusLine label="Source channel" value={p?.sourceChannel ?? track.sourceChannel ?? "Unknown"} />
          </div>
        ) : null}
      </div>
    </PanelShell>
  );
}

function PanelShell({
  title,
  subtitle,
  extra,
  children,
}: {
  title: string;
  subtitle?: string;
  extra?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="detail-shell">
      <div className="detail-header">
        <div>
          <div className="detail-title-main">{title}</div>
          {subtitle ? <div className="detail-subtitle">{subtitle}</div> : null}
        </div>
        {extra}
      </div>
      {children}
    </div>
  );
}
