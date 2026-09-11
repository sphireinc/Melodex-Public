import { useState } from "react";
import { EmptyState, SectionDivider, StatusBadge } from "../Common";
import type { Playlist, TrackRecord } from "../../types";

function SectionHeader({ title, subtitle }: { title: string; subtitle?: string }) {
  return (
    <div className="section-header">
      <div>
        <h2>{title}</h2>
        {subtitle ? <p>{subtitle}</p> : null}
      </div>
    </div>
  );
}

export function PlaylistCreateModal({
  title,
  ctaLabel,
  draftName,
  setDraftName,
  draftDescription,
  setDraftDescription,
  onClose,
  onCreate,
}: {
  title: string;
  ctaLabel: string;
  draftName: string;
  setDraftName: (value: string) => void;
  draftDescription: string;
  setDraftDescription: (value: string) => void;
  onClose: () => void;
  onCreate: () => Promise<void>;
}) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(event) => event.stopPropagation()}>
        <SectionHeader title={title} subtitle="Create a calm container for tracks in your library." />
        <label className="input-group">
          <span>Name</span>
          <input
            className="text-input"
            value={draftName}
            onChange={(event) => setDraftName(event.target.value)}
            placeholder="Evening focus"
          />
        </label>
        <label className="input-group">
          <span>Description</span>
          <textarea
            className="text-input modal-textarea"
            value={draftDescription}
            onChange={(event) => setDraftDescription(event.target.value)}
            placeholder="Optional description"
          />
        </label>
        <div className="action-row modal-actions">
          <button className="button button-secondary" onClick={onClose}>
            Cancel
          </button>
          <button className="button button-primary" onClick={() => void onCreate()} disabled={!draftName.trim()}>
            {ctaLabel}
          </button>
        </div>
      </div>
    </div>
  );
}

export function AddToPlaylistModal({
  playlists,
  track,
  onClose,
  onCreate,
  onAddExisting,
  draftName,
  setDraftName,
  draftDescription,
  setDraftDescription,
}: {
  playlists: Playlist[];
  track: TrackRecord | null;
  onClose: () => void;
  onCreate: (name: string, description: string) => Promise<void>;
  onAddExisting: (playlistID: string) => Promise<void>;
  draftName: string;
  setDraftName: (value: string) => void;
  draftDescription: string;
  setDraftDescription: (value: string) => void;
}) {
  const [search, setSearch] = useState("");
  const filtered = playlists.filter((playlist) => playlist.name.toLowerCase().includes(search.toLowerCase()));

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(event) => event.stopPropagation()}>
        <SectionHeader
          title="Add to playlist"
          subtitle={track ? `${track.title} · ${track.artist}` : "Choose a playlist for this track."}
        />
        <label className="input-group">
          <span>Search playlists</span>
          <input
            className="text-input"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Filter playlists"
          />
        </label>
        <div className="modal-list">
          {filtered.length === 0 ? (
            <EmptyState title="No playlists matched" text="Create a new playlist below instead." />
          ) : (
            filtered.map((playlist) => (
              <button key={playlist.id} className="modal-list-item" onClick={() => void onAddExisting(playlist.id)}>
                <div>
                  <div className="playlist-name">{playlist.name}</div>
                  <div className="playlist-description">{playlist.description || "No description"}</div>
                </div>
                <StatusBadge tone="neutral">{playlist.trackIds.length} tracks</StatusBadge>
              </button>
            ))
          )}
        </div>
        <SectionDivider />
        <div className="detail-title">Create new</div>
        <label className="input-group">
          <span>Name</span>
          <input
            className="text-input"
            value={draftName}
            onChange={(event) => setDraftName(event.target.value)}
            placeholder="Road trip"
          />
        </label>
        <label className="input-group">
          <span>Description</span>
          <textarea
            className="text-input modal-textarea"
            value={draftDescription}
            onChange={(event) => setDraftDescription(event.target.value)}
            placeholder="Optional description"
          />
        </label>
        <div className="action-row modal-actions">
          <button className="button button-secondary" onClick={onClose}>
            Cancel
          </button>
          <button
            className="button button-primary"
            onClick={() => void onCreate(draftName, draftDescription)}
            disabled={!draftName.trim()}
          >
            Create and add
          </button>
        </div>
      </div>
    </div>
  );
}
