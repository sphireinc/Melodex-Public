import { useEffect, useRef } from "react";
import { EmptyState, StatusBadge } from "../Common";
import type { TrackPreview, TrackRecord } from "../../types";
import {
  badgeToneFromConfidence,
  cleanStringList,
  confidenceLabel,
  formatSourceLabel,
  initialsForTrack,
  looksLikeUrl,
  metadataLinkEntries,
  metadataLinkIconLabel,
} from "../../lib/viewHelpers";

export function TrackProfileModal({
  track,
  preview,
  onClose,
  onOpenURL,
}: {
  track: TrackRecord;
  preview: TrackPreview | null;
  onClose: () => void;
  onOpenURL: (url: string) => Promise<void>;
}) {
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    previouslyFocusedRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    closeButtonRef.current?.focus();

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onCloseRef.current();
      }
    };
    document.addEventListener("keydown", handleKeyDown);

    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      previouslyFocusedRef.current?.focus();
    };
  }, []);

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

  return (
    <div className="modal-backdrop modal-backdrop-soft" onClick={onClose}>
      <div
        className="modal modal-track-profile"
        role="dialog"
        aria-modal="true"
        aria-labelledby="track-profile-title"
        tabIndex={-1}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="modal-head">
          <div>
            <div className="modal-kicker">Track Profile</div>
            <div id="track-profile-title" className="modal-title">
              {track.title}
            </div>
          </div>
          <button
            ref={closeButtonRef}
            className="icon-button"
            onClick={onClose}
            aria-label="Close track profile"
            title="Close track profile"
          >
            ✕
          </button>
        </div>

        <div className="modal-hero">
          <div className="modal-artwork">
            {preview?.artworkDataUrl || track.artworkDataUrl ? (
              <img src={preview?.artworkDataUrl ?? track.artworkDataUrl ?? ""} alt="" />
            ) : (
              <span>{initialsForTrack(track)}</span>
            )}
          </div>
          <div className="modal-hero-copy">
            <div className="collection-meta-row">
              {isrc ? <StatusBadge tone="neutral">ISRC {isrc}</StatusBadge> : null}
              {enrichmentConfidence ? (
                <StatusBadge tone={badgeToneFromConfidence(enrichmentConfidence || "unknown")}>
                  {confidenceLabel(enrichmentConfidence || "unknown")}
                </StatusBadge>
              ) : null}
            </div>
            {songMeaning ? (
              <div className="enrichment-callout">
                <div className="enrichment-callout-label">Song meaning</div>
                <div className="enrichment-callout-text">{songMeaning}</div>
              </div>
            ) : null}
            {metadataNotes ? <div className="detail-note">{metadataNotes}</div> : null}
          </div>
        </div>

        {hasEnrichment ? (
          <div className="modal-sections">
            <div className="modal-section">
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

            <div className="modal-section">
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
                            aria-label={`Open reference ${label}`}
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
          </div>
        ) : (
          <EmptyState title="No enrichment available" text="This track does not have extra trivia or links yet." />
        )}
      </div>
    </div>
  );
}
