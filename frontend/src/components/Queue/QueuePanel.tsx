import type { ReactNode } from "react";
import { EmptyState, SectionDivider } from "../Common";
import { initialsForTrack } from "../../lib/viewHelpers";
import type { Playlist, TrackRecord } from "../../types";

export function QueuePanel({
  queueTracks,
  currentTrackId,
  onClose,
  onClearQueue,
  onSaveQueue,
  onPlayTrack,
  onAddToPlaylist,
  onShuffleQueue,
  onRemoveFromQueue,
  playlists,
}: {
  queueTracks: TrackRecord[];
  currentTrackId: string;
  onClose: () => void;
  onClearQueue: () => void;
  onSaveQueue: () => Promise<void>;
  onPlayTrack: (track: TrackRecord, queueTracks?: TrackRecord[], source?: string) => Promise<void>;
  onAddToPlaylist: (playlistID: string, trackID: string) => Promise<void>;
  onShuffleQueue: () => void;
  onRemoveFromQueue: (trackID: string) => void;
  playlists: Playlist[];
}) {
  const current = queueTracks.find((track) => track.id === currentTrackId) ?? null;
  const upcoming = current ? queueTracks.filter((track) => track.id !== currentTrackId) : queueTracks;

  return (
    <PanelShell
      title="Queue"
      subtitle="The order stays with you while you browse and edit the queue."
      extra={
        <button className="icon-button" onClick={onClose}>
          Close
        </button>
      }
    >
      {current ? (
        <div className="queue-now-playing">
          <div className="detail-title">Now playing</div>
          <div className="queue-current-row">
            <div className="queue-track-art queue-track-art-large">
              {current.artworkDataUrl?.trim() ? (
                <img src={current.artworkDataUrl} alt="" />
              ) : (
                <span>{initialsForTrack(current)}</span>
              )}
            </div>
            <div className="queue-track-copy queue-current-copy">
              <div className="queue-track-title">{current.title}</div>
              <div className="queue-track-meta">
                {current.artist} · {current.album}
              </div>
            </div>
          </div>
        </div>
      ) : (
        <EmptyState title="Your queue is empty." text="Play an album, playlist, or track to fill it." />
      )}

      <SectionDivider />

      <div className="panel-actions">
        <button className="button button-secondary" onClick={onShuffleQueue} disabled={queueTracks.length < 2}>
          Shuffle
        </button>
        <button className="button button-secondary" onClick={onSaveQueue} disabled={queueTracks.length === 0}>
          Save as playlist
        </button>
        <button className="button button-secondary" onClick={onClearQueue} disabled={queueTracks.length === 0}>
          Clear queue
        </button>
      </div>

      <div className="detail-group">
        <div className="detail-title">Upcoming</div>
        {upcoming.length === 0 ? (
          <div className="detail-note">No upcoming tracks.</div>
        ) : (
          <div className="queue-list">
            {upcoming.map((track, index) => (
              <div key={track.id} className="queue-row">
                <div className="queue-index">{String(index + 1).padStart(2, "0")}</div>
                <div className="queue-main">
                  <div className="queue-track-art">
                    {track.artworkDataUrl?.trim() ? (
                      <img src={track.artworkDataUrl} alt="" />
                    ) : (
                      <span>{initialsForTrack(track)}</span>
                    )}
                  </div>
                  <div className="queue-track-copy">
                    <div className="queue-track-title">{track.title}</div>
                    <div className="queue-track-meta">
                      {track.artist} · {track.album}
                    </div>
                  </div>
                </div>
                <div className="queue-actions">
                  <button className="icon-button" onClick={() => void onPlayTrack(track, queueTracks, "manual")}>
                    Play now
                  </button>
                  <button className="icon-button" onClick={() => onRemoveFromQueue(track.id)}>
                    Remove
                  </button>
                  <button
                    className="icon-button"
                    onClick={() => {
                      const firstPlaylist = playlists[0];
                      if (firstPlaylist) {
                        void onAddToPlaylist(firstPlaylist.id, track.id);
                      }
                    }}
                    disabled={playlists.length === 0}
                  >
                    Add to playlist
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
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
