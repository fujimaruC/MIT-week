<script lang="ts">
  import { t, type MessageKey } from '$lib/i18n/index.svelte';
  import type { DossierParsed } from '$lib/api/schemas';

  interface Props {
    video: NonNullable<DossierParsed['video']>;
  }

  let { video }: Props = $props();

  const disclosure = $derived(video.creator_disclosure ?? { level: 'none' as const, phrase: '' });
  const showCreator = $derived(disclosure.level !== 'none' && disclosure.phrase !== '');
  const items = $derived((video.web_evidence ?? []).slice(0, 5));
</script>

{#if showCreator || items.length}
  <div class="prov" data-info-key="video-provenance">
    <h3 class="label">{t('video.prov.title')}</h3>
    {#if showCreator}
      <p class="creator" data-level={disclosure.level} lang="id">
        {t(`video.prov.creator.${disclosure.level}` as MessageKey, { phrase: disclosure.phrase })}
      </p>
    {/if}
    {#if items.length}
      <p class="sub">{t('video.prov.web')}</p>
      <ul class="web">
        {#each items as item (item.url)}
          <li>
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external publisher URL, not an app route -->
            <a href={item.url} target="_blank" rel="noopener noreferrer nofollow" aria-label="{item.title} ({t('video.prov.open')})"
              >{item.title}</a
            >
            <span class="meta">{item.publisher} · {t(`video.prov.says.${item.says}` as MessageKey)}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}

<style>
  .prov { display: grid; gap: var(--space-2); }
  .label { margin: 0; font-size: var(--text-sm); font-weight: 700; color: var(--ink); }
  .creator, .sub { margin: 0; font-size: var(--text-sm); color: var(--ink); }
  .creator[data-level='weak'], .sub { color: var(--muted); }
  .web { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--space-2); }
  .web li { display: grid; gap: 2px; }
  .web a { display: inline-flex; align-items: center; min-height: 44px; font-weight: 600; overflow-wrap: anywhere; }
  .meta { font-size: var(--text-sm); color: var(--muted); }
</style>
