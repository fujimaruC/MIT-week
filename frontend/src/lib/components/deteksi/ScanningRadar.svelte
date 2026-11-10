<script lang="ts">
  import { t } from '$lib/i18n/index.svelte';
  import { pillarSummary } from '$lib/pillar-summary';
  import type { PillarKey, StageStatus } from '$lib/audit.svelte';
  import Icon, { type IconName } from '$lib/components/ui/Icon.svelte';

  interface Props {
    stages: Record<PillarKey, StageStatus>;
    /** Pillar payloads already received, rendered under their own cell. */
    partials?: Partial<Record<PillarKey, unknown>>;
    running?: boolean;
  }

  let { stages, partials = {}, running = false }: Props = $props();

  // The four checks as one strip split by hairlines, each with the icon the
  // home page's diagram uses, and its state live while a check runs.
  const order: PillarKey[] = ['ai', 'ela', 'fact', 'link'];
  const ICON: Record<PillarKey, IconName> = { ai: 'sparkle', ela: 'type', fact: 'search', link: 'lock' };
  const doneCount = $derived(order.filter((k) => stages[k] === 'done').length);
  const stateText = (s: StageStatus): string => (s === 'idle' ? t('app.modules.idle') : t(`stage.${s}`));
</script>

<section class="station" aria-labelledby="steps-title">
  <h2 id="steps-title" class="sr-only">{t('radar.title')}</h2>
  {#if running}<p class="summary tabular" aria-live="polite">{t('radar.summary', { done: doneCount, total: order.length })}</p>{/if}
  <ol class="strip">
    {#each order as key (key)}
      <li class="cell {stages[key]}">
        <p class="name"><span class="ic" aria-hidden="true"><Icon name={ICON[key]} size={18} /></span>{t(`pillar.${key}`)}</p>
        <p class="state">
          <span class="mark" aria-hidden="true">
            {#if stages[key] === 'done'}<Icon name="check" size={12} />{/if}
          </span>
          {stateText(stages[key])}
        </p>
        {#if pillarSummary(key, partials[key])}
          <p class="detail">{pillarSummary(key, partials[key])}</p>
        {/if}
        <span class="bar" aria-hidden="true"><span></span></span>
      </li>
    {/each}
  </ol>
  {#if running}<p class="wait">{t('det.waitNote')}</p>{/if}
</section>

<style>
  .station { display: grid; gap: var(--space-3); }
  .summary { margin: 0; font-size: var(--text-sm); color: var(--body-2); }
  .strip {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    margin: 0;
    padding: 0;
    list-style: none;
    border: 1px solid var(--border-soft);
    border-radius: 16px;
    overflow: hidden;
  }
  .cell {
    position: relative;
    display: grid;
    align-content: start;
    gap: 6px;
    padding: 16px 18px 20px;
    border-left: 1px solid var(--border-soft);
  }
  .cell:first-child { border-left: 0; }
  .name { display: flex; align-items: center; gap: 10px; margin: 0; font-weight: 500; }
  .ic { display: grid; place-items: center; width: 32px; height: 32px; flex: none; border-radius: 50%; background: var(--indigo-soft); color: var(--indigo-text); }
  .state { display: flex; align-items: center; gap: 8px; margin: 6px 0 0; color: var(--muted); font-size: var(--text-sm); }
  .mark {
    display: grid;
    place-items: center;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 1.5px solid var(--border);
  }
  .running .state { color: var(--indigo-text); }
  .running .mark { border-color: var(--indigo); border-right-color: transparent; animation: spin 0.9s linear infinite; }
  .done .state { color: var(--ink); }
  .done .mark { background: var(--indigo); border-color: var(--indigo); color: var(--on-indigo-text); }
  .detail { margin: 0; color: var(--body-2); font-size: var(--text-sm); }
  /* A thin progress line along the cell's foot while it works. */
  .bar { position: absolute; left: 0; right: 0; bottom: 0; height: 2px; overflow: hidden; }
  .bar span { position: absolute; inset: 0 auto 0 0; width: 0; background: var(--indigo); }
  .running .bar span { width: 35%; animation: slide 1.2s var(--ease-in-out) infinite; }
  .done .bar span { width: 100%; }
  .wait { margin: 0; font-size: var(--text-sm); color: var(--muted); }

  @keyframes spin { to { transform: rotate(360deg); } }
  @keyframes slide { from { transform: translateX(-100%); } to { transform: translateX(300%); } }
  @media (prefers-reduced-motion: reduce) {
    .running .mark { animation: none; border-right-color: var(--indigo); }
    .running .bar span { animation: none; width: 50%; }
  }
  @media (max-width: 56rem) {
    .strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .cell:nth-child(3) { border-left: 0; }
    .cell:nth-child(n + 3) { border-top: 1px solid var(--border-soft); }
  }
  @media (max-width: 30rem) {
    .strip { grid-template-columns: minmax(0, 1fr); }
    .cell { border-left: 0; }
    .cell:nth-child(n + 2) { border-top: 1px solid var(--border-soft); }
  }
</style>
