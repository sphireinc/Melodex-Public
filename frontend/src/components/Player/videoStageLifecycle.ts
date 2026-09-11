export type VideoStageLifecycleInputs = {
  source: string;
  sourceType: string;
  poster: string;
};

export function videoStageRecreationKey(input: VideoStageLifecycleInputs) {
  return [input.source.trim(), input.sourceType.trim(), input.poster.trim()].join("|");
}
