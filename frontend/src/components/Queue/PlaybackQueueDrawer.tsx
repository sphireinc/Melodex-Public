import { EmptyState, SectionDivider } from "../Common";
import type { Playlist, TrackRecord } from "../../types";

type QueueSource = "library" | "playlist" | "search" | "album" | "manual";

export function PlaybackQueueDrawer({
  queueTracks,
  currentTrackId,
  onClose,
  onClearQueue,
  onSaveQueue,
  onPlayTrack,
  onShuffleQueue,
  onRemoveFromQueue,
  onMoveTrack,
  playlists,
  onAddToPlaylist,
}: {
  queueTracks: TrackRecord[];
  currentTrackId: string;
  onClose: () => void;
  onClearQueue: () => void;
  onSaveQueue: () => Promise<void>;
  onPlayTrack: (track: TrackRecord, queueTracks?: TrackRecord[], source?: QueueSource) => Promise<void>;
  onShuffleQueue: () => void;
  onRemoveFromQueue: (trackID: string) => void;
  onMoveTrack: (trackID: string, delta: number) => void;
  playlists: Playlist[];
  onAddToPlaylist: (playlistID: string, trackID: string) => Promise<void>;
}) {
  const current = queueTracks.find((track) => track.id === currentTrackId) ?? null;
  const upcoming = current ? queueTracks.filter((track) => track.id !== currentTrackId) : queueTracks;

  return (
    <div className="drawer-panel queue-drawer">
      <div className="drawer-head">
        <div>
          <div className="drawer-title">Current playback order</div>
          <div className="drawer-subtitle">The order stays with you while you browse and edit the queue.</div>
        </div>
        <button className="icon-button" onClick={onClose} aria-label="Close queue">
          ✕
        </button>
      </div>

      {current ? (
        <div className="queue-now-playing">
          <div className="queue-label">Now playing</div>
          <div className="queue-current-row">
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

      <div className="drawer-actions">
        <button className="button button-secondary" onClick={onShuffleQueue} disabled={queueTracks.length < 2}>
          Shuffle
        </button>
        <button
          className="button button-secondary"
          onClick={() => void onSaveQueue()}
          disabled={queueTracks.length === 0}
        >
          Save as playlist
        </button>
        <button className="button button-secondary" onClick={onClearQueue} disabled={queueTracks.length === 0}>
          Clear queue
        </button>
      </div>

      <SectionDivider />

      <div className="queue-list">
        {upcoming.length === 0 ? (
          <div className="detail-note">No upcoming tracks.</div>
        ) : (
          upcoming.map((track, index) => (
            <div key={track.id} className="queue-row">
              <div className="queue-index">{String(index + 1).padStart(2, "0")}</div>
              <div className="queue-main">
                <div className="queue-track-title">{track.title}</div>
                <div className="queue-track-meta">
                  {track.artist} · {track.album}
                </div>
              </div>
              <div className="queue-actions">
                <button className="icon-button" onClick={() => void onPlayTrack(track, queueTracks, "manual")}>
                  Play now
                </button>
                <button className="icon-button" onClick={() => onMoveTrack(track.id, -1)} disabled={index === 0}>
                  Up
                </button>
                <button
                  className="icon-button"
                  onClick={() => onMoveTrack(track.id, 1)}
                  disabled={index === upcoming.length - 1}
                >
                  Down
                </button>
                <button className="icon-button" onClick={() => onRemoveFromQueue(track.id)}>
                  Remove
                </button>
                <button
                  className="icon-button"
                  onClick={() => playlists[0] && void onAddToPlaylist(playlists[0].id, track.id)}
                  disabled={playlists.length === 0}
                >
                  Add
                </button>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
