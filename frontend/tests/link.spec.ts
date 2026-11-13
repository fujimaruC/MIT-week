import { test, expect } from '@playwright/test';

/**
 * PRD F1 against the real API (LLM_MOCK=true). The test server cannot reach
 * the internet, which is exactly the "page shows no preview" case: the result
 * says what it rests on, in plain words and not as an error, and still
 * concludes (Sir, 10 Nov 2026).
 */
test('a link without a preview says what the result rests on and still concludes', async ({ page }) => {
  await page.goto('/deteksi');
  await page.getByLabel('Tautan video, postingan, atau berita').fill('https://bri-verifikasi.example/login');
  await page.getByRole('button', { name: 'Periksa', exact: true }).click();

  await expect(page).toHaveURL(/\/hasil$/, { timeout: 20_000 });
  const preview = page.locator('[data-testid="source-metadata"]');
  await expect(preview).toContainText('Halaman ini tidak menampilkan pratinjau.');
  await expect(preview).toContainText('Hasil di atas memakai pemeriksaan yang tercantum di bawah.');
  await expect(preview.getByRole('button')).toHaveCount(0);

  // The link pillar still ran and found the lookalike; the verdict is never
  // "belum cukup data" for a link this risky.
  await expect(page.getByText('Domain meniru nama layanan populer').first()).toBeAttached();
  await expect(page.locator('[data-testid="verdict-badge"]')).not.toContainText('Belum cukup data');
});

test('the screenshot demo opens the ELA viewer and OCR tactics', async ({ page }) => {
  const apiCalls: string[] = [];
  page.on('request', (r) => {
    if (r.url().includes('/api/v1/')) apiCalls.push(r.url());
  });

  await page.goto('/deteksi');
  await page.getByRole('button', { name: 'Tangkapan layar diedit' }).click();
  await expect(page).toHaveURL(/\/hasil$/, { timeout: 10_000 });
  // The written detail (OCR text, tactics) sits under the full details.
  await page.getByText('Lihat rincian lengkap').click();

  await expect(page.getByLabel('Geser untuk membandingkan gambar asli dan hasil ELA')).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText('Dibaca dari gambar')).toBeVisible();
  await expect(page.locator('mark.tactic')).toContainText('SEBARKAN SEBELUM DIHAPUS');
  // Timpa Teks: the pasted price reacts to compression unlike the other text.
  await expect(page.getByText('Kompresi teks ini berbeda dari teks lain di gambar').first()).toBeVisible();
  expect(apiCalls).toEqual([]);
});
