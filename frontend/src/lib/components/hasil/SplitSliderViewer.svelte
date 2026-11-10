<script lang="ts">
  import { t } from '$lib/i18n/index.svelte';
  import type { Anomaly } from '$lib/api/types';

  interface Props {
    src: string;
    elaSrc?: string | null;
    anomalies?: Anomaly[];
    alt?: string;
  }

  let { src, elaSrc = null, anomalies = [], alt = '' }: Props = $props();

  let position = $state(50);
  let view = $state<'slider' | 'original' | 'ela'>('slider');

  const hasEla = $derived(!!elaSrc);

  function onRangeKey(e: KeyboardEvent) {
    const step = e.key === 'PageUp' || e.key === 'PageDown' ? 10 : 2;
    if (e.key === 'ArrowRight' || e.key === 'PageUp') {
      position = Math.min(100, position + step);
      e.preventDefault();
    } else if (e.key === 'ArrowLeft' || e.key === 'PageDown') {
      position = Math.max(0, position - step);
      e.preventDefault();
    } else if (e.key === 'Home') {
      position = 0;
      e.preventDefault();
    } else if (e.key === 'End') {
      position = 100;
      e.preventDefault();
    }
  }

  // scoring.ELAEvidenceMin: from here the text check counts as evidence.
  const TEXT_EVIDENCE_MIN = 50;

  // Boxes are normalized 0-1000 as [ymin, xmin, ymax, xmax] against the image's
  // own dimensions, so percentages hold at any aspect ratio.
  const boxes = $derived(
    anomalies.map((a) => {
      const [ymin, xmin, ymax, xmax] = a.bounding_box;
      return {
        top: ymin / 10,
        left: xmin / 10,
        width: (xmax - xmin) / 10,
        height: (ymax - ymin) / 10,
        reason: a.reason ?? '',
        text: a.extracted_text ?? '',
        evidence: a.evidence ?? null
      };
    })
  );
</script>

<div class="viewer">
  <div class="tabs" role="tablist" aria-label={t('slider.tabs')}>
    {#if hasEla}
      <button type="button" role="tab" aria-selected={view === 'slider'} onclick={() => (view = 'slider')}>{t('slider.slide')}</button>
    {/if}
    <button type="button" role="tab" aria-selected={view === 'original'} onclick={() => (view = 'original')}>{t('slider.original')}</button>
    {#if hasEla}
      <button type="button" role="tab" aria-selected={view === 'ela'} onclick={() => (view = 'ela')}>ELA</button>
    {/if}
  </div>

  <div class="stage">
    <img {src} alt={alt || t('slider.alt')} class="base" />

    {#if hasEla && view !== 'original'}
      <div class="overlay" style:clip-path={view === 'slider' ? `inset(0 0 0 ${position}%)` : 'none'}>
        <img src={elaSrc} alt={t('slider.elaAlt')} class="base fill" />
      </div>
    {/if}

    <!-- Boxes sit OUTSIDE the clip layer so they stay visible on both sides. -->
    <div class="boxes" aria-hidden="true">
      {#each boxes as box, i (i)}
        <span
          class="box"
          style:top="{box.top}%"
          style:left="{box.left}%"
          style:width="{box.width}%"
          style:height="{box.height}%"
        ></span>
      {/each}
    </div>

    {#if hasEla && view === 'slider'}
      <div class="handle" style:left="{position}%" aria-hidden="true"></div>
    {/if}
  </div>

  {#if hasEla && view === 'slider'}
    <input
      type="range"
      min="0"
      max="100"
      step="1"
      bind:value={position}
      aria-label={t('slider.aria')}
      aria-valuetext={t('slider.valuetext', { n: position })}
      onkeydown={onRangeKey}
    />
  {/if}
</div>

{#if boxes.length > 0}
  <ul class="anomalies">
    {#each boxes as box, i (i)}
      <li>
        <span class="anomaly-num" aria-hidden="true">{i + 1}</span>
        <span>
          <strong>{t('slider.area', { n: i + 1 })}</strong>
          {#if box.text}<span class="quote">“{box.text}”</span>{/if}
          {#if box.reason}<span class="reason">{box.reason}</span>{/if}
          {#if box.evidence !== null}
            <span class="evidence" class:strong={box.evidence >= TEXT_EVIDENCE_MIN}>
              {t(box.evidence >= TEXT_EVIDENCE_MIN ? 'slider.evidenceHigh' : 'slider.evidenceLow')}
            </span>
          {/if}
        </span>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .viewer { display: grid; gap: var(--space-3); }
  .tabs {
    display: inline-flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    justify-self: start;
    padding: var(--space-1);
    background: var(--white);
    border: var(--border-width) solid var(--border);
    border-radius: var(--radius-md);
  }
  .tabs button {
    min-height: 44px;
    padding: 0 var(--space-4);
    background: var(--white);
    color: var(--ink);
    border: 0;
    border-radius: 11px;
    font: inherit;
    font-weight: 700;
    font-size: var(--text-sm);
    cursor: pointer;
    transition:
      background-color var(--dur-fast) var(--ease-out),
      color var(--dur-fast) var(--ease-out),
      transform var(--dur-fast) var(--ease-out);
  }
  .tabs button:hover { background: var(--indigo-tint); }
  .tabs button[aria-selected='true']:hover { background: var(--brand); color: var(--on-brand); }
  .tabs button:active { transform: scale(0.97); }
  .tabs button[aria-selected='true'] { background: var(--brand); color: var(--on-brand); }
  .tabs button:focus-visible { outline: var(--focus-ring); outline-offset: var(--focus-offset); }

  .stage {
    position: relative;
    line-height: 0;
    border: var(--border-width) solid var(--border);
    border-radius: var(--radius-md);
    overflow: hidden;
    background: var(--paper);
  }
  .base { width: 100%; height: auto; display: block; }
  .fill { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: fill; }
  .overlay { position: absolute; inset: 0; }

  .boxes { position: absolute; inset: 0; pointer-events: none; }
  .box {
    position: absolute;
    border: 3px solid var(--coral);
    border-radius: 4px;
    box-shadow: 0 0 0 2px var(--ink);
  }
  .handle {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 4px;
    background: var(--ink);
    transform: translateX(-2px);
  }

  input[type='range'] { width: 100%; min-height: 44px; accent-color: var(--indigo); }
  input[type='range']:focus-visible { outline: var(--focus-ring); outline-offset: var(--focus-offset); }

  .anomalies {
    list-style: none;
    margin: var(--space-4) 0 0;
    padding: 0;
    display: grid;
    gap: var(--space-3);
  }
  .anomalies li { display: flex; align-items: flex-start; gap: var(--space-3); font-size: var(--text-base); }
  .anomaly-num {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    flex: none;
    background: var(--coral);
    border: 2px solid var(--border);
    border-radius: 50%;
    font-family: var(--font-display);
    font-weight: 800;
    font-size: var(--text-sm);
  }
  .quote { display: block; color: var(--body-2); }
  .reason { display: block; color: var(--muted); font-size: var(--text-sm); }
  .evidence { display: block; color: var(--muted); font-size: var(--text-sm); }
  .evidence.strong { color: var(--ink); font-weight: 700; }
</style>
