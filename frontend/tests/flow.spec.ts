import { test, expect, type Page } from '@playwright/test';

/**
 * Result page flow: read-aloud (PRD F14) and the three-tap link check.
 */

async function openDemo(page: Page, name: RegExp) {
  await page.goto('/deteksi');
  await page.getByRole('button', { name }).click();
  await expect(page).toHaveURL(/\/hasil$/, { timeout: 10_000 });
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Hasil pemeriksaan');
}

/** Replaces speechSynthesis with a recorder so the test can hear it. */
async function mockSpeech(page: Page) {
  await page.addInitScript(() => {
    const spoken: string[] = [];
    (window as unknown as { __spoken: string[] }).__spoken = spoken;
    class FakeUtterance {
      text: string;
      lang = '';
      rate = 1;
      constructor(text: string) {
        this.text = text;
      }
    }
    Object.defineProperty(window, 'SpeechSynthesisUtterance', { value: FakeUtterance, configurable: true });
    Object.defineProperty(window, 'speechSynthesis', {
      configurable: true,
      value: {
        speak: (u: FakeUtterance) => spoken.push(`${u.lang}|${u.text}`),
        cancel: () => {},
        getVoices: () => []
      }
    });
  });
}

test('read-aloud speaks the lead sentence first, in Indonesian', async ({ page }) => {
  await mockSpeech(page);
  await openDemo(page, 'Video TikTok');
  const sentence = await page.locator('[data-testid="verdict-sentence"]').innerText();

  await page.getByRole('button', { name: 'Bacakan hasil' }).click();
  const spoken = await page.evaluate(() => (window as unknown as { __spoken: string[] }).__spoken);
  expect(spoken).toHaveLength(1);
  expect(spoken[0].startsWith(`id-ID|${sentence}`)).toBe(true);
});

test('auto-read is off by default and speaks the next result once enabled', async ({ page }) => {
  await mockSpeech(page);
  await openDemo(page, 'Video TikTok');
  expect(await page.evaluate(() => (window as unknown as { __spoken: string[] }).__spoken)).toEqual([]);

  await page.getByLabel('Bacakan otomatis hasil berikutnya').check();
  await openDemo(page, 'Gambar buatan AI');
  await expect
    .poll(() => page.evaluate(() => (window as unknown as { __spoken: string[] }).__spoken.length))
    .toBe(1);
});

test('a link check takes at most 3 taps', async ({ page }) => {
  await page.goto('/');
  await page.getByLabel('Tautan video atau postingan').fill('https://bri-verifikasi.example/login');
  let taps = 0;
  await page.getByRole('button', { name: 'Periksa', exact: true }).click();
  taps++;
  await expect(page).toHaveURL(/\/hasil$/, { timeout: 20_000 });
  expect(taps).toBeLessThanOrEqual(3);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Hasil pemeriksaan');
});
