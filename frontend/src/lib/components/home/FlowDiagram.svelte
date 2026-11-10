<script lang="ts">
  import { t, type MessageKey } from '$lib/i18n/index.svelte';
  import Icon, { type IconName } from '$lib/components/ui/Icon.svelte';
  import StatusBadge from '$lib/components/ui/StatusBadge.svelte';
  import type { Status } from '$lib/api/types';

  /**
   * How AMBA works, as one diagram instead of three paragraphs: what you
   * send, the four checks that run on it, and the three verdicts, in the
   * same words and colours the result page uses.
   */
  const INPUTS: Array<{ icon: IconName; label: MessageKey }> = [
    { icon: 'link', label: 'flow.in.link' },
    { icon: 'image', label: 'flow.in.photo' }
  ];
  const CHECKS: Array<{ icon: IconName; label: MessageKey }> = [
    { icon: 'sparkle', label: 'flow.mod.ai' },
    { icon: 'type', label: 'flow.mod.edit' },
    { icon: 'search', label: 'flow.mod.fact' },
    { icon: 'lock', label: 'flow.mod.link' }
  ];
  const VERDICTS: Status[] = ['SAFE', 'SUSPICIOUS', 'TAMPERED'];
</script>

<ol class="flow" aria-label={t('flow.label')}>
  <li class="col">
    <p class="col-title">{t('flow.send')}</p>
    <ul class="nodes">
      {#each INPUTS as n (n.label)}
        <li class="node"><span class="ic"><Icon name={n.icon} size={20} /></span>{t(n.label)}</li>
      {/each}
    </ul>
  </li>
  <li class="arrow" aria-hidden="true"><span class="dot"></span></li>
  <li class="col">
    <p class="col-title">{t('flow.check')}</p>
    <ul class="nodes">
      {#each CHECKS as n, i (n.label)}
        <li class="node" style:--i={i}><span class="ic"><Icon name={n.icon} size={20} /></span>{t(n.label)}</li>
      {/each}
    </ul>
  </li>
  <li class="arrow" aria-hidden="true"><span class="dot"></span></li>
  <li class="col">
    <p class="col-title">{t('flow.verdict')}</p>
    <ul class="nodes nodes--out">
      {#each VERDICTS as s (s)}
        <li><StatusBadge status={s} /></li>
      {/each}
    </ul>
  </li>
</ol>

<style>
  .flow {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 10px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .col {
    display: grid;
    align-content: start;
    gap: 12px;
    padding: 18px;
    border-radius: var(--radius-lg);
    background: var(--card);
  }
  .col-title { margin: 0; color: var(--muted); font-size: var(--text-sm); font-weight: 500; }
  .nodes { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
  .node {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 52px;
    padding: 0 16px 0 8px;
    border-radius: 999px;
    background: var(--white);
    color: var(--ink);
    font-size: var(--text-base);
    font-weight: 500;
  }
  .ic {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    flex: none;
    border-radius: 50%;
    background: var(--indigo-soft);
    color: var(--indigo-text);
  }
  /* The four checks light up in turn, as they do while AMBA works. Opacity
   * only, on a ring of its own, so nothing repaints. */
  .col:nth-child(3) .node { position: relative; }
  .col:nth-child(3) .node::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: inherit;
    box-shadow: inset 0 0 0 1.5px var(--indigo);
    opacity: 0;
    animation: ping 4.4s var(--ease-in-out) infinite;
    animation-delay: calc(var(--i, 0) * 0.55s + 0.4s);
  }
  @keyframes ping {
    0%, 28%, 100% { opacity: 0; }
    9% { opacity: 1; }
  }
  .nodes--out { gap: 10px; }
  .nodes--out :global(.badge) { width: 100%; }

  /* The arrows, with a dot that carries the item through. */
  .arrow {
    position: relative;
    justify-self: center;
    width: 2px;
    height: 40px;
    background: linear-gradient(var(--border-strong) 50%, transparent 0) 0 0 / 2px 8px;
  }
  .arrow::after {
    content: '';
    position: absolute;
    left: 50%;
    bottom: -1px;
    width: 10px;
    height: 10px;
    border-right: 2px solid var(--border-strong);
    border-bottom: 2px solid var(--border-strong);
    transform: translateX(-50%) rotate(45deg);
  }
  .dot {
    position: absolute;
    top: 0;
    left: 50%;
    width: 10px;
    height: 10px;
    margin-left: -5px;
    border-radius: 50%;
    background: var(--brand);
    animation: down 2.2s var(--ease-in-out) infinite;
  }
  @keyframes down {
    0% { transform: translateY(0); opacity: 0; }
    20%, 75% { opacity: 1; }
    100% { transform: translateY(30px); opacity: 0; }
  }
  @keyframes across {
    0% { transform: translateX(0); opacity: 0; }
    20%, 75% { opacity: 1; }
    100% { transform: translateX(46px); opacity: 0; }
  }

  @media (min-width: 52rem) {
    .flow {
      grid-template-columns: minmax(0, 0.85fr) 64px minmax(0, 1.05fr) 64px minmax(0, 1.25fr);
      align-items: center;
    }
    .arrow {
      width: 100%;
      height: 2px;
      background: linear-gradient(90deg, var(--border-strong) 50%, transparent 0) 0 0 / 8px 2px;
    }
    .arrow::after {
      left: auto;
      right: 0;
      bottom: auto;
      top: 50%;
      transform: translateY(-50%) rotate(-45deg);
    }
    .dot { top: 50%; left: 0; margin: -5px 0 0; animation-name: across; }
  }

  @media (prefers-reduced-motion: reduce) {
    .dot { animation: none; opacity: 0; }
    .col:nth-child(3) .node::after { animation: none; }
  }
</style>
