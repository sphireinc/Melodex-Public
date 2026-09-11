export type VideoStageError = {
  browserEvent: string;
  playerErrorCode?: number | string | null;
  playerErrorMessage?: string;
  mimeType?: string;
};

export type VideoDiagnosticContextInput = {
  trackId?: string;
  jobId?: string;
  sourceType?: string;
  mimeType?: string;
  playerErrorCode?: number | string | null;
  browserEvent?: string;
  mode?: string;
  downloadMode?: string;
  message?: string;
  sourcePathAvailable?: boolean;
  remoteSourceAvailable?: boolean;
};

export type VideoDiagnosticContext = {
  correlationId: string;
  trackId: string;
  jobId: string;
  sourceType: string;
  mimeType: string;
  playerErrorCode: number | string | null;
  browserEvent: string;
  mode: string;
  downloadMode: string;
  message: string;
  sourcePathAvailable: boolean;
  remoteSourceAvailable: boolean;
};

const maxDiagnosticTextLength = 512;
const maxDiagnosticPayloadLength = 4096;
const urlPattern = /https?:\/\/[^\s"'<>]+/gi;
const sensitiveHeaderPattern = /(authorization:|cookie:|set-cookie:|x-api-key:)\s*(?:bearer\s+)?[^\s,;]+/gi;
const mimePattern = /^[a-z0-9][a-z0-9.+-]*\/[a-z0-9][a-z0-9.+-]*$/i;

function createCorrelationId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `video-${crypto.randomUUID()}`;
  }
  return `video-${Date.now().toString(36)}`;
}

function safeText(value: string | undefined, maxLength = maxDiagnosticTextLength): string {
  const normalized = (value ?? "")
    .trim()
    .replace(urlPattern, "<redacted-url>")
    .replace(sensitiveHeaderPattern, "$1 <redacted>");
  return normalized.length > maxLength ? normalized.slice(-maxLength) : normalized;
}

function safeEnum(value: string | undefined, fallback: string): string {
  const normalized = safeText(value, 80).replace(/[^a-zA-Z0-9_.:-]/g, "-");
  return normalized || fallback;
}

function safeMimeType(value: string | undefined): string {
  const normalized = safeText(value, 120).toLowerCase();
  return mimePattern.test(normalized) ? normalized : "unknown";
}

export function videoSourceType(videoPath: string, mediaURL: string, sourceRef: string): string {
  if (mediaURL.trim()) return "media-server";
  if (videoPath.trim()) return "downloaded-file";
  if (sourceRef.trim()) return "remote-reference";
  return "unknown";
}

export function videoDiagnosticContext(input: VideoDiagnosticContextInput): VideoDiagnosticContext {
  return {
    correlationId: createCorrelationId(),
    trackId: safeText(input.trackId, 160),
    jobId: safeText(input.jobId, 160),
    sourceType: safeEnum(input.sourceType, "unknown"),
    mimeType: safeMimeType(input.mimeType),
    playerErrorCode:
      typeof input.playerErrorCode === "number" || typeof input.playerErrorCode === "string"
        ? input.playerErrorCode
        : null,
    browserEvent: safeEnum(input.browserEvent, "unknown"),
    mode: safeEnum(input.mode, "unknown"),
    downloadMode: safeEnum(input.downloadMode, "unknown"),
    message: safeText(input.message),
    sourcePathAvailable: Boolean(input.sourcePathAvailable),
    remoteSourceAvailable: Boolean(input.remoteSourceAvailable),
  };
}

export function serializeVideoDiagnosticContext(input: VideoDiagnosticContextInput): string {
  const encoded = JSON.stringify(videoDiagnosticContext(input));
  return encoded.length > maxDiagnosticPayloadLength ? encoded.slice(0, maxDiagnosticPayloadLength) : encoded;
}
