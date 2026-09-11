import assert from "node:assert/strict";
import { videoStageRecreationKey } from "./videoStageLifecycle";

type TestCase = { name: string; run: () => void };

const tests: TestCase[] = [
  {
    name: "video stage recreation key tracks source identity only",
    run: () => {
      const base = {
        source: "http://127.0.0.1/media/one",
        sourceType: "video/mp4",
        poster: "/poster.jpg",
      };
      assert.equal(videoStageRecreationKey(base), "http://127.0.0.1/media/one|video/mp4|/poster.jpg");
      assert.equal(
        videoStageRecreationKey({
          ...base,
          source: " http://127.0.0.1/media/one ",
          sourceType: " video/mp4 ",
          poster: " /poster.jpg ",
        }),
        videoStageRecreationKey(base),
      );
      assert.notEqual(
        videoStageRecreationKey(base),
        videoStageRecreationKey({ ...base, source: "http://127.0.0.1/media/two" }),
      );
      assert.notEqual(videoStageRecreationKey(base), videoStageRecreationKey({ ...base, sourceType: "video/webm" }));
      assert.notEqual(videoStageRecreationKey(base), videoStageRecreationKey({ ...base, poster: "/poster-2.jpg" }));
    },
  },
];

let failures = 0;
for (const test of tests) {
  try {
    test.run();
    console.log(`ok - ${test.name}`);
  } catch (error) {
    failures++;
    console.error(`not ok - ${test.name}`);
    console.error(error);
  }
}

if (failures > 0) {
  throw new Error(`${failures} video stage lifecycle test(s) failed`);
}

console.log(`video stage lifecycle tests passed (${tests.length})`);
