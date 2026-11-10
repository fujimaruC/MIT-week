/**
 * The three bundled example results. They make zero API calls; each opens
 * /hasil with a labelled demo dossier (and its simulated image).
 */
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { result } from '$lib/result.svelte';
import demoData from '$lib/data/demo-cases.json';
import type { MessageKey } from '$lib/i18n/index.svelte';

export const EXAMPLES: Array<{ id: string; label: MessageKey; icon: 'video' | 'sparkle' | 'image' }> = [
  { id: 'demo-tiktok', label: 'ex.tiktok', icon: 'video' },
  { id: 'demo-gambar-ai', label: 'ex.ai', icon: 'sparkle' },
  { id: 'demo-berita-ig', label: 'ex.edit', icon: 'image' }
];

type DemoCase = { id: string; image?: string; dossier: unknown };
const demos = demoData.cases as DemoCase[];

export async function openExample(id: string): Promise<void> {
  const demo = demos.find((d) => d.id === id);
  if (!demo) return;
  let image: File | null = null;
  if (demo.image) {
    try {
      const res = await fetch(demo.image);
      if (res.ok) image = new File([await res.blob()], 'contoh-simulasi.jpg', { type: 'image/jpeg' });
    } catch {
      // The result still opens; only the image viewer is skipped.
    }
  }
  try {
    const { parseDossier } = await import('$lib/api/schemas');
    result.set(parseDossier(demo.dossier), image);
    void goto(resolve('/hasil'));
  } catch {
    // A bundled example that fails the contract is a build-time bug, not a
    // user-facing failure. Keep the page usable.
  }
}
