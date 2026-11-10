<script lang="ts">
  import { t, type MessageKey } from '$lib/i18n/index.svelte';
  import { aiLevel } from '$lib/i18n/verdict';
  import { formatClock, formatDate, platformName } from '$lib/format';
  import Panel from '$lib/components/ui/Panel.svelte';
  import Icon from '$lib/components/ui/Icon.svelte';
  import VideoProvenance from './VideoProvenance.svelte';
  import type { DossierParsed } from '$lib/api/schemas';

  interface Props {
    video: NonNullable<DossierParsed['video']>;
    /** The video's AI score lives in the AI pillar, not in `video`. */
    ai: DossierParsed['pillars']['ai'];
  }

  let { video, ai }: Props = $props();
  const uid = $props.id();

  const platform = $derived(platformName(video.platform));
  const date = $derived(formatDate(video.upload_date));

  type SignalKey = 'ai' | 'manipulation' | 'context';
  interface Signal {
    key: SignalKey;
    name: string;
    hint: string;
    /** null: the check did not run. */
    score: number | null;
    indicators: string[];
  }

  const allSignals = $derived<Signal[]>([
    {
      key: 'ai',
      name: t('pillar.ai'),
      hint: t('video.ai.hint'),
      score: ai.available ? Math.round(ai.score) : null,
      indicators: ai.available ? ai.indicators : []
    },
    {
      key: 'manipulation',
      name: t('video.manipulation.name'),
      hint: t('video.manipulation.hint'),
      score: video.manipulation.score,
      indicators: video.manipulation.indicators
    },
    {
      key: 'context',
      name: t('video.context.name'),
      hint: t('video.context.hint'),
      score: video.context.score,
      indicators: video.context.indicators
    }
  ]);

  // A signal that did not run is left out instead of shown as a blank row.
  const signals = $derived(allSignals.filter((s) => s.score !== null));

  // Every signal uses the AI card's bands: 0-39 low, 40-69 medium, 70-100 high.
  // The context score measures a MISmatch, so its level says so.
  const levelText = (key: SignalKey, score: number): string =>
    t(`${key === 'context' ? 'video.context.level' : 'video.level'}.${aiLevel(score)}` as MessageKey);

  const meterWidth = (score: number): string => `${Math.min(100, Math.max(0, score))}%`;
</script>

<Panel title={t('video.title')}>
  <div class="video" data-testid="video">
    <div class="head">
      <p class="meta">
        {#if platform}<span class="platform">{platform}</span>{/if}
        {#if video.duration_sec > 0}<span>{t('video.duration', { time: formatClock(video.duration_sec) })}</span>{/if}
        {#if date}<span>{t('video.uploaded', { date })}</span>{/if}
      </p>
      {#if video.platform_ai_label}
        <p class="ai-label"><Icon name="sparkle" size={16} /> {t('video.aiLabel')}</p>
      {/if}
    </div>

    {#if video.status === 'UNAVAILABLE'}
      <!-- Footage not read: say what the result rests on, not why the download failed (Sir, 10 Nov). -->
      <p class="desc">{t('video.previewBasis')}</p>
    {:else}
      {#if video.frames.length}
        <div class="block">
          <h3 class="label" id="{uid}-frames">{t('video.frames')}</h3>
          <!-- The strip scrolls sideways, so it must take keyboard focus (WCAG 2.1.1). The frames are not controls. -->
          <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
          <div class="strip" role="region" aria-labelledby="{uid}-frames" tabindex="0">
            <ol>
              {#each video.frames as frame, i (i)}
                <li>
                  <img
                    src={frame.thumb}
                    alt={t('video.frameAlt', { n: i + 1, sec: Math.floor(frame.at_sec) })}
                    decoding="async"
                  />
                  <span class="at" aria-hidden="true">{formatClock(frame.at_sec)}</span>
                </li>
              {/each}
            </ol>
          </div>
        </div>
      {/if}

      {#if video.summary}
        <div class="block">
          <h3 class="label">{t('video.summary')}</h3>
          <!-- Model output is written in Indonesian whatever the interface language. -->
          <p class="summary" lang="id">{video.summary}</p>
        </div>
      {/if}

      {#if video.transcript}
        <details class="transcript">
          <summary>
            <Icon name="speaker" size={18} />
            <span class="name">{t('video.transcript')}</span>
            <span class="chev" aria-hidden="true"><Icon name="chevron" size={18} /></span>
          </summary>
          <div class="transcript-body">
            <p class="quote">{video.transcript}</p>
            <p class="note">{video.audio_analyzed ? t('video.audioAnalyzed') : t('video.audioNotAnalyzed')}</p>
          </div>
        </details>
      {/if}

      <div class="block">
        <h3 class="label">{t('video.signals')}</h3>
        <ul class="signals">
          {#each signals as signal (signal.key)}
            <li class="signal" data-signal={signal.key}>
              <div class="signal-head">
                <h4>{signal.name}</h4>
                {#if signal.score !== null}
                  <span class="lvl lvl--{aiLevel(signal.score)}">{levelText(signal.key, signal.score)}</span>
                  <span class="score">{t('video.score', { n: signal.score })}</span>
                {/if}
              </div>
              <p class="hint">{signal.hint}</p>
              {#if signal.score !== null}
                <!-- The score and level are already text; the bar only repeats them. -->
                <div class="meter" aria-hidden="true">
                  <span class="meter-fill" style:width={meterWidth(signal.score)}></span>
                </div>
                {#if signal.indicators.length}
                  <ul class="indicators" lang="id">
                    {#each signal.indicators as indicator, i (i)}<li>{indicator}</li>{/each}
                  </ul>
                {:else}
                  <p class="note">{t('video.noIndicators')}</p>
                {/if}
              {/if}
            </li>
          {/each}
        </ul>
      </div>

      <VideoProvenance {video} />
      <p class="note">{t('video.caveat')}</p>
    {/if}
  </div>
</Panel>

<style>
  .video { display: grid; gap: var(--space-5); }
  .head, .block { display: grid; gap: var(--space-2); }

  /* Header: the LinkPreview meta line, plus the platform's own AI label. */
  .meta { margin: 0; display: flex; flex-wrap: wrap; gap: 6px; font-size: var(--text-sm); color: var(--muted); }
  .meta span + span::before { content: '·'; margin-right: 6px; }
  .platform { font-weight: 700; color: var(--ink); }
  .ai-label {
    justify-self: start;
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
    padding: 4px 14px;
    background: var(--yellow);
    color: var(--on-fill);
    border: var(--border-width) solid var(--border);
    border-radius: var(--radius-pill);
    font-weight: 700;
    font-size: var(--text-sm);
  }
  .label {
    margin: 0;
    font-family: var(--font-display);
    font-weight: 800;
    font-size: var(--text-sm);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--indigo-text);
  }

  /* Frame strip: scrolls sideways, snaps to frames, never animates. */
  .strip {
    overflow-x: auto;
    overscroll-behavior-x: contain;
    scroll-snap-type: x proximity;
    padding: 2px 2px var(--space-2);
    border-radius: var(--radius-sm);
  }
  .strip:focus-visible { outline: var(--focus-ring); outline-offset: var(--focus-offset); }
  .strip ol { display: flex; gap: var(--space-3); width: max-content; margin: 0; padding: 0; list-style: none; }
  .strip li { display: grid; gap: var(--space-1); scroll-snap-align: start; }
  .strip img {
    height: 9rem;
    width: auto;
    max-width: none;
    border: var(--border-width) solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface-2);
  }
  .at { font-family: var(--font-display); font-weight: 700; font-size: var(--text-sm); color: var(--muted); }

  .summary { margin: 0; max-width: var(--measure); overflow-wrap: anywhere; }

  /* Transcript: the EvidenceAccordion item, closed by default. */
  .transcript {
    border: var(--border-width) solid var(--border);
    border-radius: var(--radius-md);
    background: var(--paper);
    overflow: hidden;
    transition: background-color var(--dur-fast) var(--ease-out);
  }
  .transcript:hover { background: var(--indigo-tint); }
  .transcript[open] { background: var(--white); }
  summary {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-height: 52px;
    padding: var(--space-3) var(--space-4);
    cursor: pointer;
    list-style: none;
  }
  summary::-webkit-details-marker { display: none; }
  summary:focus-visible { outline: var(--focus-ring); outline-offset: -6px; }
  .name { font-family: var(--font-display); font-weight: 800; font-size: 1.05em; }
  .chev { margin-left: auto; display: inline-flex; transition: transform var(--dur-fast) var(--ease-out); }
  .transcript[open] .chev { transform: rotate(180deg); }
  .transcript-body { display: grid; gap: var(--space-2); padding: 0 var(--space-4) var(--space-4); }
  .quote { margin: 0; color: var(--body-2); max-width: var(--measure); white-space: pre-line; overflow-wrap: anywhere; }

  /* Signal rows: accordion-item surface, StatusBadge tones, the shared meter. */
  .signals { display: grid; gap: var(--space-3); margin: 0; padding: 0; list-style: none; }
  .signal {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-4);
    border: var(--border-width) solid var(--border);
    border-radius: var(--radius-md);
    background: var(--paper);
  }
  .signal-head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2) var(--space-3); }
  h4 { margin-right: auto; font-size: 1.05em; }
  .lvl {
    padding: 2px 10px;
    border: 2px solid var(--border);
    border-radius: var(--radius-pill);
    font-size: var(--text-sm);
    font-weight: 700;
    color: var(--on-fill);
  }
  .lvl--low { background: var(--safe); }
  .lvl--medium { background: var(--yellow); }
  .lvl--high { background: var(--coral); }
  .score { font-size: var(--text-sm); font-weight: 700; color: var(--muted); }
  .hint { margin: 0; font-size: var(--text-sm); color: var(--body-2); max-width: var(--measure); }
  .meter { height: 20px; border-radius: var(--radius-pill); overflow: hidden; }
  .meter-fill { display: block; height: 100%; }
  .indicators { display: grid; gap: var(--space-1); margin: 0; padding-left: var(--space-5); list-style: disc; color: var(--body-2); }
  .note { margin: 0; font-size: var(--text-sm); color: var(--muted); max-width: var(--measure); }

  .desc { margin: 0; color: var(--body-2); max-width: var(--measure); }
</style>
