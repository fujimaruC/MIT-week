<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { AuditSession } from '$lib/audit.svelte';
  import { isMockMode } from '$lib/api/config';
  import { readSharedItem } from '$lib/share-target';
  import { readLink, type LinkProblem } from '$lib/link-input';
  import { takeHandedFile, imageProblem } from '$lib/handoff';
  import { EXAMPLES, openExample } from '$lib/demos';
  import { sessionHistory } from '$lib/session-history.svelte';
  import { statusLabel } from '$lib/format';
  import { t } from '$lib/i18n/index.svelte';
  import PromptBar from '$lib/components/ui/PromptBar.svelte';
  import Platforms from '$lib/components/ui/Platforms.svelte';
  import ScanningRadar from '$lib/components/deteksi/ScanningRadar.svelte';
  import Icon from '$lib/components/ui/Icon.svelte';
  import type { Status } from '$lib/api/types';

  const session = new AuditSession();

  let url = $state('');
  let problem = $state<LinkProblem | null>(null);
  let notice = $state<string | null>(null);
  let fileError = $state<string | null>(null);
  let dragging = $state(false);
  // What is being checked: shown under the input, kept to retry.
  type Sent = { kind: 'url'; url: string } | { kind: 'file'; file: File; preview: string };
  let sent = $state<Sent | null>(null);

  // A link arrives as ?q=, an image picked on another page in memory
  // (?from=prompt), a share as ?share=; each starts the check at once,
  // because sending it was the explicit action.
  onMount(() => {
    if (page.url.searchParams.get('share')) return;
    const handed = page.url.searchParams.get('from') === 'prompt' ? takeHandedFile() : null;
    if (handed) {
      void runFile(handed);
      return;
    }
    const q = page.url.searchParams.get('q')?.trim();
    if (q) void tick().then(() => runLink(q));
  });

  onMount(() => {
    const id = page.url.searchParams.get('share');
    if (id) void openShared(id);
  });

  // The contract parser (Zod) is not needed to paint this page; warm it idle.
  onMount(() => {
    const warm = () => void import('$lib/api/schemas').catch(() => {});
    const w = window as Window & { requestIdleCallback?: (cb: () => void, opts?: { timeout: number }) => number };
    if (w.requestIdleCallback) w.requestIdleCallback(warm, { timeout: 3000 });
    else setTimeout(warm, 300);
  });

  $effect(() => {
    const current = sent;
    return () => {
      if (current?.kind === 'file') URL.revokeObjectURL(current.preview);
    };
  });

  async function openShared(id: string): Promise<void> {
    notice = t('det.sharedOpening');
    const item = await readSharedItem(id);
    notice = null;
    const link = item ? readLink(item.url ?? item.text ?? '').url : null;
    if (!item || (!item.file && !link)) {
      notice = item ? t('link.err.notUrl') : t('det.sharedMissing');
      return;
    }
    if (item.file) await runFile(item.file);
    else if (link) await runLink(link);
  }

  async function runLink(raw: string): Promise<void> {
    if (session.running) return;
    const reading = readLink(raw);
    problem = reading.problem;
    if (!reading.url) return;
    notice = null;
    fileError = null;
    url = reading.url;
    sent = { kind: 'url', url: reading.url };
    await session.start('url', { url: reading.url });
    if (session.verdict) await goto(resolve('/hasil'));
  }

  async function runFile(file: File): Promise<void> {
    if (session.running) return;
    const bad = imageProblem(file);
    if (bad) {
      fileError = t(bad === 'unsupported' ? 'drop.unsupported' : 'drop.tooLarge');
      return;
    }
    fileError = null;
    notice = null;
    sent = { kind: 'file', file, preview: URL.createObjectURL(file) };
    await session.start('file', { file });
    if (session.verdict) await goto(resolve('/hasil'));
  }

  function retry(): void {
    if (!sent) return;
    if (sent.kind === 'url') void runLink(sent.url);
    else void runFile(sent.file);
  }

  function drop(e: DragEvent): void {
    e.preventDefault();
    dragging = false;
    const file = e.dataTransfer?.files?.[0];
    if (file) void runFile(file);
  }

  const tone = (s: Status): string => ({ SAFE: 'safe', SUSPICIOUS: 'warn', TAMPERED: 'danger', INSUFFICIENT: 'neutral' })[s];
</script>

<svelte:head>
  <title>{t('det.title')}</title>
  <meta name="description" content={t('det.description')} />
</svelte:head>

<div class="dash">
  <h1 class="title">{t('det.h1')}</h1>

  {#if isMockMode}
    <p class="notice" role="note"><strong>{t('det.previewTitle')}</strong> {t('det.previewBody')}</p>
  {/if}

  <section
    class="panel"
    class:over={dragging}
    aria-label={t('det.form')}
    ondragover={(e) => { e.preventDefault(); dragging = true; }}
    ondragleave={() => (dragging = false)}
    ondrop={drop}
  >
    <PromptBar
      id="url"
      bind:value={url}
      bind:problem
      busy={session.running}
      autofocus={!sent}
      photo="button"
      onlink={(link) => void runLink(link)}
      onfile={(file) => void runFile(file)}
      onstop={() => session.abort()}
    />
    <Platforms />
    {#if dragging}<p class="info" role="status">{t('app.dropping')}</p>{/if}
    {#if fileError}<p class="err" role="alert"><Icon name="alert" size={16} /> {fileError}</p>{/if}
    {#if notice}<p class="info" role="status">{notice}</p>{/if}

    {#if sent}
      <div class="current">
        {#if sent.kind === 'url'}
          <span class="cur-item"><Icon name="link" size={18} /><span class="cur-text" class:shimmer={session.running}>{sent.url}</span></span>
        {:else}
          <span class="cur-item">
            <span class="thumb" class:scanning={session.running}><img src={sent.preview} alt={t('chat.imageAlt')} /></span>
            <span class="cur-text">{sent.file.name}</span>
          </span>
        {/if}
        {#if session.running}
          <button class="btn btn--sm" type="button" onclick={() => session.abort()}><Icon name="stop" size={16} />{t('det.cancel')}</button>
        {/if}
      </div>
    {/if}

    {#if session.error}
      <div class="error-panel" role="alert">
        <p class="error-title"><Icon name="alert" size={18} /> {t('det.errTitle')}</p>
        <p>{session.error.message}</p>
        {#if session.error.retryAfter}<p class="muted">{t('det.wait', { n: session.error.retryAfter })}</p>{/if}
        <button class="btn btn--tonal btn--sm" type="button" onclick={retry}><Icon name="refresh" size={16} />{t('res.checkAgain')}</button>
      </div>
    {/if}
    <p class="sr-only" aria-live="polite">{session.announcement}</p>
  </section>

  {#if sent}
    <ScanningRadar stages={session.stages} partials={session.partials} running={session.running} />
  {:else}
    <div class="examples" role="group" aria-label={t('det.demos')}>
      <span class="ex-label">{t('det.demos')}</span>
      {#each EXAMPLES as ex (ex.id)}
        <button class="chip-btn" type="button" onclick={() => openExample(ex.id)}><Icon name={ex.icon} size={16} /><span>{t(ex.label)}</span></button>
      {/each}
    </div>
  {/if}

  {#if sessionHistory.list.length}
    <section class="sec" aria-labelledby="hist-title">
      <div class="sec-head">
        <h2 id="hist-title">{t('app.history')}</h2>
        <button class="btn btn--sm btn--quiet" type="button" onclick={() => sessionHistory.clear()}>{t('app.history.clear')}</button>
      </div>
      <table class="tbl">
        <thead>
          <tr>
            <th scope="col">{t('app.history.item')}</th>
            <th scope="col">{t('app.history.verdict')}</th>
            <th scope="col" class="num">{t('app.history.score')}</th>
            <th scope="col"><span class="sr-only">{t('app.history.recheck')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each sessionHistory.list as e (e.id)}
            <tr>
              <td class="item">
                {#if e.demo}<span class="tag">{t('app.history.demo')}</span>{/if}
                <span class="item-label">{e.label}</span>
              </td>
              <td><span class="vpill tone-{tone(e.status)}">{statusLabel(e.status)}</span></td>
              <td class="num tabular">{#if e.score === null}<span class="muted">—</span>{:else}{e.score}<small>/100</small>{/if}</td>
              <td class="act">
                {#if e.url}
                  <button class="btn btn--sm btn--quiet" type="button" disabled={session.running} aria-label={t('app.history.recheck')} title={t('app.history.recheck')} onclick={() => runLink(e.url)}><Icon name="refresh" size={18} /></button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </section>
  {/if}
</div>

<style>
  .dash { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--space-5); max-width: 52rem; margin-inline: auto; }
  .title { text-align: center; font-size: clamp(1.6rem, 1.3rem + 1.2vw, 2.2rem); font-weight: 500; letter-spacing: -0.02em; }
  .notice { margin: 0; padding: var(--space-3) var(--space-4); font-size: var(--text-sm); }

  .panel {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-2);
    min-width: 0;
    padding: var(--space-5);
    border: 1.5px dashed transparent;
    border-radius: var(--radius-lg);
    background: var(--card);
    transition: border-color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out);
  }
  .panel.over { border-color: var(--indigo); background: var(--indigo-tint); }
  .err { display: flex; gap: 6px; margin: 0; color: var(--err); font-size: var(--text-sm); font-weight: 500; }
  .info { margin: 0; color: var(--body-2); font-size: var(--text-sm); text-align: center; }

  .current {
    min-width: 0;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-soft);
  }
  .cur-item { display: inline-flex; align-items: center; gap: 8px; min-width: 0; flex: 1 1 16rem; }
  .cur-item :global(svg) { flex: none; color: var(--indigo-text); }
  .thumb { position: relative; display: block; width: 64px; height: 64px; flex: none; overflow: hidden; border-radius: 12px; }
  .thumb img { display: block; width: 100%; height: 100%; object-fit: cover; }
  /* While AMBA works, a beam passes over the picture (transform only). */
  .thumb.scanning::after {
    content: '';
    position: absolute;
    inset: 0;
    background: linear-gradient(180deg, transparent 20%, color-mix(in srgb, var(--brand) 60%, transparent) 50%, transparent 80%);
    animation: beam 1.5s var(--ease-in-out) infinite;
  }
  @media (prefers-reduced-motion: reduce) {
    .thumb.scanning::after { animation: none; opacity: 0.35; }
  }
  .cur-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--text-sm); }

  .error-panel { display: grid; gap: var(--space-2); justify-items: start; padding: var(--space-4); background: var(--coral-tint); border-radius: 12px; }
  .error-panel p { margin: 0; }
  .error-title { display: flex; align-items: center; gap: 6px; color: var(--err); font-weight: 500; }

  .examples { display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: var(--space-2); }
  .ex-label { color: var(--muted); font-size: var(--text-sm); margin-right: 4px; }
  .examples :global(svg) { color: var(--indigo-text); }

  .sec { display: grid; gap: var(--space-3); }
  .sec-head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
  .sec-head h2 { font-size: var(--text-base); font-weight: 500; }

  /* The session list: rules and alignment, no zebra, no header band. */
  .tbl { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
  .tbl th { padding: 0 12px 8px; color: var(--muted); font-weight: 400; font-size: var(--text-xs); text-align: left; }
  .tbl td { padding: 0 12px; height: 56px; border-top: 1px solid var(--border-soft); color: var(--body-2); }
  .tbl .num { text-align: right; }
  .tbl td.num { color: var(--ink); }
  .tbl small { color: var(--muted); margin-left: 1px; }
  .item { max-width: 0; width: 46%; }
  .item-label { display: inline-block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; vertical-align: middle; color: var(--ink); }
  .tag { display: inline-block; margin-right: 8px; padding: 1px 8px; border-radius: 999px; background: var(--surface-2); color: var(--body-2); font-size: var(--text-xs); vertical-align: middle; }
  .vpill { display: inline-block; padding: 3px 10px; border-radius: 999px; font-size: var(--text-xs); font-weight: 500; }
  .tone-safe { background: var(--safe); color: var(--on-fill); }
  .tone-warn { background: var(--yellow); color: var(--on-fill); }
  .tone-danger { background: var(--coral); color: var(--on-fill); }
  .tone-neutral { background: var(--surface-2); color: var(--ink); }
  .act { text-align: right; white-space: nowrap; }

  @media (max-width: 40rem) {
    .panel { padding: var(--space-4); }
    .item { width: auto; }
    .tbl td, .tbl th { padding-inline: 6px; }
  }
</style>
