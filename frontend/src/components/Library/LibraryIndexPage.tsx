import { useMemo, useState, type KeyboardEvent } from "react";
import { MdOpenInFull, MdPause, MdPlayArrow, MdVideocam } from "react-icons/md";
import { EmptyState, SectionHeader, StatTile, StatusBadge, TrackFilterRow, VirtualList } from "../Common";
import type { AppState, FacetBucket, Playlist, QueueSource, TrackRecord } from "../../types";
import {
  badgeToneFromTrack,
  formatDuration,
  groupAlbums,
  groupArtists,
  initialsForTrack,
  metadataLabel,
} from "../../lib/viewHelpers";

export function LibraryIndexPage({
  state,
  tracks,
  filteredTracks,
  hasTrackFilters,
  search: _search,
  genreBuckets,
  yearBuckets,
  selectedGenre,
  setSelectedGenre,
  selectedYear,
  setSelectedYear,
  filterLyrics,
  setFilterLyrics,
  filterNeedsReview,
  setFilterNeedsReview,
  filterMissingMetadata,
  setFilterMissingMetadata,
  selectedTrackPath,
  currentTrackId,
  currentTrackPlaying,
  queueTracks,
  playlists,
  playlistMissingCount,
  queueMissingCount,
  onSelectTrack,
  onToggleTrackPlayback,
  onOpenTrackPlayer,
  onOpenTrackVideo,
  onOpenLyrics,
  onAddToPlaylist,
  onRevealTrack,
  onRescan,
  onRepairLibraryFiles,
  onReprocessUnprocessed,
}: {
  state: AppState;
  tracks: TrackRecord[];
  filteredTracks: TrackRecord[];
  hasTrackFilters: boolean;
  search: string;
  genreBuckets: FacetBucket[];
  yearBuckets: FacetBucket[];
  selectedGenre: string;
  setSelectedGenre: (value: string) => void;
  selectedYear: string;
  setSelectedYear: (value: string) => void;
  filterLyrics: boolean;
  setFilterLyrics: (value: boolean) => void;
  filterNeedsReview: boolean;
  setFilterNeedsReview: (value: boolean) => void;
  filterMissingMetadata: boolean;
  setFilterMissingMetadata: (value: boolean) => void;
  selectedTrackPath: string;
  currentTrackId: string;
  currentTrackPlaying: boolean;
  queueTracks: TrackRecord[];
  playlists: Playlist[];
  playlistMissingCount: number;
  queueMissingCount: number;
  onSelectTrack: (track: TrackRecord) => void;
  onToggleTrackPlayback: (track: TrackRecord, queueTracks?: TrackRecord[], source?: QueueSource) => Promise<void>;
  onOpenTrackPlayer: (track: TrackRecord, queueTracks?: TrackRecord[], source?: QueueSource) => Promise<void>;
  onOpenTrackVideo: (track: TrackRecord, queueTracks?: TrackRecord[], source?: QueueSource) => Promise<void>;
  onOpenLyrics: (track: TrackRecord) => void;
  onAddToPlaylist: (track: TrackRecord) => void;
  onRevealTrack: (path: string) => Promise<void>;
  onRescan: () => Promise<void>;
  onRepairLibraryFiles: () => Promise<void>;
  onReprocessUnprocessed: () => Promise<void>;
}) {
  const visibleTracks = useMemo(() => (hasTrackFilters ? filteredTracks : tracks), [filteredTracks, hasTrackFilters, tracks]);
  const artists = useMemo(() => groupArtists(visibleTracks), [visibleTracks]);
  const albums = useMemo(() => groupAlbums(visibleTracks), [visibleTracks]);
  const readyCount = state.health.readyTracks;
  const unprocessedCount = state.health.unprocessedTracks;

  return (
    <section className="content-stack library-index">
      <section className="hero-card import-hero index-hero">
        <div className="playlist-hero-top">
          <div>
            <h2 className="collection-title">Library Index</h2>
            <p className="hero-description">Fast lookup across artists, albums, and tracks.</p>
            <div className="collection-meta-row">
              <StatusBadge tone="accent">{readyCount} ready</StatusBadge>
              <StatusBadge tone={unprocessedCount > 0 ? "warning" : "success"}>
                {unprocessedCount} unprocessed
              </StatusBadge>
              <StatusBadge tone={playlistMissingCount > 0 ? "warning" : "success"}>
                {playlistMissingCount} missing playlist entries
              </StatusBadge>
              <StatusBadge tone={queueMissingCount > 0 ? "warning" : "success"}>
                {queueMissingCount} missing queue entries
              </StatusBadge>
            </div>
          </div>
          <div className="action-row index-actions">
            <button className="button button-secondary" onClick={onRepairLibraryFiles}>
              Repair
            </button>
            <button
              className="button button-secondary"
              onClick={onReprocessUnprocessed}
              disabled={unprocessedCount === 0}
            >
              Reprocess
            </button>
            <button className="button button-primary" onClick={onRescan}>
              Refresh
            </button>
          </div>
        </div>
      </section>

      <TrackFilterRow
        genreBuckets={genreBuckets}
        yearBuckets={yearBuckets}
        selectedGenre={selectedGenre}
        setSelectedGenre={setSelectedGenre}
        selectedYear={selectedYear}
        setSelectedYear={setSelectedYear}
        filterLyrics={filterLyrics}
        setFilterLyrics={setFilterLyrics}
        filterNeedsReview={filterNeedsReview}
        setFilterNeedsReview={setFilterNeedsReview}
        filterMissingMetadata={filterMissingMetadata}
        setFilterMissingMetadata={setFilterMissingMetadata}
      />

      <div className="index-report-grid">
        <section className="collection-card index-report-card">
          <SectionHeader title="Library health" />
          <div className="index-stat-grid">
            <StatTile label="Total tracks" value={state.health.totalTracks} />
            <StatTile label="Ready tracks" value={state.health.readyTracks} />
            <StatTile label="Unprocessed" value={state.health.unprocessedTracks} />
            <StatTile label="Missing audio" value={state.health.missingAudio} />
            <StatTile label="Missing lyrics" value={state.health.missingLyrics} />
            <StatTile label="Missing metadata" value={state.health.missingMetadata} />
          </div>
        </section>

        <section className="collection-card index-report-card">
          <SectionHeader title="Consistency report" />
          <div className="index-stat-grid">
            <StatTile label="Playlists" value={playlists.length} />
            <StatTile label="Missing playlists" value={playlistMissingCount} />
            <StatTile label="Queue length" value={queueTracks.length} />
            <StatTile label="Missing queue" value={queueMissingCount} />
          </div>
        </section>
      </div>

      <div className="index-grid">
        <section className="collection-card index-panel">
          <SectionHeader title="Artists" subtitle="Fast lookup by artist." />
          <VirtualList
            className="virtual-index-list"
            style={{ maxHeight: "min(50vh, 520px)" }}
            items={artists}
            itemHeight={72}
            getKey={(artist) => artist.key}
            empty={<EmptyState title="No artists found" text="Import tracks or clear the current filters." />}
            renderItem={(artist) => (
              <button
                className={`index-row ${selectedTrackPath && artist.tracks.some((track) => track.metadataPath === selectedTrackPath) ? "active" : ""}`}
                onClick={() => onSelectTrack(artist.firstTrack)}
              >
                <div className="index-row-main">
                  <div className="index-row-title">{artist.artist}</div>
                  <div className="index-row-subtitle">
                    {artist.trackCount} tracks · {artist.albumCount} albums
                  </div>
                </div>
                <StatusBadge tone={badgeToneFromTrack(artist.firstTrack)}>
                  {metadataLabel(artist.firstTrack)}
                </StatusBadge>
              </button>
            )}
          />
        </section>

        <section className="collection-card index-panel">
          <SectionHeader title="Albums" subtitle="Fast lookup by album." />
          <VirtualList
            className="virtual-index-list"
            style={{ maxHeight: "min(50vh, 520px)" }}
            items={albums}
            itemHeight={72}
            getKey={(album) => album.key}
            empty={<EmptyState title="No albums found" text="Import tracks or clear the current filters." />}
            renderItem={(album) => (
              <button
                className={`index-row ${selectedTrackPath && album.tracks.some((track) => track.metadataPath === selectedTrackPath) ? "active" : ""}`}
                onClick={() => onSelectTrack(album.firstTrack)}
              >
                <div className="index-row-main">
                  <div className="index-row-title">{album.album}</div>
                  <div className="index-row-subtitle">
                    {album.artist} · {album.tracks.length} tracks
                  </div>
                </div>
                <StatusBadge tone={badgeToneFromTrack(album.firstTrack)}>{metadataLabel(album.firstTrack)}</StatusBadge>
              </button>
            )}
          />
        </section>

        <section className="collection-card index-panel index-panel-wide">
          <SectionHeader title="Tracks" subtitle="Fast lookup across the catalog." />
          {visibleTracks.length === 0 ? (
            <EmptyState title="No tracks match" text="Try a different search or clear the filters." />
          ) : (
            <VirtualList
              className="virtual-track-list index-track-list"
              style={{ maxHeight: "min(68vh, 900px)" }}
              items={visibleTracks}
              itemHeight={72}
              getKey={(track) => track.id}
              renderItem={(track, index) => (
                <ModernTrackRow
                  track={track}
                  index={index}
                  active={track.metadataPath === selectedTrackPath}
                  playing={track.id === currentTrackId}
                  playbackActive={track.id === currentTrackId && currentTrackPlaying}
                  onSelect={() => onSelectTrack(track)}
                  onPlay={() => void onToggleTrackPlayback(track, visibleTracks, "library")}
                  onOpenPlayer={() => void onOpenTrackPlayer(track, visibleTracks, "library")}
                  onOpenVideo={() => void onOpenTrackVideo(track, visibleTracks, "library")}
                  onOpenLyrics={() => onOpenLyrics(track)}
                  onAddToPlaylist={() => onAddToPlaylist(track)}
                  onReveal={() => void onRevealTrack(track.storageDir)}
                />
              )}
            />
          )}
        </section>
      </div>
    </section>
  );
}

export function ModernTrackRow({
  track,
  index,
  active,
  playing,
  playbackActive,
  onSelect,
  onPlay,
  onOpenPlayer,
  onOpenVideo,
  onOpenLyrics,
  onAddToPlaylist,
  onReveal,
}: {
  track: TrackRecord;
  index: number;
  active: boolean;
  playing: boolean;
  playbackActive: boolean;
  onSelect: () => void;
  onPlay: () => void;
  onOpenPlayer: () => void;
  onOpenVideo: () => void;
  onOpenLyrics: () => void;
  onAddToPlaylist: () => void;
  onReveal: () => void;
}) {
  const [artHovered, setArtHovered] = useState(false);
  const handleRowKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.target !== event.currentTarget || (event.key !== "Enter" && event.key !== " ")) {
      return;
    }
    event.preventDefault();
    onSelect();
  };

  return (
    <div
      className={`modern-track-row ${active ? "active" : ""} ${playing ? "playing" : ""}`}
      onClick={onSelect}
      onDoubleClick={onPlay}
      onKeyDown={handleRowKeyDown}
      role="button"
      tabIndex={0}
      aria-label={`${track.title} by ${track.artist}`}
      aria-pressed={active}
    >
      <div className="track-index-col">
        <span className="track-index-number">{String(index + 1).padStart(2, "0")}</span>
        {playing ? <span className="playing-pill">Now</span> : null}
      </div>
      <div className="track-main-col">
        <button
          className={`track-art-thumb ${artHovered ? "hovered" : ""}`}
          onClick={(event) => {
            event.stopPropagation();
            onPlay();
          }}
          onMouseEnter={() => setArtHovered(true)}
          onMouseLeave={() => setArtHovered(false)}
          aria-label={`Play ${track.title}`}
        >
          {track.artworkDataUrl?.trim() ? (
            <img src={track.artworkDataUrl} alt="" />
          ) : (
            <span>{initialsForTrack(track)}</span>
          )}
          <span className="track-art-thumb-overlay" aria-hidden="true">
            ▶
          </span>
        </button>
        <div className="track-main-stack">
          <div className="track-title-row">
            <span className="track-title">{track.title}</span>
            {track.hasTimedLyrics ? <span className="track-dot timed" /> : null}
          </div>
          <div className="track-subtitle-row">
            {track.artist} · {track.album}
          </div>
        </div>
      </div>
      <div className="track-meta-col">
        <div>{track.artist}</div>
        <div>{track.album}</div>
      </div>
      <div className="track-duration-col">{formatDuration(track.durationSeconds ?? 0)}</div>
      <div className="track-status-col">
        <StatusBadge tone={badgeToneFromTrack(track)}>{metadataLabel(track)}</StatusBadge>
        <StatusBadge tone={track.hasTimedLyrics ? "success" : "neutral"}>
          {track.hasTimedLyrics ? "Timed" : "Plain"}
        </StatusBadge>
      </div>
      <div className="track-actions-col">
        <button
          className="icon-button"
          onClick={(event) => {
            event.stopPropagation();
            onPlay();
          }}
          aria-label={playbackActive ? "Pause track" : "Play track"}
        >
          {playbackActive ? <MdPause aria-hidden="true" /> : <MdPlayArrow aria-hidden="true" />}
        </button>
        <button
          className="icon-button"
          onClick={(event) => {
            event.stopPropagation();
            onOpenVideo();
          }}
          disabled={!track.videoPath?.trim() && !track.sourceRef.trim()}
          aria-label="Open video"
        >
          <MdVideocam aria-hidden="true" />
        </button>
        <button
          className="icon-button"
          onClick={(event) => {
            event.stopPropagation();
            onOpenPlayer();
          }}
          aria-label="Play in expanded player"
        >
          <MdOpenInFull aria-hidden="true" />
        </button>
        <button
          className="icon-button"
          onClick={(event) => {
            event.stopPropagation();
            onOpenLyrics();
          }}
          aria-label="Open lyrics"
        >
          ✦
        </button>
        <button
          className="icon-button"
          onClick={(event) => {
            event.stopPropagation();
            onAddToPlaylist();
          }}
          aria-label="Add to playlist"
        >
          ＋
        </button>
        <button
          className="icon-button"
          onClick={(event) => {
            event.stopPropagation();
            onReveal();
          }}
          aria-label="Reveal in folder"
        >
          ↗
        </button>
      </div>
    </div>
  );
}
