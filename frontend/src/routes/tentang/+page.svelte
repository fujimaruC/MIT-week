<script lang="ts">
  import Icon from '$lib/components/ui/Icon.svelte';
  import { t } from '$lib/i18n/index.svelte';

  const PRINCIPLES = ['about.p1', 'about.p2', 'about.p3', 'about.p4'] as const;

  // Order set by Sir (10 Nov 2026): Bagus, Jeri, Ridho, Mursyid.
  const TEAM = [
    { id: 'bagus', name: 'Bagus Joshua P.', role: 'Golang Backend Lead' },
    { id: 'jeri', name: 'Jericho M. T.', role: 'Frontend Lead' },
    { id: 'ridho', name: 'M. Ridho A.', role: 'UI/UX Lead' },
    { id: 'mursyid', name: 'Mursyid D. W.', role: 'PM & Lead Architect' }
  ] as const;

  const LIMITS = ['about.l1', 'about.l2', 'about.l3', 'about.l4', 'about.l5'] as const;
</script>

<svelte:head>
  <title>{t('about.title')}</title>
  <meta name="description" content={t('about.description')} />
</svelte:head>

<section class="head">
  <h1 class="h1">AMBA</h1>
  <p class="lede">{t('about.lede')}</p>
</section>

<section class="team" aria-labelledby="team-title">
  <h2 class="h2" id="team-title">{t('about.teamTitle')}</h2>
  <p class="lede team-lede">{t('about.teamLede')}</p>
  <ul class="team-grid">
    {#each TEAM as m (m.id)}
      <li class="member">
        <div class="photo">
          <img src={`/tim/${m.id}.webp`} alt={m.name} width="300" height="375" loading="lazy" decoding="async" />
        </div>
        <div class="who"><strong>{m.name}</strong><span>{m.role}</span></div>
      </li>
    {/each}
  </ul>
</section>

<section class="data card lift" aria-labelledby="data-title">
  <h2 class="h2" id="data-title">{t('about.dataTitle')}</h2>
  <dl>
    <div class="row">
      <dt><span class="tag tag--send">{t('about.sent')}</span></dt>
      <dd>{t('about.sentBody')}</dd>
    </div>
    <div class="row">
      <dt><span class="tag tag--no">{t('about.notSent')}</span></dt>
      <dd>{t('about.notSentBody')}</dd>
    </div>
    <div class="row">
      <dt><span class="tag tag--none">{t('about.stored')}</span></dt>
      <dd>{t('about.storedBody')}</dd>
    </div>
  </dl>
</section>

<div class="two-col">
  <section class="card limits lift" aria-labelledby="limits-title">
    <h2 class="h2" id="limits-title">{t('about.limitsTitle')}</h2>
    <ul>
      {#each LIMITS as item (item)}
        <li>
          <span class="bullet" aria-hidden="true"><Icon name="alert" size={16} /></span>
          {t(item)}
        </li>
      {/each}
    </ul>
  </section>

  <section class="card principles lift" aria-labelledby="principles-title">
    <h2 class="h2" id="principles-title">{t('about.principlesTitle')}</h2>
    <ul>
      {#each PRINCIPLES as item (item)}
        <li>
          <span class="bullet bullet--ok" aria-hidden="true"><Icon name="check" size={16} /></span>
          {t(item)}
        </li>
      {/each}
    </ul>
  </section>
</div>

<section class="card legal lift" aria-labelledby="legal-title">
  <h2 class="h2" id="legal-title">{t('about.legal')}</h2>
  <p>{t('disclaimer.full')}</p>
</section>

<p class="disclaimer">{t('about.disclaimerShort')}</p>

<style>
  .head { max-width: 54rem; margin-bottom: var(--space-8); }
  h1 { margin-bottom: var(--space-3); }

  .team { margin-bottom: var(--space-8); }
  .team-lede { margin-top: var(--space-3); }
  .head h1 { font-size: clamp(2.25rem, 1.6rem + 2.6vw, 3.5rem); font-weight: 500; letter-spacing: -0.02em; }
  .team-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-4);
    margin: var(--space-6) 0 0;
    padding: 0;
    list-style: none;
  }
  @media (min-width: 48rem) { .team-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--space-5); } }
  .member {
    display: grid;
    align-content: start;
    overflow: hidden;
    padding: 0;
    gap: 0;
    background: var(--white);
    border: var(--border-width) solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-hard-xs);
    transition: transform var(--dur-base) var(--ease-out), box-shadow var(--dur-base) var(--ease-out);
  }
  .photo {
    /* Taller than the picture and bottom-aligned: the head keeps clear space above it (Sir, seq 1118). */
    aspect-ratio: 4 / 5.6;
    display: flex;
    align-items: flex-end;
    justify-content: center;
    overflow: hidden;
    background:
      radial-gradient(120% 90% at 50% 0%, color-mix(in srgb, var(--brand) 38%, transparent), transparent 70%),
      var(--indigo-soft);
  }
  .photo img { width: 100%; height: auto; flex: none; display: block; transition: transform var(--dur-slow, 400ms) var(--ease-out); }
  .who { display: grid; gap: var(--space-1); padding: var(--space-4) var(--space-3) var(--space-5); text-align: center; text-wrap: balance; }
  .who strong { font-family: var(--font-display); font-size: var(--text-base); font-weight: 600; }
  .who span { color: var(--body-2); font-size: var(--text-sm); }
  @media (hover: hover) and (prefers-reduced-motion: no-preference) {
    .member:hover { transform: translateY(-4px); }
    .member:hover .photo img { transform: scale(1.04); }
  }

  .data { padding: clamp(1.25rem, 3vw, 2.25rem); margin-bottom: var(--space-2); }
  dl { display: grid; gap: var(--space-5); margin: var(--space-5) 0 0; }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: var(--space-3) var(--space-5);
    padding-top: var(--space-4);
    border-top: var(--border-width) dashed var(--border);
  }
  .row:first-child { padding-top: 0; border-top: 0; }
  dt { flex: 0 0 auto; }
  dd { flex: 1 1 22rem; margin: 0; min-width: 0; max-width: var(--measure); }
  .tag {
    display: inline-flex;
    padding: 4px 14px;
    border: var(--border-width) solid var(--border);
    border-radius: var(--radius-pill);
    font-weight: 700;
    font-size: var(--text-sm);
    white-space: nowrap;
  }
  .tag--send { background: var(--yellow); }
  .tag--no { background: var(--coral); }
  .tag--none { background: var(--safe); }

  .two-col { display: grid; grid-template-columns: 1fr; gap: var(--space-5); margin-top: var(--space-6); }
  @media (min-width: 48rem) { .two-col { grid-template-columns: repeat(2, 1fr); } }
  .two-col > * { min-width: 0; padding: clamp(1.25rem, 3vw, 1.9rem); display: flex; flex-direction: column; }
  .limits { background: var(--yellow); }
  .principles { background: var(--indigo-soft); }
  ul { margin: var(--space-4) 0 0; padding: 0; list-style: none; display: grid; gap: var(--space-3); }
  li { display: flex; align-items: flex-start; gap: var(--space-3); line-height: 1.45; }
  .bullet {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    flex: none;
    background: var(--white);
    border: 2px solid var(--border);
    border-radius: 50%;
  }
  .bullet--ok { background: var(--safe); }

  .legal { margin-top: var(--space-5); padding: clamp(1.25rem, 3vw, 1.9rem); }
  .legal p { margin: var(--space-3) 0 0; max-width: 80ch; color: var(--body-2); line-height: 1.65; }

  .disclaimer {
    margin-top: var(--space-6);
    padding: var(--space-4) var(--space-5);
    background: var(--white);
    border: var(--border-width) dashed var(--border);
    border-radius: var(--radius-md);
    color: var(--body-2);
    font-size: var(--text-sm);
    max-width: 52rem;
  }
</style>
