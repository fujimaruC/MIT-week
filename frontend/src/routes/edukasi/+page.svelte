<script lang="ts">
  import { t } from '$lib/i18n/index.svelte';
  import Icon, { type IconName } from '$lib/components/ui/Icon.svelte';
  import Quiz from '$lib/components/belajar/Quiz.svelte';
  import forensicQuiz from '$lib/data/forensic-quiz.json';
  import { AI_CUES, HOAX_CUES, CHECK_STEPS, UI, type Text } from '$lib/data/belajar';

  const tx = (text: Text): string => text.id;

  // Each sign is one short line; its explanation opens on tap (Sir, 10 Nov
  // 2026: as little text on screen as possible).
  const GROUPS: Array<{ id: string; icon: IconName; title: Text; items: Array<{ id: string; title: Text; body: Text }> }> = [
    { id: 'video-ai', icon: 'sparkle', title: UI.ai.title, items: AI_CUES },
    { id: 'video-hoaks', icon: 'alert', title: UI.hoax.title, items: HOAX_CUES }
  ];

  // ---- ELA practice (PRD F7): three synthetic images with the heat map AMBA computed. ----
  type Guess = 'asli' | 'diedit' | 'belum-pasti';
  const GUESSES: Guess[] = ['asli', 'diedit', 'belum-pasti'];
  const forensic = forensicQuiz.items as Array<{ id: string; image: string; ela: string; answer: Guess }>;
  let guesses = $state<Record<string, Guess | undefined>>({});
  let revealed = $state<Record<string, boolean>>({});

</script>

<svelte:head>
  <title>{tx(UI.title)}</title>
  <meta name="description" content={tx(UI.description)} />
</svelte:head>

<div class="belajar">
  <header class="head">
    <h1 class="h1">{tx(UI.h1)}</h1>
    <p class="lede">{tx(UI.lede)}</p>
  </header>

  {#each GROUPS as g (g.id)}
    <section id={g.id} aria-labelledby={`${g.id}-title`}>
      <h2 class="h2" id={`${g.id}-title`}><span class="h2-ic" aria-hidden="true"><Icon name={g.icon} size={20} /></span>{tx(g.title)}</h2>
      <ul class="cues">
        {#each g.items as c (c.id)}
          <li>
            <details class="cue">
              <summary><span>{tx(c.title)}</span><Icon name="chevron" size={18} /></summary>
              <p>{tx(c.body)}</p>
            </details>
          </li>
        {/each}
      </ul>
    </section>
  {/each}

  <section class="forensic" aria-labelledby="forensic-title">
    <h2 class="h2" id="forensic-title"><span class="h2-ic" aria-hidden="true"><Icon name="image" size={20} /></span>{t('fq.title')}</h2>
    <div class="forensic-grid">
      {#each forensic as item, i (item.id)}
        <article class="f-item">
          <img class="f-img" src={item.image} alt={t('fq.imageAlt', { n: i + 1 })} width="360" height="480" loading="lazy" />
          <div class="f-body">
            <p class="f-legend" id={`fq-${item.id}`}>{t('fq.question')}</p>
            <div class="f-options" role="radiogroup" aria-labelledby={`fq-${item.id}`}>
              {#each GUESSES as g (g)}
                <label class="option">
                  <input type="radio" name={`fq-${item.id}`} value={g} bind:group={guesses[item.id]} />
                  <span>{t(`fq.opt.${g}`)}</span>
                </label>
              {/each}
            </div>
            <button
              class="btn btn--tonal btn--sm"
              type="button"
              aria-expanded={!!revealed[item.id]}
              aria-controls={`fq-panel-${item.id}`}
              disabled={!guesses[item.id]}
              onclick={() => (revealed = { ...revealed, [item.id]: !revealed[item.id] })}
            >
              <Icon name="image" size={16} /> {revealed[item.id] ? t('fq.hide') : t('fq.reveal')}
            </button>
            {#if revealed[item.id]}
              <div class="f-reveal" id={`fq-panel-${item.id}`}>
                <figure>
                  <img src={item.ela} alt={t('fq.elaAlt', { n: i + 1 })} width="360" height="480" />
                  <figcaption>{t('fq.ela')}</figcaption>
                </figure>
                <p class="f-verdict" role="status">
                  {guesses[item.id] === item.answer ? t('fq.right') : t('fq.wrong', { answer: t(`fq.opt.${item.answer}`) })}
                </p>
                <p class="f-explain">{t(`fq.explain.${item.answer}`)}</p>
              </div>
            {/if}
          </div>
        </article>
      {/each}
    </div>
  </section>

  <section id="kuis" aria-labelledby="quiz-title">
    <h2 class="h2" id="quiz-title"><span class="h2-ic" aria-hidden="true"><Icon name="check" size={20} /></span>{tx(UI.quiz.title)}</h2>
    <Quiz />
  </section>

  <section id="cek-sendiri" aria-labelledby="check-title">
    <h2 class="h2" id="check-title"><span class="h2-ic" aria-hidden="true"><Icon name="search" size={20} /></span>{tx(UI.check.title)}</h2>
    <ul class="cues">
      {#each CHECK_STEPS as step (step.id)}
        <li>
          <details class="cue">
            <summary><span>{tx(step.title)}</span><Icon name="chevron" size={18} /></summary>
            <p>{tx(step.body)}</p>
            {#if step.link}
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external site, not an app route -->
              <a class="ext" href={step.link.href} target="_blank" rel="noopener noreferrer">
                {step.link.label}<Icon name="arrow-up-right" size={16} />
                <span class="sr-only"> ({tx(UI.check.newTab)})</span>
              </a>
            {/if}
          </details>
        </li>
      {/each}
    </ul>
  </section>
</div>

<style>
  .belajar { display: grid; gap: var(--space-7); }
  .head { display: grid; gap: var(--space-3); max-width: 54rem; }
  .head .lede { margin: 0; }
  section { scroll-margin-top: 6rem; }
  .h2 { display: flex; align-items: center; gap: 12px; margin: 0 0 var(--space-4); }
  .h2-ic {
    display: grid;
    place-items: center;
    width: 40px;
    height: 40px;
    flex: none;
    border-radius: 50%;
    background: var(--indigo-soft);
    color: var(--indigo-text);
  }

  /* One short line per sign; the explanation opens under it. */
  .cues { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; grid-template-columns: minmax(0, 1fr); }
  @media (min-width: 40rem) { .cues { grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; } }
  @media (min-width: 64rem) { .cues { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
  .cue { border-radius: var(--radius-md); background: var(--card); }
  .cue summary {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    min-height: 56px;
    padding: 0 var(--space-4);
    font-weight: 500;
    list-style: none;
    cursor: pointer;
  }
  .cue summary::-webkit-details-marker { display: none; }
  .cue summary:hover { background: var(--surface-2); border-radius: var(--radius-md); }
  .cue summary :global(svg) { flex: none; color: var(--muted); transition: transform var(--dur-base) var(--ease-out); }
  .cue[open] summary :global(svg) { transform: rotate(180deg); }
  .cue p { margin: 0; padding: 0 var(--space-4) var(--space-4); color: var(--body-2); line-height: 1.55; }
  .ext { display: inline-flex; align-items: center; gap: 6px; min-height: 44px; margin: 0 var(--space-4) var(--space-3); font-weight: 500; color: var(--indigo-text); }

  /* ELA practice */
  .forensic-grid { display: grid; gap: var(--space-4); grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr)); }
  .f-item { display: grid; gap: var(--space-3); align-content: start; padding: var(--space-4); border-radius: var(--radius-lg); background: var(--card); }
  .f-img, .f-reveal img { width: 100%; height: auto; border-radius: var(--radius-sm); display: block; }
  .f-body, .f-reveal { display: grid; gap: var(--space-3); }
  .f-body .f-legend { margin: 0; font-weight: 500; color: var(--ink); }
  .f-options { display: grid; gap: var(--space-2); }
  .option {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-height: 48px;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    cursor: pointer;
  }
  .option:hover { background: var(--indigo-tint); }
  .option:has(input:checked) { border-color: var(--brand); box-shadow: inset 0 0 0 1px var(--brand); }
  .option input { width: 22px; height: 22px; accent-color: var(--brand); flex: none; }
  .f-reveal figure { margin: 0; }
  .f-reveal figcaption { font-size: var(--text-sm); color: var(--muted); margin-top: var(--space-1); }
  .f-reveal p { margin: 0; }
  .f-reveal .f-verdict { font-weight: 500; color: var(--ink); }


</style>
