<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { t } from '$lib/i18n/index.svelte';
  import Icon from '$lib/components/ui/Icon.svelte';

  /**
   * Every error page leads home on its own (Sir, 10 Nov 2026). The visitor can
   * stop the countdown (WCAG 2.2.1, timing adjustable) or leave at once.
   */
  const SECONDS = 6;
  let left = $state(SECONDS);
  let stopped = $state(false);
  let timer: ReturnType<typeof setInterval> | undefined;

  const notFound = $derived(page.status === 404);

  function stop(): void {
    stopped = true;
    clearInterval(timer);
  }

  onMount(() => {
    const started = Date.now();
    timer = setInterval(() => {
      left = Math.max(0, SECONDS - Math.floor((Date.now() - started) / 1000));
      if (left === 0) {
        clearInterval(timer);
        void goto(resolve('/'), { replaceState: true });
      }
    }, 200);
    return () => clearInterval(timer);
  });
</script>

<svelte:head><title>{t('error.title404')}</title></svelte:head>

<section class="error-stage">
  <div class="error-pane glass" style:--seconds="{SECONDS}s">
    <h1 class="h1">{notFound ? t('error.h404') : t('error.hOther')}</h1>
    <p class="lede">{notFound ? t('error.lede404') : t('error.ledeOther')}</p>
    <p class="code"><Icon name="alert" size={16} /> {t('error.code', { code: page.status })}</p>

    <div class="countdown">
      <p class="count tabular" aria-hidden="true">
        {stopped ? t('error.stopped') : t('error.countdown', { n: left })}
      </p>
      <!-- Announced once, not every second. -->
      <p class="sr-only" role="status">
        {stopped ? t('error.stopped') : t('error.countdown', { n: SECONDS })}
      </p>
      {#if !stopped}<span class="track" aria-hidden="true"><span class="fill"></span></span>{/if}
    </div>

    <div class="actions">
      <a class="btn btn--primary" href={resolve('/')}>{t('error.home')}</a>
      {#if !stopped}
        <button class="btn btn--quiet" type="button" onclick={stop}>{t('error.stay')}</button>
      {:else}
        <a class="btn btn--quiet" href={resolve('/deteksi')}>{t('error.check')}</a>
      {/if}
    </div>
  </div>
</section>

<style>
  .error-stage {
    display: grid;
    place-items: center;
    min-height: min(70dvh, 40rem);
    padding-block: var(--space-6);
  }
  .error-pane {
    width: min(100%, 36rem);
    padding: clamp(1.5rem, 5vw, 2.75rem);
  }
  .code {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    margin: var(--space-3) 0 0;
    color: var(--muted);
    font-size: var(--text-sm);
    font-weight: 700;
  }
  h1 { margin: 0 0 var(--space-3); }
  .lede { margin: 0; }

  .countdown {
    margin: var(--space-6) 0 var(--space-5);
    display: grid;
    gap: var(--space-2);
  }
  .count { margin: 0; color: var(--body-2); font-size: var(--text-sm); }
  .track {
    display: block;
    height: 4px;
    border-radius: 999px;
    background: color-mix(in srgb, var(--ink) 10%, transparent);
    overflow: hidden;
  }
  /* Time left, drawn at a constant rate: the one place linear motion is right. */
  .fill {
    display: block;
    height: 100%;
    border-radius: inherit;
    background: var(--brand);
    transform-origin: left;
    animation: deplete var(--seconds) linear forwards;
  }
  @keyframes deplete {
    from { transform: scaleX(1); }
    to { transform: scaleX(0); }
  }
  @media (prefers-reduced-motion: reduce) {
    .fill { animation: none; transform: scaleX(1); opacity: 0.5; }
  }

  .actions { display: flex; flex-wrap: wrap; gap: var(--space-3); }
  @media (max-width: 40rem) {
    .actions .btn { width: 100%; }
  }
</style>
