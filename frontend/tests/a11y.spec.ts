import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/**
 * The interface is Indonesian only (Sir, 10 Nov 2026): the language switch,
 * the English copy and the phone display-controls toggle are gone, and so are
 * their tests. What stays is the accessibility guard those tests carried: no
 * serious or critical axe violation on the main pages, now in the one language
 * the site has, with the reference language declared on the page.
 */
/**
 * Axe measures contrast from the pixels on screen, so a block that is still
 * fading in (the page's entry animation) reads as low contrast for a moment.
 * Wait until every finite animation and transition has finished; the looping
 * decorative ones (aurora, glow) never end and do not touch text.
 */
async function settled(page: Page): Promise<void> {
  await page.waitForFunction(() =>
    document.getAnimations().every((a) => a.effect?.getTiming().iterations === Infinity || a.playState !== 'running')
  );
}

for (const path of ['/', '/deteksi', '/edukasi', '/tentang']) {
  test(`axe: no serious or critical violations on ${path}`, async ({ page }) => {
    await page.goto(path);
    await expect(page).toHaveTitle(/AMBA/);
    await expect(page.locator('html')).toHaveAttribute('lang', 'id');
    await settled(page);
    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
    const blocking = results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
    expect(blocking.map((v) => `${v.id}: ${v.nodes.length}`)).toEqual([]);
  });
}
