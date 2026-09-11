import { useMemo } from "react";
import { SectionHeader, EmptyState, StatusBadge, TrackFilterRow, VirtualGrid, VirtualList } from "../Common";
import type { AppState, FacetBucket, TrackRecord, ViewKey, BrowseContext, QueueSource } from "../../types";
import {
  badgeToneFromTrack,
  getTracksForBrowseContext,
  groupAlbums,
  groupArtists,
  initialsForTrack,
  metadataLabel,
  selectedBrowseAlbum,
  selectedBrowseArtist,
  shuffleList,
} from "../../lib/viewHelpers";
import { ModernTrackRow } from "./LibraryIndexPage";

type BrowseHistoryEntry = {
  context: BrowseContext;
  view: ViewKey;
};

export function MusicBrowserPage({
  section,
  browseContext,
  browseHistory,
  tracks,
  filteredTracks,
  hasTrackFilters,
  search: _search,
  setSearch: _setSearch,
  state,
  genreBuckets,
  yearBuckets,
  filterLyrics,
  setFilterLyrics,
  filterNeedsReview,
  setFilterNeedsReview,
  filterMissingMetadata,
  setFilterMissingMetadata,
  selectedGenre,
  setSelectedGenre,
  selectedYear,
  setSelectedYear,
  selectedTrackPath,
  currentTrackId,
  currentTrackPlaying,
  onSelectTrack,
  onPlayTrack,
  onToggleTrackPlayback,
  onOpenTrackPlayer,
  onOpenTrackVideo,
  onOpenLyrics,
  onAddToPlaylist,
  onRevealTrack,
  onOpenPlayer,
  onOpenImport,
  onOpenSettings,
  onNavigateBrowse,
  onBackBrowse,
  onResetBrowse,
}: {
  section: ViewKey;
  browseContext: BrowseContext;
  browseHistory: BrowseHistoryEntry[];
  tracks: TrackRecord[];
  filteredTracks: TrackRecord[];
  hasTrackFilters: boolean;
  search: string;
  setSearch: (value: string) => void;
  state: AppState | null;
  genreBuckets: FacetBucket[];
  yearBuckets: FacetBucket[];
  filterLyrics: boolean;
  setFilterLyrics: (value: boolean) => void;
  filterNeedsReview: boolean;
  setFilterNeedsReview: (value: boolean) => void;
  filterMissingMetadata: boolean;
  setFilterMissingMetadata: (value: boolean) => void;
  selectedGenre: string;
  setSelectedGenre: (value: string) => void;
  selectedYear: string;
  setSelectedYear: (value: string) => void;
  selectedTrackPath: string;
  currentTrackId: string;
  currentTrackPlaying: boolean;
  onSelectTrack: (track: TrackRecord) => void;
  onPlayTrack: (track: TrackRecord, queueTracks?: TrackRecord[], source?: QueueSource) => Promise<void>;
  onToggleTrackPlayback: (track: TrackRecord, queueTracks?: TrackRecord[], source?: QueueSource) => Promise<void>;
  onOpenTrackPlayer: (track: TrackRecord, queueTracks?: TrackRecord[], source?: QueueSource) => Promise<void>;
  onOpenTrackVideo: (track: TrackRecord, queueTracks?: TrackRecord[], source?: QueueSource) => Promise<void>;
  onOpenLyrics: (track: TrackRecord) => void;
  onAddToPlaylist: (track: TrackRecord) => void;
  onRevealTrack: (path: string) => Promise<void>;
  onOpenPlayer: () => void;
  onOpenImport: () => void;
  onOpenSettings: () => void;
  onNavigateBrowse: (context: BrowseContext, view: ViewKey) => void;
  onBackBrowse: () => void;
  onResetBrowse: (view: ViewKey) => void;
}) {
  const collectionTracks = hasTrackFilters ? filteredTracks : tracks;
  const scopedTracks = useMemo(
    () => getTracksForBrowseContext(collectionTracks, browseContext),
    [browseContext, collectionTracks],
  );
  const artistGroups = useMemo(() => groupArtists(scopedTracks), [scopedTracks]);
  const albumGroups = useMemo(() => groupAlbums(scopedTracks), [scopedTracks]);
  if (tracks.length === 0) {
    return (
      <section className="content-stack">
        <section className="hero-card onboarding-hero">
          <div className="onboarding-layout">
            <div className="collection-art collection-art-large onboarding-art">
              <img className="collection-art-logo" src="/logo.png" alt="Melodex" />
            </div>
            <div className="music-hero-copy onboarding-copy">
              <h2 className="collection-title">Import Music to start your library.</h2>
              <p className="hero-description">
                Load a folder, import a URL you own or have rights to use, or pick local files to build your catalog. AI
                is strongly recommended because it adds richer metadata, trivia, and song meaning, but it is not
                required to get started.
              </p>
              <div className="collection-meta-row">
                <StatusBadge tone={state?.settings.apiKeyConfigured ? "success" : "warning"}>
                  {state?.settings.apiKeyConfigured ? "AI ready" : "AI recommended"}
                </StatusBadge>
                <StatusBadge tone="neutral">Music import first</StatusBadge>
              </div>
              <div className="onboarding-checklist">
                <div className="onboarding-step">
                  <span>1</span>
                  <div>
                    <div className="onboarding-step-title">Import music</div>
                    <div className="onboarding-step-text">
                      Use a URL you own or have rights to use, load a folder, or pick local files.
                    </div>
                  </div>
                </div>
                <div className="onboarding-step">
                  <span>2</span>
                  <div>
                    <div className="onboarding-step-title">Set up AI metadata</div>
                    <div className="onboarding-step-text">
                      Recommended for richer artist, album, trivia, and song meaning details.
                    </div>
                  </div>
                </div>
                <div className="onboarding-step">
                  <span>3</span>
                  <div>
                    <div className="onboarding-step-title">Play and organize</div>
                    <div className="onboarding-step-text">
                      Melodex will catalog, enrich, and keep your library on disk.
                    </div>
                  </div>
                </div>
              </div>
              <div className="action-row">
                <button className="button button-primary" onClick={onOpenImport}>
                  Import Music
                </button>
                <button className="button button-secondary" onClick={onOpenSettings}>
                  Open Settings
                </button>
                <button
                  className="button button-secondary"
                  onClick={onOpenPlayer}
                  disabled={!state?.playback.currentTrackId}
                >
                  Open player
                </button>
              </div>
            </div>
          </div>
        </section>
      </section>
    );
  }
  const selectedArtist = selectedBrowseArtist(artistGroups, browseContext);
  const selectedAlbum = selectedBrowseAlbum(albumGroups, browseContext);
  const visibleTracks = section === "songs" ? scopedTracks : scopedTracks;
  const queueSource: QueueSource =
    browseContext.mode === "album"
      ? "album"
      : browseContext.mode === "songs" && browseContext.albumKey
        ? "album"
        : "library";
  const heroTrack =
    browseContext.mode === "album" || browseContext.mode === "songs"
      ? (selectedAlbum?.firstTrack ?? scopedTracks[0] ?? tracks[0] ?? null)
      : browseContext.mode === "artist"
        ? (selectedArtist?.firstTrack ?? scopedTracks[0] ?? tracks[0] ?? null)
        : (scopedTracks[0] ?? tracks[0] ?? null);
  const heroTitle =
    browseContext.mode === "artist"
      ? browseContext.artistName
      : browseContext.mode === "album" || browseContext.mode === "songs"
        ? (browseContext.albumTitle ?? "Album")
        : "Library";
  const heroSubtitle =
    browseContext.mode === "artist"
      ? selectedArtist
        ? `${selectedArtist.trackCount} tracks · ${selectedArtist.albumCount} albums`
        : "Browse the albums for this artist"
      : browseContext.mode === "album" || browseContext.mode === "songs"
        ? (browseContext.artistName ?? "Browse the songs in this album")
        : "Browse your local library";
  const heroMeta =
    browseContext.mode === "artist"
      ? selectedArtist?.sampleAlbums?.join(" · ") || "No albums"
      : browseContext.mode === "album" || browseContext.mode === "songs"
        ? `${selectedAlbum?.tracks.length ?? scopedTracks.length} tracks`
        : `${scopedTracks.length} tracks`;
  const heroDescription =
    browseContext.mode === "songs"
      ? "Songs in this album are shown directly below the pinned header."
      : browseContext.mode === "artist"
        ? "Choose an album to drill down into the songs."
        : "";

  const renderArtistCard = (artist: (typeof artistGroups)[number]) => (
    <div
      key={artist.artist}
      className={`browse-card browse-card-grid ${selectedArtist?.artist === artist.artist ? "active" : ""}`}
      role="button"
      tabIndex={0}
      onClick={() =>
        onNavigateBrowse(
          {
            mode: "artist",
            artistKey: artist.artistKey,
            artistName: artist.artist,
          },
          "albums",
        )
      }
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onNavigateBrowse(
            {
              mode: "artist",
              artistKey: artist.artistKey,
              artistName: artist.artist,
            },
            "albums",
          );
        }
      }}
    >
      <div className="browse-card-art browse-card-art-large">
        {artist.firstTrack.artworkDataUrl?.trim() ? (
          <img src={artist.firstTrack.artworkDataUrl} alt="" />
        ) : (
          <span>{initialsForTrack(artist.firstTrack)}</span>
        )}
      </div>
      <div className="browse-card-body">
        <div className="browse-card-title">{artist.artist}</div>
        <div className="browse-card-subtitle">
          {artist.trackCount} tracks · {artist.albumCount} albums
        </div>
        <div className="browse-card-metadata">
          <span>{artist.sampleAlbums?.[0] ?? "No albums"}</span>
        </div>
      </div>
      <div className="browse-card-footer">
        <StatusBadge tone={badgeToneFromTrack(artist.firstTrack)}>{metadataLabel(artist.firstTrack)}</StatusBadge>
        <button
          className="icon-button browse-card-play"
          onClick={(event) => {
            event.stopPropagation();
            void onPlayTrack(artist.firstTrack, artist.tracks, "library");
          }}
          aria-label={`Play ${artist.artist}`}
        >
          ▶
        </button>
      </div>
    </div>
  );

  const renderAlbumCard = (album: (typeof albumGroups)[number]) => {
    const active = selectedAlbum?.key === album.key;
    const timedCount = album.tracks.filter((track) => track.hasTimedLyrics).length;
    return (
      <div
        key={album.key}
        className={`browse-card browse-card-grid ${active ? "active" : ""}`}
        role="button"
        tabIndex={0}
        onClick={() => {
          onNavigateBrowse(
            {
              mode: "album",
              artistKey: album.artistKey,
              artistName: album.artist,
              albumKey: album.key,
              albumTitle: album.album,
            },
            "songs",
          );
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            onNavigateBrowse(
              {
                mode: "album",
                artistKey: album.artistKey,
                artistName: album.artist,
                albumKey: album.key,
                albumTitle: album.album,
              },
              "songs",
            );
          }
        }}
      >
        <div className="browse-card-art browse-card-art-large">
          {album.firstTrack.artworkDataUrl?.trim() ? (
            <img src={album.firstTrack.artworkDataUrl} alt="" />
          ) : (
            <span>{initialsForTrack(album.firstTrack)}</span>
          )}
        </div>
        <div className="browse-card-body">
          <div className="browse-card-title">{album.album}</div>
          <div className="browse-card-subtitle">{album.artist}</div>
          <div className="browse-card-metadata">
            <span>{album.tracks.length} tracks</span>
            <span>{timedCount > 0 ? `${timedCount} timed` : "Plain"}</span>
          </div>
        </div>
        <div className="browse-card-footer">
          <StatusBadge tone={badgeToneFromTrack(album.firstTrack)}>{metadataLabel(album.firstTrack)}</StatusBadge>
          <button
            className="icon-button browse-card-play"
            onClick={(event) => {
              event.stopPropagation();
              void onPlayTrack(album.firstTrack, album.tracks, "album");
            }}
            aria-label={`Play ${album.album}`}
          >
            ▶
          </button>
        </div>
      </div>
    );
  };

  const artistGrid =
    artistGroups.length > 250 ? (
      <VirtualGrid
        className="artist-grid virtual-browse-grid"
        style={{ maxHeight: "min(68vh, 900px)" }}
        items={artistGroups}
        itemHeight={320}
        minColumnWidth={210}
        gap={16}
        getKey={(artist) => artist.key}
        renderItem={renderArtistCard}
      />
    ) : (
      <div className="artist-grid">{artistGroups.map(renderArtistCard)}</div>
    );
  const albumGrid =
    albumGroups.length > 250 ? (
      <VirtualGrid
        className="album-grid virtual-browse-grid"
        style={{ maxHeight: "min(68vh, 900px)" }}
        items={albumGroups}
        itemHeight={320}
        minColumnWidth={210}
        gap={16}
        getKey={(album) => album.key}
        renderItem={renderAlbumCard}
      />
    ) : (
      <div className="album-grid">{albumGroups.map(renderAlbumCard)}</div>
    );

  return (
    <section className="content-stack music-page">
      <div className="browse-toolbar">
        <div className="browse-breadcrumbs">
          <button className="browse-breadcrumb" onClick={() => onResetBrowse("library")}>
            Library
          </button>
          {browseContext.mode === "artist" || browseContext.mode === "album" || browseContext.mode === "songs" ? (
            <>
              <span className="browse-breadcrumb-separator">/</span>
              <button
                className="browse-breadcrumb"
                onClick={() =>
                  onNavigateBrowse(
                    {
                      mode: "artist",
                      artistKey: browseContext.artistKey ?? selectedArtist?.artist ?? "Unknown Artist",
                      artistName: browseContext.artistName ?? selectedArtist?.artist ?? "Unknown Artist",
                    },
                    "albums",
                  )
                }
              >
                {browseContext.artistName ?? selectedArtist?.artist ?? "Artist"}
              </button>
            </>
          ) : null}
          {browseContext.mode === "album" || browseContext.mode === "songs" ? (
            <>
              <span className="browse-breadcrumb-separator">/</span>
              <button
                className="browse-breadcrumb"
                onClick={() =>
                  onNavigateBrowse(
                    {
                      mode: "album",
                      artistKey: browseContext.artistKey ?? selectedArtist?.artist ?? "Unknown Artist",
                      artistName: browseContext.artistName ?? selectedArtist?.artist ?? "Unknown Artist",
                      albumKey: browseContext.albumKey ?? selectedAlbum?.key ?? "Unknown Album",
                      albumTitle: browseContext.albumTitle ?? selectedAlbum?.album ?? "Unknown Album",
                    },
                    "songs",
                  )
                }
              >
                {browseContext.albumTitle ?? selectedAlbum?.album ?? "Album"}
              </button>
            </>
          ) : null}
          {browseContext.mode === "songs" ? (
            <>
              <span className="browse-breadcrumb-separator">/</span>
              <span className="browse-breadcrumb browse-breadcrumb-current">Songs</span>
            </>
          ) : null}
        </div>
        <button className="button button-secondary" onClick={onBackBrowse} disabled={browseHistory.length === 0}>
          Back
        </button>
      </div>

      <div className="music-hero">
        <div className="collection-art collection-art-large">
          {heroTrack?.artworkDataUrl?.trim() ? (
            <img src={heroTrack.artworkDataUrl} alt="" />
          ) : heroTrack ? (
            <span>{initialsForTrack(heroTrack)}</span>
          ) : (
            <img className="collection-art-logo" src="/logo.png" alt="Melodex" />
          )}
        </div>
        <div className="music-hero-copy">
          <h2 className="collection-title">{heroTitle}</h2>
          <div className="collection-subtitle">{heroSubtitle}</div>
          <div className="collection-meta-row">
            <StatusBadge tone="neutral">{heroMeta}</StatusBadge>
            {heroTrack ? (
              <StatusBadge tone={badgeToneFromTrack(heroTrack)}>{metadataLabel(heroTrack)}</StatusBadge>
            ) : null}
            {heroTrack?.hasTimedLyrics ? (
              <StatusBadge tone="success">Timed lyrics</StatusBadge>
            ) : (
              <StatusBadge tone="neutral">Plain lyrics</StatusBadge>
            )}
          </div>
          {heroDescription ? <div className="hero-description">{heroDescription}</div> : null}
          <div className="action-row">
            <button
              className="button button-primary"
              onClick={() => {
                if (heroTrack) void onPlayTrack(heroTrack, visibleTracks, queueSource);
              }}
              disabled={!heroTrack}
            >
              Play
            </button>
            <button
              className="button button-secondary"
              onClick={() => {
                if (!heroTrack) return;
                const ordered = shuffleList(visibleTracks);
                void onPlayTrack(ordered[0] ?? heroTrack, ordered.length ? ordered : visibleTracks, queueSource);
              }}
              disabled={!heroTrack}
            >
              Shuffle
            </button>
            <button
              className="button button-secondary"
              onClick={() => {
                if (heroTrack) onAddToPlaylist(heroTrack);
              }}
              disabled={!heroTrack}
            >
              Add
            </button>
            <button className="button button-secondary" onClick={onOpenPlayer} disabled={!heroTrack}>
              Open player
            </button>
          </div>
        </div>
      </div>

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

      {section === "library" ? (
        <section className="browse-section">
          <SectionHeader title="Artists" subtitle="Pick an artist to drill down into its albums." />
          {artistGrid}
        </section>
      ) : null}

      {section === "albums" ? (
        <section className="browse-section">
          <SectionHeader
            title={browseContext.mode === "artist" ? `Albums by ${browseContext.artistName}` : "Albums"}
            subtitle={
              browseContext.mode === "artist"
                ? "Choose an album to open its song list."
                : "Open albums and focus on one record at a time."
            }
          />
          {albumGrid}
        </section>
      ) : null}

      {section === "artists" ? (
        <section className="browse-section">
          <SectionHeader title="Artists" subtitle="Browse by artist, then jump into the selected artist's albums." />
          {artistGrid}
        </section>
      ) : null}

      {section === "songs" ? (
        <section className="collection-card">
          <div className="track-list-header">
            <div className="track-list-head track-index-col">#</div>
            <div className="track-list-head track-main-col">Title</div>
            <div className="track-list-head track-meta-col">Artist / Album</div>
            <div className="track-list-head track-duration-col">Time</div>
            <div className="track-list-head track-status-col">Status</div>
            <div className="track-list-head track-actions-col" />
          </div>
          <VirtualList
            className="virtual-track-list"
            style={{ maxHeight: "min(68vh, 900px)" }}
            items={visibleTracks}
            itemHeight={72}
            getKey={(track) => track.id}
            empty={
              <EmptyState
                title="No songs found."
                text={
                  browseContext.mode === "album"
                    ? "This album no longer has tracks or its metadata changed."
                    : "Try a different search or clear the filters."
                }
              />
            }
            renderItem={(track, index) => (
              <ModernTrackRow
                track={track}
                index={index}
                active={track.metadataPath === selectedTrackPath}
                playing={track.id === currentTrackId}
                playbackActive={track.id === currentTrackId && currentTrackPlaying}
                onSelect={() => onSelectTrack(track)}
                onPlay={() => void onToggleTrackPlayback(track, visibleTracks, queueSource)}
                onOpenPlayer={() => void onOpenTrackPlayer(track, visibleTracks, queueSource)}
                onOpenVideo={() => void onOpenTrackVideo(track, visibleTracks, queueSource)}
                onOpenLyrics={() => onOpenLyrics(track)}
                onAddToPlaylist={() => onAddToPlaylist(track)}
                onReveal={() => void onRevealTrack(track.storageDir)}
              />
            )}
          />
        </section>
      ) : null}

      {section === "library" ? (
        <>
          <section className="browse-section">
            <SectionHeader title="Albums" subtitle="Open an album to focus on its songs." />
            {albumGrid}
          </section>

          <section className="collection-card">
            <div className="track-list-header">
              <div className="track-list-head track-index-col">#</div>
              <div className="track-list-head track-main-col">Title</div>
              <div className="track-list-head track-meta-col">Artist / Album</div>
              <div className="track-list-head track-duration-col">Time</div>
              <div className="track-list-head track-status-col">Status</div>
              <div className="track-list-head track-actions-col" />
            </div>
            <VirtualList
              className="virtual-track-list"
              style={{ maxHeight: "min(68vh, 900px)" }}
              items={scopedTracks}
              itemHeight={72}
              getKey={(track) => track.id}
              empty={
                <EmptyState
                  title="Your library is empty."
                  text="Import a track to start building your Melodex collection."
                />
              }
              renderItem={(track, index) => (
                <ModernTrackRow
                  track={track}
                  index={index}
                  active={track.metadataPath === selectedTrackPath}
                  playing={track.id === currentTrackId}
                  playbackActive={track.id === currentTrackId && currentTrackPlaying}
                  onSelect={() => onSelectTrack(track)}
                  onPlay={() => void onToggleTrackPlayback(track, scopedTracks, "library")}
                  onOpenPlayer={() => void onOpenTrackPlayer(track, scopedTracks, "library")}
                  onOpenVideo={() => void onOpenTrackVideo(track, scopedTracks, "library")}
                  onOpenLyrics={() => onOpenLyrics(track)}
                  onAddToPlaylist={() => onAddToPlaylist(track)}
                  onReveal={() => void onRevealTrack(track.storageDir)}
                />
              )}
            />
          </section>
        </>
      ) : null}

      {section === "artists" && artistGroups.length === 0 ? (
        <EmptyState
          title="No artists found."
          text="Try a different search or clear the filters."
          actions={
            <>
              <button className="button button-secondary" onClick={() => onResetBrowse("library")}>
                Back to library
              </button>
            </>
          }
        />
      ) : null}

      {section === "albums" && albumGroups.length === 0 ? (
        <EmptyState
          title={
            browseContext.mode === "artist" ? `No albums found for ${browseContext.artistName}.` : "No albums found."
          }
          text={
            browseContext.mode === "artist"
              ? "Choose a different artist or go back to the library overview."
              : "Try a different search or clear the filters."
          }
          actions={
            <>
              <button className="button button-secondary" onClick={onBackBrowse} disabled={browseHistory.length === 0}>
                Back
              </button>
              <button className="button button-secondary" onClick={() => onResetBrowse("library")}>
                Library
              </button>
            </>
          }
        />
      ) : null}

      {section === "songs" && scopedTracks.length === 0 ? (
        <EmptyState
          title={
            browseContext.mode === "album"
              ? `No songs found for ${browseContext.albumTitle}.`
              : "No songs match the current search."
          }
          text={
            browseContext.mode === "album"
              ? "This album may have moved, been reprocessed, or its metadata changed."
              : "Try a different search or clear the filters."
          }
          actions={
            <>
              <button className="button button-secondary" onClick={onBackBrowse} disabled={browseHistory.length === 0}>
                Back
              </button>
              <button className="button button-secondary" onClick={() => onResetBrowse("library")}>
                Library
              </button>
            </>
          }
        />
      ) : null}
    </section>
  );
}
