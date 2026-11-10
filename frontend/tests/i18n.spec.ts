import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/** PRD P1: EN/ID toggle. Indonesian stays the default and the reference copy. */
test('the language switch translates the interface and is remembered', async ({ page }) => {
  await page.goto('/deteksi');
  await expect(page.locator('html')).toHaveAttribute('lang', 'id');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Periksa konten');

  await page.getByRole('button', { name: 'English' }).click();
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Check content');
  await expect(page.getByRole('tab', { name: 'Image' })).toBeVisible();

  await page.reload();
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Check content');
  await expect(page).toHaveTitle('Check content — AMBA');

  await page.getByRole('button', { name: 'Bahasa Indonesia' }).click();
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Periksa konten');
});

test('a result in English rebuilds the verdict copy from its codes', async ({ page }) => {
  await page.goto('/deteksi');
  await page.getByRole('button', { name: 'English' }).click();
  await page.getByRole('button', { name: 'TikTok video' }).click();
  await expect(page).toHaveURL(/\/hasil$/, { timeout: 10_000 });
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Check result');
  await expect(page.locator('[data-testid="verdict-sentence"]')).toHaveText(
    'Trusted sources contradict this. Do not share it.'
  );
  await expect(page.locator('[data-testid="verdict-badge"]')).toContainText('Likely hoax or manipulated');
  // PRD §10: the footer keeps the legal disclaimer verbatim, in Indonesian.
  const legal = page.locator('footer [lang="id"]');
  await expect(legal).toContainText('Berita Acara Pemeriksaan (BAP)');
});

for (const path of ['/', '/deteksi', '/edukasi', '/tentang']) {
  test(`axe in English: no serious or critical violations on ${path}`, async ({ page }) => {
    await page.addInitScript(() => localStorage.setItem('amba.lang', 'en'));
    await page.goto(path);
    await expect(page).toHaveTitle(/AMBA/);
    await expect(page.locator('html')).toHaveAttribute('lang', 'en');
    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
    const blocking = results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
    expect(blocking.map((v) => `${v.id}: ${v.nodes.length}`)).toEqual([]);
  });
}

test('on a phone the display controls open from one toggle', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 780 });
  await page.goto('/');
  const toggle = page.getByRole('button', { name: 'Buka pengaturan tampilan' });
  await expect(page.getByRole('button', { name: 'English' })).toBeHidden();
  await toggle.click();
  await expect(page.getByRole('button', { name: 'English' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Tutup pengaturan tampilan' })).toHaveAttribute('aria-expanded', 'true');
});
