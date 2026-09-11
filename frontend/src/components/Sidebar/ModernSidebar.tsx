import type { AppState, ViewKey } from "../../types";
import { aiStatusAlertClass } from "../../lib/viewHelpers";

export function ModernSidebar({
  state,
  section,
  setSection,
  search,
  setSearch,
  busy,
  onOpenSettings,
  onToggleWindowMaximize,
}: {
  state: AppState | null;
  section: ViewKey;
  setSection: (view: ViewKey) => void;
  search: string;
  setSearch: (value: string) => void;
  busy: boolean;
  onOpenSettings: () => void;
  onToggleWindowMaximize: () => Promise<void>;
}) {
  const nav = [
    { key: "library", label: "Library" },
    { key: "playlists", label: "Playlists" },
    { key: "artists", label: "Artists" },
    { key: "albums", label: "Albums" },
    { key: "songs", label: "Songs" },
    { key: "import", label: "Import" },
    { key: "processing", label: "Processing" },
    { key: "index", label: "Index" },
    { key: "settings", label: "Settings" },
  ] as const;

  return (
    <nav className="sidebar modern-sidebar">
      <div className="brand">
        <div className="brand-mark" aria-hidden="true">
          <img src="/logo_cropped.png" alt="" />
        </div>
        <div className="brand-copy">
          <div className="brand-name">Melodex</div>
        </div>
      </div>

      <label className="sidebar-search">
        <span>Search</span>
        <input
          className="text-input"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Search tracks, albums, artists"
        />
      </label>

      <div className="nav-group">
        {nav.map((item) => (
          <button
            key={item.key}
            className={`nav-item ${section === item.key ? "active" : ""}`}
            onClick={() => setSection(item.key)}
          >
            <span>{item.label}</span>
          </button>
        ))}
        <div className="nav-divider" aria-hidden="true" />
        <div className={`nav-item nav-item-status ${aiStatusAlertClass(state)}`} aria-label="AI status">
          <div className="nav-item-label">AI Status</div>
          <div className="nav-item-value">
            {state?.aiStatus ?? (state?.toolStatus.ai ? "Configured" : "Missing key")}
          </div>
          {!state?.settings.apiKeyConfigured ? (
            <div className="nav-item-recommendation">
              <span className="nav-item-recommendation-tag">Recommended next step</span>
              <div className="nav-item-recommendation-text">
                Set up AI in Settings for richer metadata, trivia, and song meaning.
              </div>
              <button className="text-action nav-item-recommendation-action" onClick={onOpenSettings}>
                Open settings
              </button>
            </div>
          ) : null}
        </div>
        <button className="nav-item nav-item-action" onClick={onToggleWindowMaximize} disabled={busy}>
          <span>{state?.windowFullscreen ? "Exit Fullscreen" : "Fullscreen"}</span>
        </button>
      </div>
    </nav>
  );
}
