<script lang="ts">
  import { t } from '$lib/i18n/index.svelte';
  import { platformName } from '$lib/format';
  import type { SourceMetadata } from '$lib/api/types';

  interface Props {
    metadata: SourceMetadata;
  }

  let { metadata }: Props = $props();

  const platform = $derived(platformName(metadata.platform) || t('lp.web'));
</script>

{#if metadata.status === 'OK'}
  <div class="preview">
    <p class="meta">
      <span class="platform">{platform}</span>
      {#if metadata.site_name && metadata.site_name !== platform}<span>· {metadata.site_name}</span>{/if}
      {#if metadata.author}<span>· {metadata.author}</span>{/if}
    </p>
    {#if metadata.title}<p class="title">{metadata.title}</p>{/if}
    {#if metadata.description && metadata.description !== metadata.title}
      <p class="desc">{metadata.description}</p>
    {/if}
    <p class="note">{t('lp.note')}</p>
  </div>
{:else}
  <!-- No preview: a plain statement of what the result rests on, not an error (Sir, 10 Nov). -->
  <div class="preview">
    <p class="meta"><span class="platform">{platform}</span></p>
    <p class="desc">
      {metadata.status === 'BLOCKED' ? t('lp.blocked', { platform }) : t('lp.unavailable')}
      {metadata.status === 'BLOCKED' ? t('lp.blockedBody') : t('lp.unavailableBody')}
    </p>
  </div>
{/if}

<style>
  .preview { display: grid; gap: var(--space-2); }
  .meta { margin: 0; display: flex; flex-wrap: wrap; gap: 6px; font-size: var(--text-sm); color: var(--muted); }
  .platform { font-weight: 700; color: var(--ink); }
  .title { margin: 0; font-weight: 700; font-size: var(--text-lg); line-height: 1.35; overflow-wrap: anywhere; }
  .desc { margin: 0; color: var(--body-2); max-width: var(--measure); overflow-wrap: anywhere; white-space: pre-line; }
  .note { margin: var(--space-2) 0 0; font-size: var(--text-sm); color: var(--muted); }
</style>
