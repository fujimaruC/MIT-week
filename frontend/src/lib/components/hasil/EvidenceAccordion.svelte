<script lang="ts">
  import { t } from '$lib/i18n/index.svelte';
  import type { DossierParsed } from '$lib/api/schemas';
  import Skeleton from '$lib/components/ui/Skeleton.svelte';
  import Icon from '$lib/components/ui/Icon.svelte';
  import { linkReason, caveat, reliabilityNote, aiLines } from '$lib/i18n/verdict';

  interface Props {
    pillars: DossierParsed['pillars'];
    /** URL input: the AI card judged the link's preview image. */
    inputType?: DossierParsed['input_type'];
    /** The AI card judged the frames of an analysed social video instead. */
    fromVideo?: boolean;
    pending?: boolean;
  }

  let { pillars, inputType = 'FILE', fromVideo = false, pending = false }: Props = $props();

  const LABELS = {
    ela: 'ev.ela',
    ai: 'pillar.ai',
    fact: 'pillar.fact',
    link: 'pillar.link'
  } as const;

  type Key = keyof typeof LABELS;
  const ORDER: Key[] = ['ela', 'ai', 'fact', 'link'];

  const FACT_LABELS = {
    CONTRADICTS: 'ev.fact.CONTRADICTS',
    SUPPORTS: 'ev.fact.SUPPORTS',
    UNVERIFIED: 'ev.fact.UNVERIFIED'
  } as const;

  const available = (k: Key) => pillars[k].available;
  // A check that did not run is not shown at all: an empty card helps nobody.
  // While results stream in (pending) every card stays, as a loading skeleton.
  const order = $derived(pending ? ORDER : ORDER.filter((k) => available(k)));

  /**
   * Fact-check list: only the articles that decided the status, at most five,
   * each one a real link. Same-topic articles that settle nothing read as
   * evidence beside a clear verdict (a SAFE result listing "[SALAH] ..."
   * headlines about other claims), so UNVERIFIED shows none.
   */
  const MAX_SOURCES = 5;
  const isWebUrl = (u: string): boolean => /^https?:\/\//i.test(u);
  const factSources = $derived.by(() => {
    if (pillars.fact.status === 'UNVERIFIED') return [];
    const all = (pillars.fact.sources ?? []).filter((s) => isWebUrl(s.url));
    const deciding = all.filter((s) => s.match === pillars.fact.status);
    return (deciding.length ? deciding : all).slice(0, MAX_SOURCES);
  });

  // Marked text whose compression differs from the image's other text
  // (forensics TextEvidence >= scoring.ELAEvidenceMin).
  const pastedCount = $derived(pillars.ela.anomalies.filter((a) => (a.evidence ?? -1) >= 50).length);

  // scoring.ELAEvidenceMin: below it image analysis is reported as checked
  // but is not evidence either way, so the card says so.
  const ELA_EVIDENCE_MIN = 50;

  // The pillar union has no shared `score`; narrow by key instead of casting.
  function pillarScore(p: DossierParsed['pillars'], k: Key): number {
    switch (k) {
      case 'ela':
        return p.ela.score;
      case 'ai':
        return p.ai.score;
      case 'fact':
        return p.fact.status === 'CONTRADICTS' ? 100 : p.fact.status === 'UNVERIFIED' ? 40 : 0;
      case 'link':
        return p.link.score;
    }
  }
</script>

<div class="accordion">
  {#each order as key (key)}
    <details class="item" data-testid={`pillar-${key}`} open={available(key)}>
      <summary>
        <span class="name">{t(LABELS[key])}</span>
        {#if pending}
          <span class="state pending">{t('ev.checking')}</span>
        {:else if key === 'fact'}
          <span class="state done">{t(FACT_LABELS[pillars.fact.status])}</span>
        {:else}
          <span class="state done">{t('ev.score', { n: pillarScore(pillars, key) })}</span>
        {/if}
        <span class="chev" aria-hidden="true"><Icon name="chevron" size={18} /></span>
      </summary>

      <div class="body">
        {#if pending}
          <Skeleton lines={2} />
        {:else if key === 'ela'}
          <p>
            {t('ev.reliability', { n: Math.round(pillars.ela.reliability * 100) })}
            {#if pillars.ela.reliability_note}{reliabilityNote(pillars.ela.reliability_note)}{/if}
          </p>
          {#if pillars.ela.score < ELA_EVIDENCE_MIN}<p>{t('ev.elaInconclusive')}</p>{/if}
          {#if pillars.ela.anomalies.length}
            <p>{t('ev.areas', { n: pillars.ela.anomalies.length })}</p>
            {#if pastedCount > 0}<p>{t('ev.textEvidence', { n: pastedCount })}</p>{/if}
          {:else}
            <p>{t('ev.noAreas')}</p>
          {/if}
        {:else if key === 'ai'}
          {#each aiLines(pillars.ai, inputType, fromVideo) as line (line)}<p>{line}</p>{/each}
          <p class="note">{caveat(pillars.ai.caveat ?? '')}</p>
          {#if (pillars.ai.indicators ?? []).length}
            <ul>
              {#each pillars.ai.indicators ?? [] as indicator (indicator)}<li>{indicator}</li>{/each}
            </ul>
          {/if}
        {:else if key === 'fact'}
          <div data-testid="sources">
            {#if pillars.fact.claim}<p>{t('ev.claim', { claim: pillars.fact.claim })}</p>{/if}
            <p>{t('ev.status', { status: t(FACT_LABELS[pillars.fact.status]) })}</p>
            {#if factSources.length}
              <ul class="sources">
                {#each factSources as s (s.url)}
                  <li>
                    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external publisher URL, not an app route -->
                    <a href={s.url} rel="noopener noreferrer nofollow" target="_blank">{s.title || s.publisher}</a>
                    <span class="muted">{s.publisher}</span>
                  </li>
                {/each}
              </ul>
            {:else}
              <p class="note">{t('ev.noSources')}</p>
            {/if}
          </div>
        {:else if key === 'link'}
          {#if pillars.link.final_url}<p class="url">{t('ev.finalUrl', { url: pillars.link.final_url })}</p>{/if}
          {#if (pillars.link.reasons ?? []).length}
            <ul>
              {#each pillars.link.reasons ?? [] as r (r.code)}<li>{linkReason(r.text)}</li>{/each}
            </ul>
          {:else}
            <p>{t('ev.noReasons')}</p>
          {/if}
        {/if}
      </div>
    </details>
  {/each}
</div>

<style>
  .accordion { display: grid; gap: var(--space-3); }
  .item {
    border: var(--border-width) solid var(--border);
    border-radius: var(--radius-md);
    background: var(--paper);
    overflow: hidden;
    transition: background-color var(--dur-fast) var(--ease-out);
  }
  .item:hover { background: var(--indigo-tint); }
  .item[open] { background: var(--white); }
  summary {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-height: 52px;
    padding: var(--space-3) var(--space-4);
    cursor: pointer;
    list-style: none;
    transition: background-color var(--dur-fast) var(--ease-out);
  }
  summary:hover { background: var(--hover-overlay); }
  summary::-webkit-details-marker { display: none; }
  summary:focus-visible { outline: var(--focus-ring); outline-offset: -6px; }
  .name { font-family: var(--font-display); font-weight: 800; font-size: 1.05em; }
  .state {
    margin-left: auto;
    padding: 2px 10px;
    border: 2px solid var(--border);
    border-radius: var(--radius-pill);
    font-size: var(--text-sm);
    font-weight: 700;
  }
  .state.done { background: var(--safe); color: var(--on-fill); }
  
  .state.pending { background: var(--yellow); color: var(--on-fill); }
  .chev {
    display: inline-flex;
    color: var(--ink);
    transition: transform var(--dur-fast) var(--ease-out);
  }
  details[open] .chev { transform: rotate(180deg); }
  .body {
    padding: 0 var(--space-4) var(--space-4);
    color: var(--body-2);
    font-size: var(--text-base);
  }
  .body p { margin: 0 0 var(--space-2); }
  .body ul { margin: 0; padding-left: var(--space-5); display: grid; gap: var(--space-1); }
  .note { color: var(--muted); }
  .muted { color: var(--muted); }
  .sources { list-style: none; padding-left: 0 !important; gap: var(--space-2) !important; }
  .sources li { display: grid; }
  .sources a { display: inline-flex; align-items: center; min-height: 44px; overflow-wrap: anywhere; font-weight: 700; }
</style>
