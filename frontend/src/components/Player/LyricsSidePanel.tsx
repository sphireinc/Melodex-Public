import { KeyValue, SectionDivider, StatusBadge } from "../Common";
import { LyricsScroller } from "./LyricsScroller";
import type { TrackPreview, TrackRecord } from "../../types";
import {
  badgeToneFromConfidence,
  cleanStringList,
  confidenceLabel,
  formatSourceLabel,
  looksLikeUrl,
  metadataLinkEntries,
  metadataLinkIconLabel,
} from "../../lib/viewHelpers";

export function LyricsSidePanel({
  track,
  preview,
  previewLoading,
  previewStatus,
  currentTime,
  onClose,
  onOpenPlayer,
  onOpenFolder,
  onOpenTxt,
  onOpenLrc,
  onOpenURL,
}: {
  track: TrackRecord | null;
  preview: TrackPreview | null;
  previewLoading: boolean;
  previewStatus: string;
  currentTime: number;
  onClose: () => void;
  onOpenPlayer: () => void;
  onOpenFolder: (path: string) => Promise<void>;
  onOpenTxt: () => void;
  onOpenLrc: () => void;
  onOpenURL: (url: string) => Promise<void>;
}) {
  return (
    <div className="drawer-panel lyrics-drawer">
      <div className="drawer-head">
        <div>
          <div className="drawer-title">{track?.title ?? "No track selected"}</div>
          <div className="drawer-subtitle">
            {track ? `${track.artist} · ${track.album}` : "Choose a track to reveal lyrics."}
          </div>
        </div>
        <button className="icon-button" onClick={onClose} aria-label="Close lyrics panel">
          ✕
        </button>
      </div>
      <div className="drawer-actions">
        <button className="button button-secondary" onClick={onOpenPlayer} disabled={!track}>
          Open player
        </button>
        <button
          className="button button-secondary"
          onClick={() => track && void onOpenFolder(track.storageDir)}
          disabled={!track}
        >
          Open folder
        </button>
      </div>
      <SectionDivider />
      <TrackEnrichmentProfile track={track} preview={preview} onOpenURL={onOpenURL} />
      <SectionDivider />
      <LyricsScroller
        variant="panel"
        track={track}
        preview={preview}
        previewLoading={previewLoading}
        previewStatus={previewStatus}
        currentTime={currentTime}
      />
      <SectionDivider />
      <div className="drawer-actions">
        <button className="button button-secondary" onClick={onOpenTxt} disabled={!track}>
          Open .txt
        </button>
        <button className="button button-secondary" onClick={onOpenLrc} disabled={!track}>
          Open .lrc
        </button>
      </div>
    </div>
  );
}

function TrackEnrichmentProfile({
  track,
  preview,
  onOpenURL,
}: {
  track: TrackRecord | null;
  preview: TrackPreview | null;
  onOpenURL: (url: string) => Promise<void>;
}) {
  if (!track) return null;

  const enrichArtistLinks = metadataLinkEntries(preview?.artistLinks ?? track.artistLinks);
  const enrichAlbumLinks = metadataLinkEntries(preview?.albumLinks ?? track.albumLinks);
  const enrichSongLinks = metadataLinkEntries(preview?.songLinks ?? track.songLinks);
  const enrichArtistTrivia = cleanStringList(preview?.artistTrivia ?? track.artistTrivia);
  const enrichAlbumTrivia = cleanStringList(preview?.albumTrivia ?? track.albumTrivia);
  const enrichSongTrivia = cleanStringList(preview?.songTrivia ?? track.songTrivia);
  const enrichTidbits = cleanStringList(preview?.tidbits ?? track.tidbits);
  const enrichSources = cleanStringList(preview?.sources ?? track.sources);
  const releaseType = preview?.releaseType ?? track.releaseType ?? "";
  const isrc = preview?.isrc ?? track.isrc ?? "";
  const enrichmentConfidence = preview?.enrichmentConfidence ?? track.enrichmentConfidence ?? "";
  const songMeaning = (preview?.songMeaning ?? track.songMeaning ?? "").trim();
  const metadataNotes = (preview?.metadataNotes ?? track.metadataNotes ?? "").trim();
  const hasEnrichment =
    Boolean(releaseType) ||
    Boolean(isrc) ||
    Boolean(enrichmentConfidence) ||
    enrichArtistLinks.length > 0 ||
    enrichAlbumLinks.length > 0 ||
    enrichSongLinks.length > 0 ||
    enrichArtistTrivia.length > 0 ||
    enrichAlbumTrivia.length > 0 ||
    enrichSongTrivia.length > 0 ||
    enrichTidbits.length > 0 ||
    enrichSources.length > 0 ||
    Boolean(songMeaning) ||
    Boolean(metadataNotes);

  if (!hasEnrichment) return null;

  return (
    <div className="enrichment-stack">
      <div className="enrichment-card">
        <div className="detail-toggle-row enrichment-head">
          <div>
            <div className="detail-title">Track Profile</div>
          </div>
          <div className="enrichment-badge-row">
            {isrc ? <StatusBadge tone="neutral">ISRC {isrc}</StatusBadge> : null}
            {enrichmentConfidence ? (
              <StatusBadge tone={badgeToneFromConfidence(enrichmentConfidence || "unknown")}>
                {confidenceLabel(enrichmentConfidence || "unknown")}
              </StatusBadge>
            ) : null}
          </div>
        </div>

        {songMeaning ? (
          <div className="enrichment-callout">
            <div className="enrichment-callout-label">Song meaning</div>
            <div className="enrichment-callout-text">{songMeaning}</div>
          </div>
        ) : null}

        {metadataNotes ? <div className="enrichment-note">{metadataNotes}</div> : null}

        <div className="detail-metadata-grid enrichment-grid">
          <KeyValue label="Release type" value={releaseType || "—"} />
          <KeyValue label="ISRC" value={isrc || "—"} />
          <KeyValue label="Enrichment confidence" value={confidenceLabel(enrichmentConfidence || "unknown")} />
        </div>
      </div>

      {enrichArtistLinks.length > 0 || enrichAlbumLinks.length > 0 || enrichSongLinks.length > 0 ? (
        <div className="enrichment-card">
          <div className="detail-title">Artist, album, and song links</div>
          <div className="enrichment-link-grid">
            {enrichArtistLinks.length > 0 ? (
              <div className="enrichment-link-card">
                <div className="detail-subtitle">Artist links</div>
                <div className="link-icon-row">
                  {enrichArtistLinks.map((entry) => (
                    <button
                      key={`artist-${entry.label}-${entry.url}`}
                      className="link-icon-button"
                      onClick={() => void onOpenURL(entry.url)}
                      aria-label={entry.label}
                    >
                      <span aria-hidden="true">{metadataLinkIconLabel(entry.label)}</span>
                      <span className="sr-only">{entry.label}</span>
                    </button>
                  ))}
                </div>
              </div>
            ) : null}
            {enrichAlbumLinks.length > 0 ? (
              <div className="enrichment-link-card">
                <div className="detail-subtitle">Album links</div>
                <div className="link-icon-row">
                  {enrichAlbumLinks.map((entry) => (
                    <button
                      key={`album-${entry.label}-${entry.url}`}
                      className="link-icon-button"
                      onClick={() => void onOpenURL(entry.url)}
                      aria-label={entry.label}
                    >
                      <span aria-hidden="true">{metadataLinkIconLabel(entry.label)}</span>
                      <span className="sr-only">{entry.label}</span>
                    </button>
                  ))}
                </div>
              </div>
            ) : null}
            {enrichSongLinks.length > 0 ? (
              <div className="enrichment-link-card">
                <div className="detail-subtitle">Song links</div>
                <div className="link-icon-row">
                  {enrichSongLinks.map((entry) => (
                    <button
                      key={`song-${entry.label}-${entry.url}`}
                      className="link-icon-button"
                      onClick={() => void onOpenURL(entry.url)}
                      aria-label={entry.label}
                    >
                      <span aria-hidden="true">{metadataLinkIconLabel(entry.label)}</span>
                      <span className="sr-only">{entry.label}</span>
                    </button>
                  ))}
                </div>
              </div>
            ) : null}
          </div>
        </div>
      ) : null}

      {enrichArtistTrivia.length > 0 ||
      enrichAlbumTrivia.length > 0 ||
      enrichSongTrivia.length > 0 ||
      enrichTidbits.length > 0 ||
      enrichSources.length > 0 ? (
        <div className="enrichment-card">
          <div className="detail-title">Trivia, tidbits, and references</div>
          <div className="enrichment-notes-grid">
            {enrichArtistTrivia.length > 0 ? (
              <div className="enrichment-note-card">
                <div className="detail-subtitle">Artist trivia</div>
                <ul className="detail-bullet-list">
                  {enrichArtistTrivia.map((item) => (
                    <li key={item} className="detail-bullet-item">
                      {item}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            {enrichAlbumTrivia.length > 0 ? (
              <div className="enrichment-note-card">
                <div className="detail-subtitle">Album trivia</div>
                <ul className="detail-bullet-list">
                  {enrichAlbumTrivia.map((item) => (
                    <li key={item} className="detail-bullet-item">
                      {item}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            {enrichSongTrivia.length > 0 ? (
              <div className="enrichment-note-card">
                <div className="detail-subtitle">Song trivia</div>
                <ul className="detail-bullet-list">
                  {enrichSongTrivia.map((item) => (
                    <li key={item} className="detail-bullet-item">
                      {item}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            {enrichTidbits.length > 0 ? (
              <div className="enrichment-note-card">
                <div className="detail-subtitle">Tidbits</div>
                <ul className="detail-bullet-list">
                  {enrichTidbits.map((item) => (
                    <li key={item} className="detail-bullet-item">
                      {item}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            {enrichSources.length > 0 ? (
              <div className="enrichment-note-card">
                <div className="detail-subtitle">References</div>
                <div className="enrichment-reference-list">
                  {enrichSources.map((item) => {
                    const label = formatSourceLabel(item);
                    const isUrl = looksLikeUrl(item);
                    return isUrl ? (
                      <button
                        key={item}
                        className="link-chip link-chip-muted enrichment-reference-link"
                        onClick={() => void onOpenURL(item)}
                      >
                        {label}
                      </button>
                    ) : (
                      <div key={item} className="enrichment-reference-text">
                        {label}
                      </div>
                    );
                  })}
                </div>
              </div>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}
