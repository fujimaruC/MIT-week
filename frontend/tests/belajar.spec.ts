import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

test('Cek cepat and the family guide are gone from the app', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByText('Cek cepat')).toHaveCount(0);
  const missing = await page.goto('/cek-cepat');
  // The SPA fallback serves its error page for an unknown route.
  expect(missing?.status()).toBeLessThan(500);
  await expect(page.getByRole('heading', { level: 1 })).not.toHaveText(/cek cepat/i);

  const family = await page.goto('/panduan-keluarga');
  expect(family?.status()).toBeLessThan(500);
  await expect(page.getByRole('heading', { level: 2, name: 'Kata sandi keluarga' })).toHaveCount(0);
});

test('the Belajar quiz runs by keyboard and ends with a score', async ({ page }) => {
  await page.goto('/edukasi');
  const quiz = page.locator('#kuis');
  await expect(quiz.getByText(/^Pertanyaan 1 dari \d+$/)).toBeVisible();

  // Answer with the keyboard only: focus the first radio, pick it, press the button.
  for (let n = 0; n < 8; n++) {
    const first = quiz.getByRole('radio').first();
    await first.focus();
    await page.keyboard.press('Space');
    await quiz.getByRole('button', { name: 'Jawab' }).focus();
    await page.keyboard.press('Enter');
    await expect(quiz.getByRole('status').first()).toContainText(/tepat/i);
    const next = quiz.getByRole('button', { name: /Pertanyaan berikutnya|Lihat nilai/ });
    const label = await next.innerText();
    await next.focus();
    await page.keyboard.press('Enter');
    if (/nilai/i.test(label)) break;
  }
  await expect(page.getByTestId('quiz-result')).toContainText(/\d+ dari 6 benar/);
});

for (const theme of ['dark', 'light'] as const) {
  test(`axe: Belajar has no serious or critical violations (${theme})`, async ({ page }) => {
    await page.addInitScript((t) => localStorage.setItem('amba-theme', t), theme);
    await page.goto('/edukasi');
    await page.getByRole('radio').first().check();
    await page.getByRole('button', { name: 'Jawab' }).click();
    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
    const blocking = results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
    expect(blocking.map((v) => `${v.id}: ${v.nodes.length}`)).toEqual([]);
  });
}
