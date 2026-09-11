import type { ReactNode } from "react";
import { EmptyState, SectionDivider, StatusBadge } from "../Common";
import {
  badgeToneFromConfidence,
  confidenceLabel,
  formatDuration,
  initialsForTrack,
  playlistEntriesFromRecord,
  trackDuration,
} from "../../lib/viewHelpers";
import type { Playlist, TrackRecord } from "../../types";

export function PlaylistDetailPanel({
  playlist,
  tracks,
  onPlayPlaylist,
  onShufflePlaylist,
  onPlayTrack,
  onRemoveTrack,
  onMoveTrack,
  onRevealTrack,
  onOpenTrackDetail,
}: {
  playlist: Playlist;
  tracks: TrackRecord[];
  onPlayPlaylist: () => Promise<void>;
  onShufflePlaylist: () => Promise<void>;
  onPlayTrack: (track: TrackRecord, queueTracks?: TrackRecord[], source?: string) => Promise<void>;
  onRemoveTrack: (trackID: string) => Promise<void>;
  onMoveTrack: (trackID: string, delta: number) => Promise<void>;
  onRevealTrack: (path: string) => Promise<void>;
  onOpenTrackDetail: (track: TrackRecord) => void;
}) {
  const entries = playlistEntriesFromRecord(playlist, tracks);
  const availableTracks = entries.map((entry) => entry.track).filter(Boolean) as TrackRecord[];
  const missingCount = entries.length - availableTracks.length;
  const totalDuration = availableTracks.reduce((sum, track) => sum + trackDuration(track), 0);

  return (
    <PanelShell
      title={playlist.name}
      subtitle={`${entries.length} tracks · ${formatDuration(totalDuration)}`}
      extra={<StatusBadge tone={entries.length > 0 ? "accent" : "neutral"}>{entries.length} saved</StatusBadge>}
    >
      <div className="panel-actions">
        <button className="button button-primary" onClick={onPlayPlaylist} disabled={availableTracks.length === 0}>
          Play
        </button>
        <button className="button button-secondary" onClick={onShufflePlaylist} disabled={availableTracks.length === 0}>
          Shuffle
        </button>
      </div>

      {playlist.description ? <div className="detail-note">{playlist.description}</div> : null}

      <SectionDivider />

      <div className="detail-group">
        <div className="detail-title">Playlist tracks</div>
        {entries.length === 0 ? (
          <EmptyState title="Nothing in this playlist yet." text="Add tracks from your library to start building it." />
        ) : (
          <div className="playlist-track-list">
            {entries.map((entry, index) => (
              <div
                key={`${playlist.id}:${entry.trackID}`}
                className={`playlist-track-row ${entry.track ? "" : "missing"}`}
              >
                <div className="playlist-track-index">{String(index + 1).padStart(2, "0")}</div>
                <div className="playlist-track-main">
                  {entry.track ? (
                    <div className="playlist-track-art">
                      {entry.track.artworkDataUrl?.trim() ? (
                        <img src={entry.track.artworkDataUrl} alt="" />
                      ) : (
                        <span>{initialsForTrack(entry.track)}</span>
                      )}
                    </div>
                  ) : null}
                  <div className="playlist-track-copy">
                    <div className="playlist-track-title">{entry.track?.title ?? "Missing track"}</div>
                    <div className="playlist-track-meta">
                      {entry.track ? `${entry.track.artist} · ${entry.track.album}` : entry.trackID}
                    </div>
                  </div>
                </div>
                <div className="playlist-track-badges">
                  {entry.track ? (
                    <>
                      <StatusBadge tone={badgeToneFromConfidence(entry.track.metadataConfidence)}>
                        {confidenceLabel(entry.track.metadataConfidence || "unknown")}
                      </StatusBadge>
                      <StatusBadge tone={entry.track.hasTimedLyrics ? "success" : "neutral"}>
                        {entry.track.hasTimedLyrics ? "Timed" : "Plain"}
                      </StatusBadge>
                    </>
                  ) : (
                    <StatusBadge tone="danger">Missing file</StatusBadge>
                  )}
                </div>
                <div className="playlist-track-actions">
                  {entry.track ? (
                    <>
                      <button
                        className="icon-button"
                        onClick={() => void onPlayTrack(entry.track!, availableTracks, "playlist")}
                      >
                        Play now
                      </button>
                      <button className="icon-button" onClick={() => onOpenTrackDetail(entry.track!)}>
                        Details
                      </button>
                      <button className="icon-button" onClick={() => void onRevealTrack(entry.track!.storageDir)}>
                        Reveal
                      </button>
                    </>
                  ) : null}
                  <button className="icon-button" onClick={() => void onRemoveTrack(entry.trackID)}>
                    Remove
                  </button>
                  <button
                    className="icon-button"
                    onClick={() => void onMoveTrack(entry.trackID, -1)}
                    disabled={index === 0}
                  >
                    Up
                  </button>
                  <button
                    className="icon-button"
                    onClick={() => void onMoveTrack(entry.trackID, 1)}
                    disabled={index === entries.length - 1}
                  >
                    Down
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {missingCount > 0 ? (
        <>
          <SectionDivider />
          <div className="detail-note">{missingCount} entries do not have a currently indexed track file.</div>
        </>
      ) : null}
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
