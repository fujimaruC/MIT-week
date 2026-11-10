<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { t } from '$lib/i18n/index.svelte';
  import type { LinkProblem } from '$lib/link-input';
  import { inview, lines } from '$lib/motion';
  import { handOver } from '$lib/handoff';
  import { EXAMPLES, openExample } from '$lib/demos';
  import { keyReasons } from '$lib/reasons';
  import Icon from '$lib/components/ui/Icon.svelte';
  import PromptBar from '$lib/components/ui/PromptBar.svelte';
  import Platforms from '$lib/components/ui/Platforms.svelte';
  import StatusBadge from '$lib/components/ui/StatusBadge.svelte';
  import FlowDiagram from '$lib/components/home/FlowDiagram.svelte';
  import RiskMap from '$lib/components/hasil/RiskMap.svelte';
  import demoData from '$lib/data/demo-cases.json';
  import type { DossierParsed } from '$lib/api/schemas';

  let url = $state('');
  let problem = $state<LinkProblem | null>(null);

  // The check runs on the dashboard: a link as ?q=, an image in memory.
  function onlink(link: string): void {
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() is applied to the route; the query string is appended
    void goto(`${resolve('/deteksi')}?q=${encodeURIComponent(link)}`);
  }
  function onfile(file: File): void {
    handOver(file);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() is applied to the route; the query string is appended
    void goto(`${resolve('/deteksi')}?from=prompt`);
  }

  // The example is a bundled demo result (labelled as such), drawn with the
  // result page's own components. The fixture is checked against the
  // contract in the tests, so it is used here without loading the parser.
  const SAMPLE_ID = 'demo-berita-ig';
  const sample = demoData.cases.find((c) => c.id === SAMPLE_ID);
  const dossier = sample?.dossier as unknown as DossierParsed | undefined;
  const reasons = dossier ? keyReasons(dossier).slice(0, 2) : [];
</script>

<svelte:head>
  <title>{t('home.title')}</title>
  <meta name="description" content={t('home.description')} />
</svelte:head>

<!-- ============ hero: the check itself ============ -->
<section class="hero" use:inview>
  <div class="hero-inner">
    <h1 class="hero-title">
      {#each lines(t('land.h1')) as line, i (i)}
        <span class="rise" class:gradient-text={i === 1} style:--i={i}>{line}</span>
      {/each}
    </h1>
    <p class="hero-lede rise" style:--i="2">{t('land.lede')}</p>
    <div class="hero-check rise" style:--i="3">
      <PromptBar id="hero-url" bind:value={url} bind:problem {onlink} {onfile} photo="button" />
    </div>
    <div class="rise" style:--i="4"><Platforms /></div>
  </div>
  <svg class="wave" viewBox="0 0 1440 420" preserveAspectRatio="none" aria-hidden="true" focusable="false">
    <defs>
      <linearGradient id="wg" x1="0" x2="1" y1="0" y2="0">
        <stop offset="0" stop-color="#c6613f" stop-opacity="0" />
        <stop offset="0.25" stop-color="#c6613f" />
        <stop offset="0.55" stop-color="#d97757" />
        <stop offset="0.8" stop-color="#e8b088" />
        <stop offset="1" stop-color="#e8b088" stop-opacity="0" />
      </linearGradient>
    </defs>
    <g class="wave-glow">
      <path d="M0 300 C 240 200, 420 380, 720 260 S 1180 120, 1440 240" />
    </g>
    <g class="wave-line">
      <path d="M0 300 C 240 200, 420 380, 720 260 S 1180 120, 1440 240" />
      <path d="M0 330 C 260 250, 460 400, 760 300 S 1200 180, 1440 290" />
    </g>
  </svg>
</section>

<!-- ============ how it works, drawn ============ -->
<section class="sec" id="cara-kerja" aria-labelledby="how-title" use:inview>
  <div class="wrap wrap--wide">
    <h2 id="how-title" class="sec-title rise">{t('land.how.title')}</h2>
    <div class="rise" style:--i="1"><FlowDiagram /></div>
  </div>
</section>

<!-- ============ an example result ============ -->
{#if dossier && sample?.image}
  <section class="sec" aria-labelledby="ex-title" use:inview>
    <div class="wrap wrap--wide">
      <h2 id="ex-title" class="sec-title rise">{t('land.ex.title')}</h2>
      <div class="ex rise" style:--i="1">
        <figure class="ex-media">
          <img src={sample.image} alt={t('home.sample.alt')} width="360" height="450" loading="lazy" decoding="async" />
          <figcaption>{t('land.ex.caption')}</figcaption>
        </figure>
        <div class="ex-body">
          <StatusBadge status={dossier.verdict.status} score={dossier.verdict.score} />
          <ul class="why">
            {#each reasons as reason (reason)}
              <li><span class="dot" aria-hidden="true"></span>{reason}</li>
            {/each}
          </ul>
          <RiskMap {dossier} />
        </div>
      </div>
      <div class="ex-more rise" style:--i="2" role="group" aria-label={t('det.demos')}>
        <span>{t('det.demos')}</span>
        {#each EXAMPLES as ex (ex.id)}
          <button class="chip-btn" type="button" onclick={() => openExample(ex.id)}><Icon name={ex.icon} size={16} /><span>{t(ex.label)}</span></button>
        {/each}
      </div>
    </div>
  </section>
{/if}

<!-- ============ learn: what an edit looks like to AMBA ============ -->
<section class="sec" aria-labelledby="learn-title" use:inview>
  <div class="wrap wrap--wide">
    <div class="learn rise">
      <div class="learn-pics">
        <figure>
          <img src="/edukasi/quiz-diedit.jpg" alt={t('land.learn.altPhoto')} width="360" height="480" loading="lazy" decoding="async" />
          <figcaption>{t('land.learn.photo')}</figcaption>
        </figure>
        <span class="learn-arrow" aria-hidden="true"><Icon name="arrow-right" size={22} /></span>
        <figure>
          <img src="/edukasi/quiz-diedit-ela.jpg" alt={t('land.learn.altEla')} width="360" height="480" loading="lazy" decoding="async" />
          <figcaption>{t('land.learn.ela')}</figcaption>
        </figure>
      </div>
      <div class="learn-copy">
        <h2 id="learn-title">{t('land.learn.title')}</h2>
        <p>{t('land.learn.lede')}</p>
        <a class="btn btn--primary" href={resolve('/edukasi')}><Icon name="book" size={18} />{t('land.learn.cta')}</a>
      </div>
    </div>
  </div>
</section>

<style>
  /* ---------- hero ---------- */
  /* The glow behind the hero now comes from the page-wide aurora. */
  .hero { position: relative; isolation: isolate; overflow: hidden; padding: clamp(3rem, 9vw, 6.5rem) 0 0; }
  .hero-inner { display: grid; justify-items: center; gap: var(--space-5); width: min(100%, 56rem); margin-inline: auto; padding-inline: var(--gutter); text-align: center; }
  .hero-title {
    display: grid;
    margin: 0;
    font-size: clamp(2.5rem, 1.4rem + 4.6vw, 4.6rem);
    font-weight: 500;
    line-height: 1.04;
    letter-spacing: -0.03em;
  }
  .hero-lede { margin: 0; max-width: 34rem; color: var(--body-2); font-size: var(--text-lg); line-height: 1.5; }
  .hero-check { width: min(100%, 44rem); text-align: left; }
  .wave { display: block; width: 100%; height: clamp(7rem, 18vw, 15rem); margin-top: -1rem; pointer-events: none; }
  .wave path { fill: none; stroke: url(#wg); }
  .wave-glow { filter: blur(22px); opacity: 0.5; }
  .wave-glow path { stroke-width: 46; }
  .wave-line path { stroke-width: 1.6; opacity: 0.85; }
  .wave-line path:nth-child(2) { opacity: 0.45; }
  .wave g { animation: drift 16s var(--ease-in-out) infinite alternate; transform-origin: center; }
  @keyframes drift {
    from { transform: translateX(-2%) scaleY(1); }
    to { transform: translateX(2%) scaleY(1.12); }
  }

  /* ---------- sections ---------- */
  .sec { padding-block: clamp(2rem, 5vw, 3.5rem); }
  .sec-title {
    margin: 0 0 var(--space-6);
    text-align: center;
    font-size: clamp(1.75rem, 1.3rem + 1.6vw, 2.5rem);
    font-weight: 500;
    letter-spacing: -0.02em;
  }

  /* ---------- the example ---------- */
  .ex {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-5);
    padding: clamp(16px, 3vw, 28px);
    border-radius: var(--radius-xl);
    background: var(--card);
  }
  .ex-media { margin: 0; display: grid; gap: 8px; justify-items: center; }
  .ex-media img { display: block; width: 100%; max-width: 20rem; height: auto; border-radius: var(--radius-md); }
  .ex-media figcaption { color: var(--muted); font-size: var(--text-xs); }
  .ex-body { display: grid; align-content: start; gap: var(--space-4); min-width: 0; }
  .ex-body :global(.badge) { justify-self: start; }
  .why { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; color: var(--ink); font-size: var(--text-base); line-height: 1.5; }
  .why li { display: flex; gap: 10px; align-items: baseline; }
  .why .dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--err); transform: translateY(-1px); }
  .ex-more { display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 8px 10px; margin-top: var(--space-5); color: var(--muted); font-size: var(--text-sm); }

  /* ---------- learn ---------- */
  .learn {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-5);
    align-items: center;
    padding: clamp(16px, 3vw, 28px);
    border-radius: var(--radius-xl);
    background: var(--card);
  }
  .learn-pics { display: flex; align-items: center; justify-content: center; gap: 10px; }
  .learn-pics figure { margin: 0; display: grid; gap: 6px; justify-items: center; flex: 1 1 0; max-width: 11rem; }
  .learn-pics img { display: block; width: 100%; height: auto; border-radius: var(--radius-sm); }
  .learn-pics figcaption { color: var(--muted); font-size: var(--text-xs); }
  .learn-arrow { color: var(--muted); display: grid; }
  .learn-copy { display: grid; gap: var(--space-3); justify-items: start; }
  .learn-copy h2 { font-size: clamp(1.5rem, 1.2rem + 1.2vw, 2rem); font-weight: 500; }
  .learn-copy p { margin: 0; color: var(--body-2); font-size: var(--text-lg); }

  @media (min-width: 52rem) {
    .ex { grid-template-columns: minmax(0, 20rem) minmax(0, 1fr); align-items: start; }
    .learn { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
  }
  @media (prefers-reduced-motion: reduce) {
    .wave g { animation: none; }
  }
</style>
