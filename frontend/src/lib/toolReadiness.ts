import type { ToolReadiness } from "../types";

export function toolReadinessLabel(readiness: ToolReadiness | undefined, detected: boolean): string {
  switch (readiness?.status) {
    case "ready":
      return "Ready";
    case "missing":
      return "Missing";
    case "probe-failed":
      return "Check failed";
    case "invalid-version":
      return "Invalid version";
    case "timeout":
      return "Timed out";
    case "unchecked":
      return detected ? "Not tested" : "Not ready";
    default:
      return detected ? "Detected" : "Not ready";
  }
}

export function toolReadinessDetail(readiness: ToolReadiness | undefined): string {
  if (!readiness) return "Run the tool test to verify it before starting an import.";
  return [readiness.error, readiness.remediation].filter(Boolean).join(" ");
}
