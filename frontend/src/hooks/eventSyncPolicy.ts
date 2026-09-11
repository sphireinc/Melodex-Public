/**
 * State polling is only needed when the Wails event bridge is unavailable.
 * Packaged runtime state is delivered through targeted/full state events.
 */
export function shouldUseStatePolling(eventSubscriptionsAvailable: boolean): boolean {
  return !eventSubscriptionsAvailable;
}

export type EventSyncKind = "job-progress" | "playback" | "structural";

export type EventSyncDecision = {
  apply: boolean;
  reason: "ordered-targeted-event" | "authoritative-structural-snapshot" | "unknown-target" | "invalid-payload";
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasPlaybackContract(value: unknown): boolean {
  if (!isRecord(value)) return false;
  return (
    typeof value.currentTrackId === "string" &&
    typeof value.currentTrackPath === "string" &&
    Array.isArray(value.queue) &&
    typeof value.queueSource === "string" &&
    typeof value.isPlaying === "boolean" &&
    typeof value.isLoading === "boolean" &&
    typeof value.currentTime === "number" &&
    typeof value.duration === "number" &&
    typeof value.volume === "number" &&
    typeof value.muted === "boolean" &&
    typeof value.shuffleEnabled === "boolean" &&
    typeof value.repeatMode === "string" &&
    typeof value.error === "string"
  );
}

function hasStructuralContract(value: unknown): boolean {
  if (!isRecord(value)) return false;
  return (
    isRecord(value.settings) &&
    isRecord(value.stats) &&
    isRecord(value.health) &&
    isRecord(value.workers) &&
    Array.isArray(value.jobs) &&
    Array.isArray(value.libraryTracks) &&
    Array.isArray(value.recentTracks) &&
    Array.isArray(value.playlists) &&
    hasPlaybackContract(value.playback)
  );
}

/**
 * Apply the event-sync contract shared by the Wails event hook and its tests.
 *
 * The backend event payloads currently contain no sequence/revision field.
 * Therefore an older-looking but valid payload cannot be identified as stale;
 * valid events are applied in the order delivered by the event bridge. Unknown
 * job targets and malformed payloads are ignored because applying those would
 * mutate unrelated or incomplete state.
 */
export function targetedEventPolicy(
  kind: EventSyncKind,
  payload: unknown,
  knownJobIds?: ReadonlySet<string>,
): EventSyncDecision {
  if (kind === "job-progress") {
    if (!isRecord(payload) || typeof payload.jobId !== "string" || payload.jobId.trim() === "") {
      return { apply: false, reason: "invalid-payload" };
    }
    if (knownJobIds && !knownJobIds.has(payload.jobId)) {
      return { apply: false, reason: "unknown-target" };
    }
    return { apply: true, reason: "ordered-targeted-event" };
  }

  if (kind === "playback") {
    return hasPlaybackContract(payload)
      ? { apply: true, reason: "ordered-targeted-event" }
      : { apply: false, reason: "invalid-payload" };
  }

  return hasStructuralContract(payload)
    ? { apply: true, reason: "authoritative-structural-snapshot" }
    : { apply: false, reason: "invalid-payload" };
}
