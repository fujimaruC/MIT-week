import { test, expect } from '@playwright/test';

/**
 * The one flow that matters: a real audit against the real API must reach
 * /hasil with a verdict. This is the regression guard for the contract bug
 * where the server serialised empty slices as null and the Zod schema threw.
 */
test('a link audit reaches the result page with a verdict', async ({ page }) => {
  const consoleErrors: string[] = [];
  page.on('pageerror', (e) => consoleErrors.push(e.message));

  await page.goto('/deteksi');
  await page.getByLabel('Tautan video, postingan, atau berita').fill('https://bri-verifikasi.xyz/login');
  await page.getByRole('button', { name: 'Periksa', exact: true }).click();

  await expect(page).toHaveURL(/\/hasil$/, { timeout: 20_000 });
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(/Tidak ditemukan tanda bahaya|Perlu diperiksa|Terindikasi hoaks atau manipulasi/);
  await expect(page.getByRole('img', { name: /Skor risiko \d+ dari 100/ })).toBeVisible();
  // The misleading network message must never appear on a successful audit.
  await expect(page.getByText('Pemeriksaan terhenti sebelum selesai')).toHaveCount(0);
  expect(consoleErrors).toEqual([]);
});

test('the result page never says a check could not be done', async ({ page }) => {
  await page.goto('/deteksi');
  await page.getByLabel('Tautan video, postingan, atau berita').fill('https://bri-verifikasi.xyz/login');
  await page.getByRole('button', { name: 'Periksa', exact: true }).click();

  await expect(page).toHaveURL(/\/hasil$/, { timeout: 20_000 });

  // Only checks that ran get a card; the limits panel (under the full details) says what the result is based on.
  await page.getByText('Lihat rincian lengkap').click();
  await expect(page.locator('details').first()).toBeVisible();
  await expect(page.getByText('Berdasarkan:')).toBeVisible();
  await expect(page.getByText(/tidak dapat diperiksa/i)).toHaveCount(0);
  await expect(page.getByText(/belum cukup data/i)).toHaveCount(0);
});
