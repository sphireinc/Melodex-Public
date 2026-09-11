import { test, expect, type Page } from "@playwright/test";

test.setTimeout(120_000);

const viewports = [
  { name: "wide", width: 1440, height: 1000 },
  { name: "medium", width: 1120, height: 900 },
  { name: "narrow", width: 760, height: 900 },
];

async function prepare(page: Page, width: number, height: number) {
  await page.setViewportSize({ width, height });
  await page.goto("/?visual=1");
  await expect(page.locator(".modern-sidebar")).toBeAttached();
  await page.addStyleTag({
    content:
      "*, *::before, *::after { animation-duration: 0s !important; transition-duration: 0s !important; caret-color: transparent !important; }",
  });
}

async function capture(page: Page, name: string, viewport: string) {
  await expect(page.locator(".workspace")).toHaveScreenshot(`workspace-${name}-${viewport}.png`, {
    animations: "disabled",
    maxDiffPixelRatio: 0.01,
  });
}

async function expectNoHorizontalOverflow(page: Page) {
  const dimensions = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth + 1);
}

for (const viewport of viewports) {
  test(`library and navigation layout - ${viewport.name}`, async ({ page }) => {
    await prepare(page, viewport.width, viewport.height);
    await expectNoHorizontalOverflow(page);
    await capture(page, "library", viewport.name);

    for (const label of ["Artists", "Albums", "Songs", "Playlists", "Import", "Processing", "Settings"]) {
      await page.getByRole("button", { name: label, exact: true }).click();
      await expect(page.locator(".workspace")).toBeVisible();
      await expectNoHorizontalOverflow(page);
      await capture(page, label.toLowerCase(), viewport.name);
    }
  });

  test(`player, queue, profile, and video recovery - ${viewport.name}`, async ({ page }) => {
    await prepare(page, viewport.width, viewport.height);

    const firstTrack = page.locator(".modern-track-row").first();
    await firstTrack.focus();
    await firstTrack.press("Enter");
    await expect(firstTrack).toBeFocused();

    if (viewport.width <= 900) {
      const playerLayout = await page.locator(".compact-player-bar").evaluate((element) => {
        const style = window.getComputedStyle(element);
        return { columns: style.gridTemplateColumns, rows: style.gridTemplateAreas };
      });
      expect(playerLayout.columns.split(" ")).toHaveLength(1);
      expect(playerLayout.rows).toContain('"top"');
      expect(playerLayout.rows).toContain('"middle"');
      expect(playerLayout.rows).toContain('"bottom"');
    }

    await page.getByRole("button", { name: "Toggle queue" }).click();
    await expect(page.locator(".queue-drawer")).toBeVisible();
    await capture(page, "queue", viewport.name);
    await page.getByRole("button", { name: "Close queue" }).click();

    await page.getByRole("button", { name: "Open player" }).first().click();
    await expect(page.locator(".full-player-screen")).toBeVisible();
    await capture(page, "player", viewport.name);

    await page.getByRole("button", { name: "Track profile" }).click();
    await expect(page.locator(".modal-track-profile")).toBeVisible();
    await expect(page.locator('[role="dialog"]')).toHaveAttribute("aria-modal", "true");
    await expect(page.getByRole("button", { name: "Close track profile" })).toBeFocused();
    await capture(page, "track-profile", viewport.name);
    await page.keyboard.press("Escape");
    await expect(page.locator(".modal-track-profile")).toBeHidden();

    await page.getByRole("button", { name: "Close player" }).click();
    await page.getByRole("button", { name: "Open player" }).first().click();
    await page.getByRole("button", { name: "Play Video" }).dispatchEvent("click");
    await expect(page.locator(".video-stage-shell")).toBeVisible();
    await expect(page.getByRole("button", { name: "Close video" })).toBeVisible();
    await expectNoHorizontalOverflow(page);
    await capture(page, "video-recovery", viewport.name);

    await expect(page.locator(".video-fullscreen-modal")).toBeVisible();
    await expect(page.getByRole("button", { name: "Exit video fullscreen" })).toBeVisible();
    await capture(page, "video-fullscreen", viewport.name);
    await page.getByRole("button", { name: "Exit video fullscreen" }).click();
    await expect(page.locator(".video-fullscreen-modal")).toBeHidden();
  });
}
