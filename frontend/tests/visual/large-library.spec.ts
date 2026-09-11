import { test, expect } from "@playwright/test";

test.setTimeout(120_000);

test("large sanitized fixture keeps Library Index render windows bounded", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/?visual=1&large=1");
  await expect(page.locator(".modern-sidebar")).toBeAttached();

  const fixtureCounts = await page.evaluate(() => ({
    tracks: document.querySelectorAll(".library-index").length,
    totalTracksText: Array.from(document.querySelectorAll(".stat-tile strong, .stat-tile b"))
      .map((element) => element.textContent?.trim())
      .find((value) => value === "10000" || value === "10,000"),
  }));
  expect(fixtureCounts.tracks).toBe(0);
  expect(fixtureCounts.totalTracksText).toBeUndefined();

  const initialBrowseRows = await page.evaluate(() => ({
    artistCards: document.querySelectorAll(".artist-grid .browse-card").length,
    albumCards: document.querySelectorAll(".album-grid .browse-card").length,
  }));
  expect(initialBrowseRows.artistCards).toBeLessThan(100);
  expect(initialBrowseRows.albumCards).toBeLessThan(100);

  await page.evaluate(() => {
    for (const selector of [".artist-grid", ".album-grid"]) {
      const element = document.querySelector<HTMLElement>(selector);
      if (!element) continue;
      element.scrollTop = element.scrollHeight;
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
    }
  });
  await page.waitForTimeout(50);
  const afterBrowseScrollRows = await page.evaluate(() => ({
    artistCards: document.querySelectorAll(".artist-grid .browse-card").length,
    albumCards: document.querySelectorAll(".album-grid .browse-card").length,
  }));
  expect(afterBrowseScrollRows.artistCards).toBeLessThan(100);
  expect(afterBrowseScrollRows.albumCards).toBeLessThan(100);

  await page.getByRole("button", { name: "Index", exact: true }).click();
  await expect(page.locator(".library-index")).toBeVisible();
  await expect(page.getByText("10000 ready", { exact: true })).toBeVisible();

  const initial = await page.evaluate(() => ({
    artistRows: document.querySelectorAll(".virtual-index-list .index-row").length,
    trackRows: document.querySelectorAll(".index-track-list .modern-track-row").length,
    playlistOptions: document.querySelectorAll("option").length,
  }));
  expect(initial.artistRows).toBeLessThan(100);
  expect(initial.trackRows).toBeLessThan(100);

  const beforeScroll = await page.evaluate(() => performance.now());
  await page.locator(".index-track-list").evaluate((element) => {
    element.scrollTop = element.scrollHeight;
    element.dispatchEvent(new Event("scroll", { bubbles: true }));
  });
  await page.waitForTimeout(50);
  const afterScroll = await page.evaluate(() => ({
    elapsedMs: performance.now(),
    artistRows: document.querySelectorAll(".virtual-index-list .index-row").length,
    trackRows: document.querySelectorAll(".index-track-list .modern-track-row").length,
  }));
  expect(afterScroll.artistRows).toBeLessThan(100);
  expect(afterScroll.trackRows).toBeLessThan(100);

  await page.getByRole("button", { name: "Playlists", exact: true }).click();
  await expect(page.locator(".playlists-modern")).toBeVisible();
  const playlistRenderStarted = await page.evaluate(() => performance.now());
  await expect(page.locator(".playlist-track-row")).toHaveCount(100);
  const playlistRender = await page.evaluate(() => ({
    elapsedMs: performance.now(),
    trackRows: document.querySelectorAll(".playlist-track-row").length,
  }));
  expect(playlistRender.trackRows).toBe(100);

  await page.getByRole("button", { name: "Import", exact: true }).click();
  await expect(page.locator(".import-textarea-done")).toBeVisible();
  const historyRenderStarted = await page.evaluate(() => performance.now());
  const historyRender = await page.locator(".import-textarea-done").evaluate((element) => ({
    elapsedMs: performance.now(),
    entries: (element as HTMLTextAreaElement).value.split("\n").length,
    characters: (element as HTMLTextAreaElement).value.length,
  }));
  expect(historyRender.entries).toBe(10_000);
  expect(historyRender.characters).toBeGreaterThan(400_000);

  // Keep the output sanitized and useful for large-library profiling without
  // persisting fixture metadata or any user paths.
  console.log(
    JSON.stringify({
      fixture: { tracks: 10_000, artists: 2_000, albums: 1_000, playlists: 100 },
      initialBrowseRows,
      afterBrowseScrollRows,
      rendered: initial,
      afterScroll: {
        artistRows: afterScroll.artistRows,
        trackRows: afterScroll.trackRows,
        elapsedMs: Number((afterScroll.elapsedMs - beforeScroll).toFixed(2)),
      },
      playlist: {
        trackRows: playlistRender.trackRows,
        elapsedMs: Number((playlistRender.elapsedMs - playlistRenderStarted).toFixed(2)),
      },
      importHistory: {
        entries: historyRender.entries,
        characters: historyRender.characters,
        elapsedMs: Number((historyRender.elapsedMs - historyRenderStarted).toFixed(2)),
      },
    }),
  );
});
