<script lang="ts">
  import { t, type MessageKey } from '$lib/i18n/index.svelte';
  import { platformName } from '$lib/format';
  import { readLink, type LinkProblem } from '$lib/link-input';
  import { imageProblem } from '$lib/handoff';
  import Icon from './Icon.svelte';

  /**
   * The prompt bar, as the Gemini web app draws it: one round pill holding
   * the link field, a + button that picks an image, and on the right a
   * paste button that becomes the send button once there is text (and a
   * stop button while a check runs).
   */
  interface Props {
    id?: string;
    value?: string;
    /** Set after a failed submit. Typing clears it. */
    problem?: LinkProblem | null;
    busy?: boolean;
    disabled?: boolean;
    autofocus?: boolean;
    onlink: (url: string) => void;
    onfile: (file: File) => void;
    onstop?: () => void;
    /** 'plus': Gemini's + in the pill. 'button': a labelled "Pilih foto"
     * button under the pill, plainer for older readers (Sir, 10 Nov 2026). */
    photo?: 'plus' | 'button';
  }

  let {
    id = 'prompt',
    value = $bindable(''),
    problem = $bindable(null),
    busy = false,
    disabled = false,
    autofocus = false,
    onlink,
    onfile,
    onstop,
    photo = 'plus'
  }: Props = $props();

  let input: HTMLInputElement | undefined = $state();
  let picker: HTMLInputElement | undefined = $state();
  let fileError = $state<string | null>(null);
  let pasteFailed = $state(false);

  const reading = $derived(readLink(value));
  const hasText = $derived(value.trim().length > 0);
  const chip = $derived(
    reading.url
      ? reading.platform
        ? t('link.platform', { platform: platformName(reading.platform) })
        : t('link.web')
      : ''
  );
  const errorText = $derived(
    fileError ?? (pasteFailed ? t('link.pasteFailed') : problem ? t(`link.err.${problem}` as MessageKey) : '')
  );

  $effect(() => {
    if (autofocus) input?.focus({ preventScroll: true });
  });

  function clearErrors(): void {
    problem = null;
    pasteFailed = false;
    fileError = null;
  }

  function take(text: string): void {
    const read = readLink(text);
    // Keep the extracted link only; the surrounding share text is noise.
    value = read.url ?? text.trim();
    clearErrors();
  }

  function onpaste(e: ClipboardEvent): void {
    const text = e.clipboardData?.getData('text');
    if (!text) return;
    e.preventDefault();
    take(text);
  }

  async function pasteButton(): Promise<void> {
    clearErrors();
    try {
      const text = await navigator.clipboard.readText();
      if (!text.trim()) {
        problem = 'empty';
        return;
      }
      take(text);
      input?.focus();
    } catch {
      pasteFailed = true;
      input?.focus();
    }
  }

  function submit(e: SubmitEvent): void {
    e.preventDefault();
    if (busy || disabled) return;
    const read = readLink(value);
    problem = read.problem;
    if (!read.url) {
      input?.focus();
      return;
    }
    value = read.url;
    onlink(read.url);
  }

  function pick(e: Event): void {
    const el = e.currentTarget as HTMLInputElement;
    const file = el.files?.[0];
    el.value = '';
    if (!file) return;
    const bad = imageProblem(file);
    if (bad) {
      fileError = t(bad === 'unsupported' ? 'drop.unsupported' : 'drop.tooLarge');
      return;
    }
    clearErrors();
    onfile(file);
  }
</script>

<form class="prompt-wrap" onsubmit={submit} novalidate aria-label={t('det.form')}>
  <div class="prompt" class:has-error={!!errorText} class:busy class:no-plus={photo === 'button'}>
    {#if photo === 'plus'}
      <button
        type="button"
        class="icon-btn"
        aria-label={t('prompt.upload')}
        title={t('prompt.upload')}
        disabled={busy || disabled}
        onclick={() => picker?.click()}
      >
        <Icon name="plus" size={22} />
      </button>
    {/if}
    <input
      bind:this={picker}
      class="sr-only"
      type="file"
      accept="image/jpeg,image/png,image/webp"
      tabindex="-1"
      aria-hidden="true"
      onchange={pick}
    />
    <label class="sr-only" for={id}>{t('prompt.label')}</label>
    <input
      {id}
      bind:this={input}
      bind:value
      class="prompt-input"
      type="text"
      inputmode="url"
      enterkeyhint="go"
      autocomplete="off"
      autocapitalize="off"
      spellcheck="false"
      disabled={busy || disabled}
      placeholder={t('prompt.placeholder')}
      aria-invalid={errorText ? 'true' : undefined}
      aria-describedby="{id}-status"
      oninput={clearErrors}
      {onpaste}
    />
    {#if busy}
      <button type="button" class="icon-btn act stop" aria-label={t('prompt.stop')} title={t('prompt.stop')} onclick={() => onstop?.()}>
        <Icon name="stop" size={20} />
      </button>
    {:else if hasText}
      <button type="submit" class="icon-btn act send" aria-label={t('prompt.send')} title={t('prompt.send')} {disabled}>
        <Icon name="arrow-up" size={20} />
      </button>
    {:else}
      <button type="button" class="icon-btn act" aria-label={t('prompt.paste')} title={t('prompt.paste')} {disabled} onclick={pasteButton}>
        <Icon name="copy" size={20} />
      </button>
    {/if}
  </div>
  {#if photo === 'button'}
    <div class="or">
      <span>{t('prompt.or')}</span>
      <button type="button" class="btn btn--tonal photo-btn" disabled={busy || disabled} onclick={() => picker?.click()}>
        <Icon name="image" size={20} />{t('prompt.photo')}
      </button>
    </div>
  {/if}
  <div class="status" id="{id}-status" aria-live="polite">
    {#if errorText}
      <p class="msg msg--error"><Icon name="alert" size={16} /><span>{errorText}</span></p>
    {:else if chip}
      <p class="msg msg--ok">
        <Icon name="check" size={16} />
        <span>{chip}{reading.extracted ? ` · ${t('link.extracted')}` : ''}</span>
      </p>
    {/if}
  </div>
</form>

<style>
  .prompt-wrap {
    display: grid;
    gap: 6px;
    width: 100%;
    view-transition-name: prompt;
  }
  .prompt {
    position: relative;
    display: flex;
    align-items: center;
    gap: 4px;
    min-height: 64px;
    padding: 8px;
    background: var(--white);
    border-radius: var(--radius-prompt);
    box-shadow: var(--shadow-prompt);
    transition: box-shadow var(--dur-base) var(--ease-out);
  }
  /* Focus draws the greeting's gradient as a hairline ring around the pill. */
  .prompt::before {
    content: '';
    position: absolute;
    inset: 0;
    padding: 1.5px;
    border-radius: inherit;
    background: var(--grad);
    -webkit-mask: linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0);
    -webkit-mask-composite: xor;
    mask: linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0);
    mask-composite: exclude;
    opacity: 0;
    transition: opacity var(--dur-base) var(--ease-out);
    pointer-events: none;
  }
  .prompt:focus-within::before,
  .prompt.busy::before { opacity: 1; }
  .prompt.busy::before { animation: ring-pulse 1.6s var(--ease-in-out) infinite alternate; }
  @keyframes ring-pulse { from { opacity: 0.45; } to { opacity: 1; } }
  .prompt.has-error { box-shadow: var(--shadow-prompt), 0 0 0 1.5px var(--err); }
  .prompt.no-plus { padding-left: 20px; }
  .or {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-3);
    margin-top: 6px;
    color: var(--muted);
    font-size: var(--text-sm);
  }
  .photo-btn { min-height: 48px; padding-inline: 20px; font-size: var(--text-base); }

  .prompt-input {
    flex: 1 1 auto;
    min-width: 0;
    height: 48px;
    padding: 0 6px;
    border: 0;
    background: transparent;
    color: var(--ink);
    font: inherit;
    font-size: 1.0625rem;
    outline: none;
  }
  .prompt-input::placeholder { color: var(--muted); opacity: 1; }
  .prompt-input:disabled { color: var(--muted); }

  .act.send { background: var(--brand); color: var(--on-brand); }
  .act.send:hover { background: var(--brand-hover); color: var(--on-brand); }
  .act.stop { background: var(--surface-2); color: var(--ink); }
  .icon-btn:disabled { opacity: var(--disabled-opacity); cursor: not-allowed; }

  .status { min-height: 1.4em; padding-inline: 20px; }
  .msg { display: flex; align-items: flex-start; gap: 6px; margin: 0; font-size: var(--text-sm); }
  .msg :global(svg) { flex: none; margin-top: 3px; }
  .msg--error { color: var(--err); font-weight: 500; }
  .msg--ok { color: var(--body-2); }

  @media (prefers-reduced-motion: reduce) {
    .prompt.busy::before { animation: none; }
  }
</style>
