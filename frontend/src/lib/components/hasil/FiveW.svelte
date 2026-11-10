<script lang="ts">
  import { t, type MessageKey } from '$lib/i18n/index.svelte';
  import Icon, { type IconName } from '$lib/components/ui/Icon.svelte';
  import type { VideoW5H1, VideoResearch } from '$lib/api/schemas';

  /**
   * What the video shows, as 5W1H (Sir, 10 Nov 2026: it replaces the old
   * "context match"), and the trusted sources the web search found for it,
   * so the conclusion rests on data the family can open and read.
   */
  interface Props {
    w5h1: VideoW5H1 | null;
    research: VideoResearch | null;
  }
  let { w5h1, research }: Props = $props();

  const ROWS: Array<{ key: keyof VideoW5H1; label: MessageKey; icon: IconName }> = [
    { key: 'what', label: 'w5.what', icon: 'info' },
    { key: 'who', label: 'w5.who', icon: 'user' },
    { key: 'when', label: 'w5.when', icon: 'clock' },
    { key: 'where', label: 'w5.where', icon: 'pin' },
    { key: 'why', label: 'w5.why', icon: 'help' },
    { key: 'how', label: 'w5.how', icon: 'steps' }
  ];

  // Only web links are shown as links; anything else stays plain text.
  const safeUrl = (u: string): string | null => (/^https?:\/\//i.test(u) ? u : null);
  const initial = (domain: string): string => (domain.replace(/^www\./, '')[0] ?? '?').toUpperCase();
  function day(value: string): string {
    if (!value) return '';
    const d = new Date(value);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' });
  }
  const sources = $derived(research?.sources ?? []);
</script>

<section class="w5" aria-labelledby="w5-title">
  <header class="head">
    <h2 id="w5-title">{t('w5.title')}</h2>
    <p class="note">{t('w5.note')}</p>
  </header>

  {#if w5h1}
    <dl class="grid">
      {#each ROWS as r, i (r.key)}
        <div class="cell rise" style:--i={i}>
          <dt><span class="ic" aria-hidden="true"><Icon name={r.icon} size={18} /></span>{t(r.label)}</dt>
          <dd class:empty={!w5h1[r.key]}>{w5h1[r.key] || t('w5.unknown')}</dd>
        </div>
      {/each}
    </dl>
  {/if}

  {#if research}
    <div class="trail">
      <h3>{t('trail.title')}</h3>
      {#if research.query}<p class="query">{t('trail.query', { q: research.query })}</p>{/if}
      {#if sources.length}
        <ol class="sources">
          {#each sources as s, i (s.url + i)}
            <li class="src rise" style:--i={i + 3}>
              <span class="dom" aria-hidden="true">{initial(s.domain)}</span>
              <div class="src-body">
                {#if safeUrl(s.url)}
                  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- an external source, not an app route -->
                  <a href={safeUrl(s.url)} target="_blank" rel="noopener noreferrer">{s.title || s.domain}<span class="sr-only"> ({t('trail.newTab')})</span></a>
                {:else}
                  <span class="src-title">{s.title || s.domain}</span>
                {/if}
                <p class="meta">{s.domain}{day(s.published) ? ` · ${day(s.published)}` : ''}</p>
                {#if s.snippet}<p class="snip">{s.snippet}</p>{/if}
              </div>
            </li>
          {/each}
        </ol>
      {:else}
        <p class="empty-trail">{research.status === 'UNAVAILABLE' ? t('trail.unavailable') : t('trail.none')}</p>
      {/if}
    </div>
  {/if}
</section>

<style>
  .w5 { display: grid; gap: var(--space-4); }
  .head { display: grid; gap: 4px; }
  h2 { font-size: var(--text-base); font-weight: 500; }
  .note { margin: 0; color: var(--muted); font-size: var(--text-sm); }

  /* Six cells: what the video shows, one question each. */
  .grid { display: grid; grid-template-columns: minmax(0, 1fr); gap: 8px; margin: 0; }
  @media (min-width: 36rem) { .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (min-width: 60rem) { .grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
  .cell { display: grid; align-content: start; gap: 6px; min-width: 0; padding: 14px 16px; border-radius: var(--radius-md); background: var(--white); }
  dt { display: flex; align-items: center; gap: 8px; color: var(--muted); font-size: var(--text-sm); font-weight: 500; }
  .ic { display: grid; place-items: center; width: 30px; height: 30px; flex: none; border-radius: 50%; background: var(--indigo-soft); color: var(--indigo-text); }
  dd { margin: 0; color: var(--ink); line-height: 1.45; overflow-wrap: anywhere; }
  dd.empty { color: var(--muted); font-size: var(--text-sm); }

  .trail { display: grid; gap: var(--space-3); }
  h3 { font-size: var(--text-base); font-weight: 500; }
  .query { margin: 0; color: var(--muted); font-size: var(--text-sm); overflow-wrap: anywhere; }
  .sources { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
  .src { display: flex; gap: 12px; align-items: flex-start; min-width: 0; padding: 12px 14px; border-radius: var(--radius-md); background: var(--white); }
  .dom { display: grid; place-items: center; width: 36px; height: 36px; flex: none; border-radius: 10px; background: var(--surface-2); color: var(--ink); font-weight: 600; }
  .src-body { display: grid; gap: 2px; min-width: 0; }
  .src a, .src-title { color: var(--ink); font-weight: 500; line-height: 1.4; overflow-wrap: anywhere; }
  .src a { text-decoration-color: color-mix(in srgb, var(--ink) 35%, transparent); text-underline-offset: 3px; }
  .src a:hover { color: var(--indigo-text); }
  .meta { margin: 0; color: var(--muted); font-size: var(--text-xs); }
  .snip {
    margin: 4px 0 0;
    color: var(--body-2);
    font-size: var(--text-sm);
    line-height: 1.5;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .empty-trail { margin: 0; padding: 12px 14px; border-radius: var(--radius-md); background: var(--white); color: var(--body-2); font-size: var(--text-sm); }
</style>
