import { test, expect } from '@playwright/test';
import { fileURLToPath } from 'node:url';

/**
 * The ELA viewer is the only place the uploaded image survives navigation,
 * and it is new code with no other coverage. Guard the whole path: upload a
 * real PNG, reach /hasil, and confirm the comparison slider renders.
 */
test('a screenshot upload reaches the ELA comparison slider', async ({ page }) => {
  await page.goto('/deteksi');
  await page.getByRole('tab', { name: 'Gambar' }).click();
  await page.setInputFiles('#amba-file', fileURLToPath(new URL('./fixtures/sample.png', import.meta.url)));

  await page.getByRole('button', { name: 'Periksa', exact: true }).click();

  await expect(page).toHaveURL(/\/hasil$/, { timeout: 25_000 });
  await expect(page.getByRole('heading', { name: 'Tampilan gambar' })).toBeVisible();

  // The slider only appears once the worker has produced a heatmap.
  const range = page.getByRole('slider', {
    name: 'Geser untuk membandingkan gambar asli dan hasil ELA'
  });
  await expect(range).toBeVisible({ timeout: 25_000 });

  // Both layers must exist: the original and the ELA overlay.
  await expect(page.getByAltText('Peta perbedaan kompresi (ELA)')).toBeVisible();
});
