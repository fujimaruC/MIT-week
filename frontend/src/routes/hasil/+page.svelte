<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { result } from '$lib/result.svelte';
  import { formatScore, platformName, formatClock } from '$lib/format';
  import StatusBadge from '$lib/components/ui/StatusBadge.svelte';
  import Panel from '$lib/components/ui/Panel.svelte';
  import EvidenceAccordion from '$lib/components/hasil/EvidenceAccordion.svelte';
  import HighlightedText from '$lib/components/hasil/HighlightedText.svelte';
  import ReadAloud from '$lib/components/a11y/ReadAloud.svelte';
  import ElaViewer from '$lib/components/hasil/ElaViewer.svelte';
  import LinkPreview from '$lib/components/hasil/LinkPreview.svelte';
  import VideoAnalysis from '$lib/components/hasil/VideoAnalysis.svelte';
  import RiskMap from '$lib/components/hasil/RiskMap.svelte';
  import FiveW from '$lib/components/hasil/FiveW.svelte';
  import { t } from '$lib/i18n/index.svelte';
  import { simpleSentence, explanation, basisList } from '$lib/i18n/verdict';
  import { keyReasons } from '$lib/reasons';
  import { inview } from '$lib/motion';
  import { sessionHistory } from '$lib/session-history.svelte';
  import tacticLabelsId from '$lib/data/tactics.json';
  import Icon from '$lib/components/ui/Icon.svelte';
  import type { DossierParsed } from '$lib/api/schemas';

  const dossier: DossierParsed | null = result.value;
  const imageFile: File | null = result.imageFile;
  let verdictHeading: HTMLElement | null = $state(null);

  // The session's list on the dashboard gains this check (memory only).
  onMount(() => {
    if (dossier) sessionHistory.add(dossier, imageFile?.name ?? null);
  });

  // Move focus to the verdict once it exists, so a screen reader lands on the
  // answer instead of staying on whatever was focused during the scan.
  $effect(() => {
    if (dossier && verdictHeading) verdictHeading.focus({ preventScroll: true });
  });

  const elaAnomalies = $derived(dossier && dossier.pillars.ela.available ? dossier.pillars.ela.anomalies : []);

  // The text tactic phrases were verified against: the link's caption, or
  // what the model read in the image.
  const highlight = $derived.by(() => {
    if (!dossier) return { text: '', label: '' };
    if (dossier.input_type === 'FILE') return { text: dossier.ocr_text, label: t('res.ocrLabel') };
    if (dossier.input_type === 'URL' && dossier.source_metadata) {
      const md = dossier.source_metadata;
      const caption = [md.title, md.description !== md.title ? md.description : ''].filter(Boolean).join('\n');
      return { text: caption, label: t('res.captionLabel') };
    }
    return { text: '', label: '' };
  });

  const sentence = $derived(dossier ? simpleSentence(dossier) : '');
  const explained = $derived(dossier ? explanation(dossier) : '');
  const basis = $derived(dossier ? basisList(dossier) : '');
  const showExplained = $derived(dossier?.verdict.status === 'INSUFFICIENT' && explained !== '');
  const reasons = $derived(dossier ? keyReasons(dossier) : []);
  const tone = $derived(
    dossier
      ? ({ SAFE: 'safe', SUSPICIOUS: 'warn', TAMPERED: 'danger', INSUFFICIENT: 'neutral' } as const)[dossier.verdict.status]
      : 'neutral'
  );
  const showLinkPreview = $derived(
    !!dossier &&
      dossier.input_type === 'URL' &&
      !!dossier.source_metadata &&
      !(dossier.source_metadata.status === 'BLOCKED' && dossier.video?.status === 'OK')
  );
  const video = $derived(dossier?.video?.status === 'OK' ? dossier.video : null);

  // The item as it was checked: the uploaded image or the video's first frame.
  // Nothing external is loaded (CSP: self, data, blob).
  let imageUrl = $state<string | null>(null);
  $effect(() => {
    if (!imageFile) return;
    const objectUrl = URL.createObjectURL(imageFile);
    imageUrl = objectUrl;
    return () => URL.revokeObjectURL(objectUrl);
  });
  const picture = $derived.by(() => {
    if (imageUrl && dossier?.input_type === 'FILE') return imageUrl;
    const frame = dossier?.video?.frames?.[0]?.thumb ?? '';
    return /^(data:image\/|blob:|\/)/.test(frame) ? frame : null;
  });

  // The caption names the item: the post's title, its site, or the upload.
  const itemName = $derived.by(() => {
    if (!dossier) return '';
    if (dossier.input_type === 'FILE') return imageFile?.name || t('res.item.image');
    const md = dossier.source_metadata;
    if (md?.title) return md.title;
    try {
      return new URL(dossier.source_reference).hostname.replace(/^www\./, '');
    } catch {
      return dossier.source_reference;
    }
  });
  const meta = $derived.by(() => {
    if (!dossier) return '';
    const parts: string[] = [];
    parts.push(t(dossier.input_type === 'FILE' ? 'app.type.image' : video ? 'app.type.video' : 'app.type.link'));
    if (video?.platform) parts.push(platformName(video.platform));
    else if (dossier.source_metadata?.site_name) parts.push(dossier.source_metadata.site_name);
    if (video?.duration_sec) parts.push(formatClock(video.duration_sec));
    return parts.join(' · ');
  });
</script>

<svelte:head><title>{t('res.title')}</title></svelte:head>

{#if !dossier}
  <section class="state">
    <h1 class="title">{t('res.goneH1')}</h1>
    <p class="lede">{t('res.goneLede')}</p>
    <a class="btn btn--primary" href={resolve('/deteksi')}>{t('res.checkAgain')}</a>
  </section>
{:else}
  <div class="res tone-{tone}" use:inview>
    {#if dossier.is_demo}<p class="notice notice--demo" role="note">{t('res.demoNotice')}</p>{/if}
    {#if dossier.degraded}<p class="notice" role="note">{t('res.degraded')}</p>{/if}

    <!-- The answer first: the item, its verdict, one sentence, the score and why. -->
    <section class="verdict rise" style:--i="0" aria-labelledby="verdict-title">
      <figure class="media">
        {#if picture}
          <img src={picture} alt={t('chat.imageAlt')} />
        {:else}
          <div class="media-empty"><Icon name={dossier.input_type === 'URL' ? 'link' : 'image'} size={32} /></div>
        {/if}
        <figcaption><span class="item">{itemName}</span>{#if meta}<span>{meta}</span>{/if}</figcaption>
      </figure>
      <div class="v-body">
        <h1 id="verdict-title" class="verdict-heading" tabindex="-1" bind:this={verdictHeading} data-testid="verdict-badge">
          <StatusBadge status={dossier.verdict.status} />
        </h1>
        <p class="sentence" data-testid="verdict-sentence">{sentence}</p>
        {#if dossier.verdict.score !== null}
          <div class="score-row">
            <span class="score tabular">{dossier.verdict.score}<small>/100</small></span>
            <div class="meter" role="img" aria-label={formatScore(dossier.verdict.score)}>
              <span class="meter-fill" style:width="{dossier.verdict.score}%"></span>
            </div>
          </div>
          <p class="score-cap">{t('res.scoreCap')}</p>
        {/if}
        <ul class="why" aria-label={t('res.why')}>
          {#each reasons as reason, i (reason)}
            <li class="rise" style:--i={i + 2}><span class="dot" aria-hidden="true"></span><span>{reason}</span></li>
          {/each}
        </ul>
        <div class="actions">
          <ReadAloud text={`${sentence} ${explained}`} />
          <a class="btn btn--primary" href={resolve('/deteksi')}><Icon name="plus" size={18} />{t('res.checkAgain')}</a>
        </div>
      </div>
    </section>

    {#if video && (video.w5h1 || video.research)}
      <section class="card rise" style:--i="1">
        <FiveW w5h1={video.w5h1} research={video.research} />
      </section>
    {/if}

    <section class="card rise" style:--i="1">
      <RiskMap {dossier} />
    </section>

    {#if showLinkPreview && dossier.source_metadata}
      <section class="card rise" style:--i="2" data-testid="source-metadata">
        <h2 class="k-label">{t('res.panelLink')}</h2>
        <LinkPreview metadata={dossier.source_metadata} />
      </section>
    {/if}

    {#if dossier.video && (dossier.video.status !== 'UNAVAILABLE' || dossier.video.platform_ai_label)}
      <VideoAnalysis video={dossier.video} ai={dossier.pillars.ai} />
    {/if}
    {#if imageFile && dossier.input_type === 'FILE'}
      <Panel title={t('res.panelImage')}>
        <ElaViewer file={imageFile} anomalies={elaAnomalies} />
      </Panel>
    {/if}

    <!-- The written detail stays one tap away. -->
    <details class="more">
      <summary><span>{t('res.more')}</span><Icon name="chevron" size={18} /></summary>
      <div class="evidence">
        <Panel title={t('res.panelDetails')}>
          <div class="details">
            {#if dossier.tactics.length > 0}
              <div class="block">
                <h3 class="label">{t('res.panelTactics')}</h3>
                <HighlightedText text={highlight.text} sourceLabel={highlight.label} tactics={dossier.tactics} labels={tacticLabelsId} />
              </div>
            {/if}
            <EvidenceAccordion pillars={dossier.pillars} inputType={dossier.input_type} fromVideo={dossier.video?.status === 'OK'} />
          </div>
        </Panel>
        <Panel title={t('res.panelLimits')} tone="soft">
          {#if showExplained}<p class="explained">{explained}</p>{/if}
          {#if basis}<p class="basis"><strong>{t('res.basis')}:</strong> {basis}.</p>{/if}
          <p class="note">{t('res.limitsNote')}</p>
        </Panel>
      </div>
    </details>
  </div>
{/if}

<style>
  .state { display: grid; gap: var(--space-4); justify-items: start; max-width: 40rem; margin-inline: auto; padding-block: var(--space-6); }
  .res { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--space-4); max-width: 64rem; margin-inline: auto; }
  .tone-safe { --tone: var(--ok); }
  .tone-warn { --tone: var(--warn); }
  .tone-danger { --tone: var(--err); }
  .tone-neutral { --tone: var(--ink); }
  .notice { margin: 0; padding: var(--space-3) var(--space-4); font-size: var(--text-sm); font-weight: 500; }

  .verdict {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-5);
    padding: clamp(16px, 3vw, 28px);
    border-radius: var(--radius-xl);
    background: var(--card);
  }
  .media { display: grid; gap: var(--space-2); margin: 0; min-width: 0; }
  .media img { display: block; width: 100%; max-height: 22rem; object-fit: contain; border-radius: 16px; background: var(--sheet); }
  .media-empty { display: grid; place-items: center; min-height: 8rem; border-radius: 16px; background: var(--surface-2); color: var(--muted); }
  .media figcaption { display: grid; gap: 2px; color: var(--muted); font-size: var(--text-xs); }
  .media .item { color: var(--body-2); font-size: var(--text-sm); overflow-wrap: anywhere; }
  .v-body { display: grid; align-content: start; gap: var(--space-4); min-width: 0; }
  .verdict-heading { margin: 0; font-size: inherit; line-height: inherit; }
  .verdict-heading :global(.badge) { font-size: var(--text-lg); padding: 10px 18px; animation: pop 600ms var(--ease-out) 120ms both; transform-origin: left center; }
  .verdict-heading:focus { outline: none; }
  .verdict-heading:focus-visible { outline: var(--focus-ring); outline-offset: 4px; border-radius: 12px; }
  .sentence { margin: 0; max-width: 30ch; font-size: clamp(1.35rem, 1.1rem + 1vw, 1.9rem); font-weight: 400; line-height: 1.3; }
  .score-row { display: flex; align-items: center; gap: var(--space-4); }
  .score { flex: none; color: var(--tone); font-size: 2rem; line-height: 1; }
  .score small { margin-left: 2px; color: var(--muted); font-size: var(--text-sm); }
  .meter { flex: 1; --meter-color: var(--tone); }
  .meter-fill { display: block; height: 100%; transform-origin: left center; }
  .score-cap { margin: calc(var(--space-2) * -1) 0 0; color: var(--muted); font-size: var(--text-xs); }
  .why { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--space-2); }
  .why li { display: flex; gap: var(--space-3); align-items: baseline; color: var(--ink); line-height: 1.5; }
  .dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--tone); transform: translateY(-1px); }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }

  .card { margin: 0; padding: var(--space-5); background: var(--card); border-radius: var(--radius-lg); min-width: 0; }
  .k-label { margin: 0 0 var(--space-3); color: var(--muted); font-size: var(--text-sm); font-weight: 500; }

  .more summary {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    min-height: 56px;
    padding: 0 var(--space-5);
    border-radius: var(--radius-lg);
    background: var(--card);
    font-weight: 500;
    list-style: none;
    cursor: pointer;
  }
  .more summary::-webkit-details-marker { display: none; }
  .more summary:hover { background: var(--surface-2); }
  .more summary :global(svg) { transition: transform var(--dur-base) var(--ease-out); }
  .more[open] summary :global(svg) { transform: rotate(180deg); }
  .evidence { display: grid; gap: var(--space-4); margin-top: var(--space-4); }
  .details { display: grid; gap: var(--space-5); }
  .block { display: grid; gap: var(--space-2); }
  .label { margin: 0; font-weight: 500; font-size: var(--text-sm); color: var(--body-2); }
  .explained { margin: 0 0 var(--space-4); max-width: var(--measure); color: var(--body-2); }
  .note { margin: var(--space-4) 0 0; color: var(--body-2); font-size: var(--text-sm); max-width: var(--measure); }
  .basis { margin: 0; color: var(--ink); }

  :global([data-motion='in']) .meter-fill { animation: fill 900ms var(--ease-out) 300ms both; }
  @keyframes fill { from { transform: scaleX(0); } }

  @media (min-width: 52rem) {
    .verdict { grid-template-columns: minmax(0, 18rem) minmax(0, 1fr); align-items: start; }
  }
  @media (max-width: 36rem) {
    .card { padding: var(--space-4); }
  }
  @media (prefers-reduced-motion: reduce) {
    .verdict-heading :global(.badge) { animation: none; }
  }
</style>
