import { useEffect, useMemo, useState } from "react";
import type { CSSProperties } from "react";
import { closestCenter, DndContext, KeyboardSensor, PointerSensor, useSensor, useSensors } from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  defaultAnimateLayoutChanges,
  rectSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { EmptyState, InfoField, SectionDivider, StatusBadge } from "../Common";
import type { Playlist, QueueSource, TrackRecord } from "../../types";
import {
  badgeToneFromTrack,
  formatDateTime,
  formatDuration,
  initialsForTrack,
  playlistEntriesFromRecord,
  playlistTrackCountFromSet,
  shuffleList,
  trackDuration,
  metadataLabel,
} from "../../lib/viewHelpers";

type PlaylistTrackEntry = {
  trackID: string;
  track: TrackRecord | null;
};

export function PlaylistsPageModern({
  playlists,
  tracks,
  selectedPlaylistId,
  setSelectedPlaylistId,
  onOpenCreatePlaylist,
  onRequestRenamePlaylist,
  onDeletePlaylist,
  onPlayTrack,
  onAddToPlaylist: _onAddToPlaylist,
  onMoveTrack,
  onRemoveTrack,
  onOpenTrackDetail,
}: {
  playlists: Playlist[];
  tracks: TrackRecord[];
  selectedPlaylistId: string;
  setSelectedPlaylistId: (id: string) => void;
  onOpenCreatePlaylist: () => void;
  onRequestRenamePlaylist: (playlist: Playlist) => void;
  onDeletePlaylist: (playlistID: string) => Promise<void>;
  onPlayTrack: (track: TrackRecord, queueTracks?: TrackRecord[], source?: QueueSource) => Promise<void>;
  onAddToPlaylist: (playlistID: string, trackID: string) => Promise<void>;
  onMoveTrack: (playlistID: string, trackID: string, delta: number) => Promise<void>;
  onRemoveTrack: (playlistID: string, trackID: string) => Promise<void>;
  onOpenTrackDetail: (track: TrackRecord) => void;
}) {
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );
  const trackIDs = useMemo(() => new Set(tracks.map((track) => track.id)), [tracks]);
  const playlistTrackCounts = useMemo(
    () => new Map(playlists.map((playlist) => [playlist.id, playlistTrackCountFromSet(playlist, trackIDs)])),
    [playlists, trackIDs],
  );
  const selectedPlaylist = useMemo(
    () => playlists.find((playlist) => playlist.id === selectedPlaylistId) ?? null,
    [playlists, selectedPlaylistId],
  );
  const selectedTrackIds = selectedPlaylist?.trackIds ?? [];
  const selectedTrackIdsKey = selectedTrackIds.join("\u0001");
  const [orderedTrackIds, setOrderedTrackIds] = useState<string[]>(selectedPlaylist?.trackIds ?? []);
  useEffect(() => {
    setOrderedTrackIds(selectedTrackIdsKey ? selectedTrackIdsKey.split("\u0001") : []);
  }, [selectedPlaylist?.id, selectedTrackIdsKey]);
  const orderedPlaylist = useMemo(
    () => (selectedPlaylist ? { ...selectedPlaylist, trackIds: orderedTrackIds } : null),
    [orderedTrackIds, selectedPlaylist],
  );
  const selectedEntries = useMemo(
    () => (orderedPlaylist ? playlistEntriesFromRecord(orderedPlaylist, tracks) : []),
    [orderedPlaylist, tracks],
  );
  const selectedTracks = useMemo(
    () => selectedEntries.map((entry) => entry.track).filter(Boolean) as TrackRecord[],
    [selectedEntries],
  );
  const selectedDuration = useMemo(
    () => selectedTracks.reduce((sum, track) => sum + trackDuration(track), 0),
    [selectedTracks],
  );

  async function moveTrackToIndex(trackID: string, targetIndex: number) {
    if (!selectedPlaylist) return;
    const sourceIndex = orderedTrackIds.findIndex((id) => id === trackID);
    if (sourceIndex < 0 || sourceIndex === targetIndex) return;
    const nextOrder = arrayMove(orderedTrackIds, sourceIndex, targetIndex);
    setOrderedTrackIds(nextOrder);
    await onMoveTrack(selectedPlaylist.id, trackID, targetIndex - sourceIndex);
  }

  async function handleDragEnd(activeID: string, overID: string | null) {
    if (!selectedPlaylist || !overID) return;
    const sourceIndex = orderedTrackIds.findIndex((id) => id === activeID);
    const targetIndex = orderedTrackIds.findIndex((id) => id === overID);
    if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return;
    await moveTrackToIndex(activeID, targetIndex);
  }

  return (
    <section className="content-stack playlists-modern">
      <section className="hero-card import-hero">
        <div className="playlist-hero-top playlist-hero-top-single">
          <div>
            <h2 className="collection-title">Collections Built From Your Local Library.</h2>
            <p className="hero-description">
              Create playlists, reorder tracks, and preserve missing entries when files move.
            </p>
          </div>
          <div className="playlist-hero-actions">
            <label className="input-group playlist-select-group">
              <span>Playlist</span>
              <select
                className="text-input playlist-select"
                value={selectedPlaylistId}
                onChange={(event) => setSelectedPlaylistId(event.target.value)}
              >
                {playlists.map((playlist) => {
                  const count = playlistTrackCounts.get(playlist.id) ?? 0;
                  return (
                    <option key={playlist.id} value={playlist.id}>
                      {playlist.name} ({count})
                    </option>
                  );
                })}
              </select>
            </label>
            <button className="button button-primary" onClick={onOpenCreatePlaylist}>
              New playlist
            </button>
          </div>
        </div>
      </section>

      <section className="playlist-info-panel modern-playlist-detail playlist-info-panel-full">
        {selectedPlaylist ? (
          <div className="playlist-summary-card">
            <div className="playlist-summary-top">
              <div className="playlist-summary-art">
                {selectedTracks[0]?.artworkDataUrl?.trim() ? (
                  <img src={selectedTracks[0].artworkDataUrl} alt="" />
                ) : selectedTracks[0] ? (
                  initialsForTrack(selectedTracks[0])
                ) : (
                  "M"
                )}
              </div>
              <div>
                <div className="playlist-title-xl">{selectedPlaylist.name}</div>
                <div className="playlist-description">{selectedPlaylist.description || "No description"}</div>
              </div>
              <StatusBadge tone="accent">{selectedPlaylist.trackIds.length} saved</StatusBadge>
            </div>
            <div className="playlist-summary-grid">
              <InfoField label="Tracks" value={`${selectedTracks.length}/${selectedPlaylist.trackIds.length}`} />
              <InfoField label="Duration" value={formatDuration(selectedDuration)} />
              <InfoField label="Updated" value={formatDateTime(selectedPlaylist.updatedAt)} />
              <InfoField label="Created" value={formatDateTime(selectedPlaylist.createdAt)} />
            </div>
            <div className="action-row">
              <button
                className="button button-primary"
                onClick={() => selectedTracks[0] && void onPlayTrack(selectedTracks[0], selectedTracks, "playlist")}
                disabled={selectedTracks.length === 0}
              >
                Play
              </button>
              <button
                className="button button-secondary"
                onClick={() =>
                  selectedTracks[0] && void onPlayTrack(selectedTracks[0], shuffleList(selectedTracks), "playlist")
                }
                disabled={selectedTracks.length === 0}
              >
                Shuffle
              </button>
              <button className="button button-secondary" onClick={() => onRequestRenamePlaylist(selectedPlaylist)}>
                Edit
              </button>
              <button
                className="button button-secondary"
                onClick={() => {
                  const confirmed = window.confirm(
                    `Delete playlist "${selectedPlaylist.name}"? This cannot be undone.`,
                  );
                  if (confirmed) {
                    void onDeletePlaylist(selectedPlaylist.id);
                  }
                }}
              >
                Delete
              </button>
            </div>

            <SectionDivider />

            {selectedEntries.length === 0 ? (
              <EmptyState
                title="Nothing in this playlist yet."
                text="Add tracks from your library to start building it."
              />
            ) : (
              <DndContext
                sensors={sensors}
                collisionDetection={closestCenter}
                onDragEnd={(event) => {
                  void handleDragEnd(String(event.active.id), event.over?.id ? String(event.over.id) : null);
                }}
              >
                <SortableContext items={selectedEntries.map((entry) => entry.trackID)} strategy={rectSortingStrategy}>
                  <div className="playlist-track-list modern-playlist-track-list">
                    {selectedEntries.map((entry, index) => (
                      <SortablePlaylistTrackRow
                        key={`${selectedPlaylist.id}:${entry.trackID}`}
                        entry={entry}
                        index={index}
                        selectedPlaylist={selectedPlaylist}
                        selectedTracks={selectedTracks}
                        onPlayTrack={onPlayTrack}
                        onRemoveTrack={onRemoveTrack}
                        onOpenTrackDetail={onOpenTrackDetail}
                      />
                    ))}
                  </div>
                </SortableContext>
              </DndContext>
            )}
          </div>
        ) : (
          <EmptyState
            title="Select a playlist"
            text="Choose a playlist to inspect its tracks, duration, and playback options."
          />
        )}
      </section>
    </section>
  );
}

export function SortablePlaylistTrackRow({
  entry,
  index,
  selectedPlaylist,
  selectedTracks,
  onPlayTrack,
  onRemoveTrack,
  onOpenTrackDetail,
}: {
  entry: PlaylistTrackEntry;
  index: number;
  selectedPlaylist: Playlist;
  selectedTracks: TrackRecord[];
  onPlayTrack: (track: TrackRecord, queueTracks?: TrackRecord[], source?: QueueSource) => Promise<void>;
  onRemoveTrack: (playlistID: string, trackID: string) => Promise<void>;
  onOpenTrackDetail: (track: TrackRecord) => void;
}) {
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({
    id: entry.trackID,
    animateLayoutChanges: defaultAnimateLayoutChanges,
    transition: {
      duration: 280,
      easing: "cubic-bezier(0.2, 0, 0, 1)",
    },
  });
  const style: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
  };
  const track = entry.track;

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`playlist-track-row ${track ? "" : "missing"} ${isDragging ? "dragging" : ""}`}
      onClick={() => {
        if (track) {
          onOpenTrackDetail(track);
        }
      }}
      onDoubleClick={() => {
        if (track) {
          void onPlayTrack(track, selectedTracks, "playlist");
        }
      }}
      role="button"
      tabIndex={0}
      onKeyDown={(event) => {
        if ((event.key === "Enter" || event.key === " ") && track) {
          event.preventDefault();
          void onPlayTrack(track, selectedTracks, "playlist");
        }
      }}
    >
      <div className="playlist-track-index">{String(index + 1).padStart(2, "0")}</div>
      <div className="playlist-track-main">
        <div className="playlist-track-art">
          {track?.artworkDataUrl?.trim() ? (
            <img src={track.artworkDataUrl} alt="" />
          ) : (
            <span>{track ? initialsForTrack(track) : "♪"}</span>
          )}
        </div>
        <div className="playlist-track-copy">
          <div className="playlist-track-title">{track?.title ?? "Missing track"}</div>
          <div className="playlist-track-meta">{track ? `${track.artist} · ${track.album}` : entry.trackID}</div>
        </div>
      </div>
      <div className="playlist-track-badges">
        {track ? (
          <>
            <StatusBadge tone={badgeToneFromTrack(track)}>{metadataLabel(track)}</StatusBadge>
            <StatusBadge tone={track.hasTimedLyrics ? "success" : "neutral"}>
              {track.hasTimedLyrics ? "Timed" : "Plain"}
            </StatusBadge>
          </>
        ) : (
          <StatusBadge tone="danger">Missing file</StatusBadge>
        )}
      </div>
      <div className="playlist-track-actions">
        {track ? (
          <button
            className="icon-button"
            onClick={(event) => {
              event.stopPropagation();
              void onPlayTrack(track, selectedTracks, "playlist");
            }}
          >
            Play
          </button>
        ) : null}
        <button
          className="icon-button"
          onClick={(event) => {
            event.stopPropagation();
            void onRemoveTrack(selectedPlaylist.id, entry.trackID);
          }}
        >
          Remove
        </button>
        <button
          ref={setActivatorNodeRef}
          className="icon-button playlist-drag-handle"
          aria-label="Drag to reorder"
          title="Drag to reorder"
          type="button"
          {...attributes}
          {...listeners}
          onClick={(event) => event.stopPropagation()}
        >
          ☰
        </button>
      </div>
    </div>
  );
}
