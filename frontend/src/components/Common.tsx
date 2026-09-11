import { useEffect, useRef, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import type { FacetBucket } from "../types";
import { getVirtualWindow } from "../lib/virtualWindow";

export function SectionHeader({ title, subtitle }: { title: string; subtitle?: string }) {
  return (
    <div className="section-header">
      <div>
        <h2>{title}</h2>
        {subtitle ? <p>{subtitle}</p> : null}
      </div>
    </div>
  );
}

export function SettingsCard({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <section className="settings-card">
      <div className="settings-card-head">
        <h3>{title}</h3>
        {description ? <p>{description}</p> : null}
      </div>
      <div className="settings-card-body">{children}</div>
    </section>
  );
}

export function EmptyState({ title, text, actions }: { title: string; text: string; actions?: ReactNode }) {
  return (
    <div className="empty-state">
      <div className="empty-state-title empty-title">{title}</div>
      <div className="empty-state-text empty-text">{text}</div>
      {actions ? <div className="empty-state-actions empty-actions">{actions}</div> : null}
    </div>
  );
}

export function InfoField({ label, value }: { label: string; value: string }) {
  return (
    <div className="info-field">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export function KeyValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="key-value">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export function StatusLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="status-line">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export function ToggleChip({
  label,
  value,
  onChange,
}: {
  label: string;
  value: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <button className={`chip ${value ? "active" : ""}`} onClick={() => onChange(!value)}>
      {label}
    </button>
  );
}

export function FacetSelect({
  label,
  value,
  onChange,
  options,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: string[];
  placeholder: string;
}) {
  return (
    <label className="facet-select-group">
      <span>{label}</span>
      <select className="text-input facet-select" value={value} onChange={(event) => onChange(event.target.value)}>
        <option value="">{placeholder}</option>
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
    </label>
  );
}

export function TrackFilterRow({
  selectedGenre,
  setSelectedGenre,
  genreBuckets,
  selectedYear,
  setSelectedYear,
  yearBuckets,
  filterLyrics: _filterLyrics,
  setFilterLyrics: _setFilterLyrics,
  filterNeedsReview: _filterNeedsReview,
  setFilterNeedsReview: _setFilterNeedsReview,
  filterMissingMetadata: _filterMissingMetadata,
  setFilterMissingMetadata: _setFilterMissingMetadata,
}: {
  selectedGenre: string;
  setSelectedGenre: (value: string) => void;
  genreBuckets: FacetBucket[];
  selectedYear: string;
  setSelectedYear: (value: string) => void;
  yearBuckets: FacetBucket[];
  filterLyrics: boolean;
  setFilterLyrics: (value: boolean) => void;
  filterNeedsReview: boolean;
  setFilterNeedsReview: (value: boolean) => void;
  filterMissingMetadata: boolean;
  setFilterMissingMetadata: (value: boolean) => void;
}) {
  return (
    <div className="filter-row">
      <FacetSelect
        label="Genre"
        value={selectedGenre}
        onChange={setSelectedGenre}
        options={genreBuckets.map((bucket) => bucket.value)}
        placeholder="All genres"
      />
      <FacetSelect
        label="Year"
        value={selectedYear}
        onChange={setSelectedYear}
        options={yearBuckets.map((bucket) => bucket.value)}
        placeholder="All years"
      />
    </div>
  );
}

export function StatTile({ label, value }: { label: string; value: number }) {
  return (
    <div className="stat-tile">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export function StatusBadge({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: "neutral" | "success" | "warning" | "danger" | "accent";
}) {
  return <span className={`status-inline status-badge tone-${tone}`}>{children}</span>;
}

export function SectionDivider() {
  return <div className="section-divider" />;
}

export function ProgressBar({ value }: { value: number }) {
  const safeValue = Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0));
  return (
    <div className="progress-bar" aria-hidden="true">
      <div className="progress-bar-fill" style={{ width: `${safeValue}%` }} />
    </div>
  );
}

export function VirtualList<T>({
  items,
  itemHeight,
  renderItem,
  getKey,
  className,
  style,
  overscan = 6,
  empty,
}: {
  items: T[];
  itemHeight: number;
  renderItem: (item: T, index: number) => ReactNode;
  getKey: (item: T, index: number) => string | number;
  className?: string;
  style?: CSSProperties;
  overscan?: number;
  empty?: ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [viewportHeight, setViewportHeight] = useState(itemHeight * Math.min(items.length || 1, 8));
  const [scrollTop, setScrollTop] = useState(0);

  useEffect(() => {
    const node = ref.current;
    if (!node) {
      return;
    }
    const update = () => {
      setViewportHeight(Math.max(itemHeight, node.clientHeight || itemHeight * Math.min(items.length || 1, 8)));
    };
    update();
    if (typeof ResizeObserver !== "undefined") {
      const observer = new ResizeObserver(update);
      observer.observe(node);
      return () => observer.disconnect();
    }
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, [itemHeight, items.length]);

  useEffect(() => {
    setScrollTop(0);
  }, [items.length]);

  if (items.length === 0) {
    return <>{empty}</>;
  }

  const virtualWindow = getVirtualWindow(items.length, itemHeight, viewportHeight, scrollTop, overscan);
  const { start, end, paddingTop, paddingBottom } = virtualWindow;
  const visible = items.slice(start, end);

  return (
    <div
      ref={ref}
      className={className}
      style={{ ...style, overflowY: "auto" }}
      onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
    >
      <div style={{ paddingTop, paddingBottom }}>
        {visible.map((item, index) => (
          <div key={getKey(item, start + index)} style={{ height: itemHeight }}>
            {renderItem(item, start + index)}
          </div>
        ))}
      </div>
    </div>
  );
}

export function VirtualGrid<T>({
  items,
  itemHeight,
  minColumnWidth,
  gap,
  renderItem,
  getKey,
  className,
  style,
  overscan = 2,
  empty,
}: {
  items: T[];
  itemHeight: number;
  minColumnWidth: number;
  gap: number;
  renderItem: (item: T, index: number) => ReactNode;
  getKey: (item: T, index: number) => string | number;
  className?: string;
  style?: CSSProperties;
  overscan?: number;
  empty?: ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [viewportHeight, setViewportHeight] = useState(itemHeight * 2);
  const [columnCount, setColumnCount] = useState(1);
  const [scrollTop, setScrollTop] = useState(0);

  useEffect(() => {
    const node = ref.current;
    if (!node) return;

    const update = () => {
      const width = node.clientWidth;
      const nextColumns = Math.max(1, Math.floor((width + gap) / (minColumnWidth + gap)));
      setColumnCount(nextColumns);
      setViewportHeight(Math.max(itemHeight, node.clientHeight || itemHeight * 2));
    };
    update();
    if (typeof ResizeObserver !== "undefined") {
      const observer = new ResizeObserver(update);
      observer.observe(node);
      return () => observer.disconnect();
    }
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, [gap, itemHeight, items.length, minColumnWidth]);

  useEffect(() => {
    setScrollTop(0);
  }, [items.length, columnCount]);

  if (items.length === 0) return <>{empty}</>;

  const rowCount = Math.ceil(items.length / columnCount);
  const virtualWindow = getVirtualWindow(rowCount, itemHeight, viewportHeight, scrollTop, overscan);
  const startItem = virtualWindow.start * columnCount;
  const endItem = Math.min(items.length, virtualWindow.end * columnCount);
  const visible = items.slice(startItem, endItem);

  return (
    <div
      ref={ref}
      className={className}
      style={{ ...style, display: "block", overflowY: "auto" }}
      onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
    >
      <div
        className="virtual-grid-content"
        style={{
          display: "grid",
          gridTemplateColumns: `repeat(${columnCount}, minmax(0, 1fr))`,
          columnGap: gap,
          paddingTop: virtualWindow.paddingTop,
          paddingBottom: virtualWindow.paddingBottom,
        }}
      >
        {visible.map((item, index) => (
          <div key={getKey(item, startItem + index)} style={{ height: itemHeight }}>
            {renderItem(item, startItem + index)}
          </div>
        ))}
      </div>
    </div>
  );
}
