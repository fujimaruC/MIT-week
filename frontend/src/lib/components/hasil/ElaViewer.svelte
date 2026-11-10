<script lang="ts">
  import { t } from '$lib/i18n/index.svelte';
  import SplitSliderViewer from './SplitSliderViewer.svelte';
  import Skeleton from '$lib/components/ui/Skeleton.svelte';
  import type { Anomaly } from '$lib/api/types';

  interface Props {
    file: File;
    anomalies?: Anomaly[];
  }

  let { file, anomalies = [] }: Props = $props();

  let originalUrl = $state<string | null>(null);
  let elaUrl = $state<string | null>(null);
  let downscaled = $state(false);
  let failed = $state(false);

  // Recompute only when the file changes. Every object URL created here is
  // revoked on cleanup, so nothing survives the component.
  $effect(() => {
    const f = file;
    let disposed = false;
    let worker: Worker | null = null;
    let heatUrl: string | null = null;
    const srcUrl = URL.createObjectURL(f);
    originalUrl = srcUrl;
    elaUrl = null;
    failed = false;
    downscaled = false;

    void (async () => {
      try {
        const bitmap = await createImageBitmap(f);
        if (disposed) {
          bitmap.close();
          return;
        }
        worker = new Worker(new URL('../../forensics/ela.worker.ts', import.meta.url), {
          type: 'module'
        });
        worker.onmessage = async (
          event: MessageEvent<{
            type: string;
            bitmap?: ImageBitmap;
            downscaled?: boolean;
          }>
        ) => {
          const msg = event.data;
          if (msg.type !== 'done' || !msg.bitmap) {
            if (msg.type === 'error' && !disposed) failed = true;
            return;
          }
          const heat = msg.bitmap;
          try {
            // The worker hands back an ImageBitmap; turn it into a URL the
            // <img> in the slider can load.
            const canvas = new OffscreenCanvas(heat.width, heat.height);
            const ctx = canvas.getContext('2d');
            if (!ctx) throw new Error('canvas unavailable');
            ctx.drawImage(heat, 0, 0);
            const blob = await canvas.convertToBlob({ type: 'image/png' });
            if (disposed) return;
            heatUrl = URL.createObjectURL(blob);
            elaUrl = heatUrl;
            downscaled = msg.downscaled ?? false;
          } catch {
            if (!disposed) failed = true;
          } finally {
            heat.close();
          }
        };
        worker.postMessage({ bitmap }, [bitmap]);
      } catch {
        if (!disposed) failed = true;
      }
    })();

    return () => {
      disposed = true;
      worker?.terminate();
      URL.revokeObjectURL(srcUrl);
      if (heatUrl) URL.revokeObjectURL(heatUrl);
      originalUrl = null;
      elaUrl = null;
    };
  });
</script>

{#if failed}
  <p class="note">
    {t('ela.failed')}
  </p>
{:else if !originalUrl || !elaUrl}
  <div class="waiting" aria-live="polite">
    <p class="note">{t('ela.preparing')}</p>
    <Skeleton lines={3} height="2.5rem" />
  </div>
{:else}
  <SplitSliderViewer
    src={originalUrl}
    elaSrc={elaUrl}
    {anomalies}
    alt={t('ela.alt')}
  />
  {#if downscaled}
    <p class="note">
      {t('ela.downscaled')}
    </p>
  {/if}
{/if}

<style>
  .note { margin: var(--space-3) 0 0; color: var(--body-2); font-size: var(--text-sm); }
  .waiting { display: grid; gap: var(--space-2); }
</style>
