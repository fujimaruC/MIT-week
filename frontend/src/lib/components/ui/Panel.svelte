<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    title?: string;
    /** Colour role of the panel surface. `white` is the default translucent pane. */
    tone?: 'white' | 'paper' | 'ink' | 'safe' | 'yellow' | 'coral' | 'indigo' | 'soft';
    children: Snippet;
    footer?: Snippet;
  }

  let { title, tone = 'white', children, footer }: Props = $props();

  const TONE_CLASS: Record<NonNullable<Props['tone']>, string> = {
    white: '',
    paper: 'panel--paper',
    ink: 'panel--ink',
    safe: 'panel--safe',
    yellow: 'panel--yellow',
    coral: 'panel--coral',
    indigo: 'panel--indigo',
    soft: 'panel--soft'
  };
</script>

<section class="panel {TONE_CLASS[tone]}">
  {#if title}
    <h2>{title}</h2>
  {/if}
  <div class="body">{@render children()}</div>
  {#if footer}
    <div class="footer">{@render footer()}</div>
  {/if}
</section>

<style>
  /* A tonal pane, as Gemini draws its cards: no border, a round corner. */
  .panel {
    background: var(--card);
    border: 0;
    border-radius: var(--radius-lg);
    box-shadow: none;
    padding: clamp(1.1rem, 3vw, 1.6rem);
    min-width: 0;
  }
  .panel--paper { background: var(--paper); }
  .panel--ink { background: var(--panel-dark); color: var(--on-panel-dark); }
  .panel--safe { background: var(--safe); color: var(--on-fill); }
  .panel--yellow { background: var(--yellow); color: var(--on-fill); }
  .panel--coral { background: var(--coral); color: var(--on-fill); }
  .panel--indigo { background: var(--indigo-soft); color: var(--ink); }
  .panel--soft { background: var(--surface-2); color: var(--ink); }

  h2 { font-size: var(--text-lg); font-weight: 500; margin: 0 0 var(--space-4); }
  .body { min-width: 0; }
  .footer {
    margin-top: var(--space-5);
    padding-top: var(--space-4);
    border-top: 1px solid var(--border);
  }
  .panel--ink :global(a) { color: var(--on-panel-dark); }
</style>
