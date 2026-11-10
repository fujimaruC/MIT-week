<script lang="ts">
  import { t } from '$lib/i18n/index.svelte';
  import { onMount } from 'svelte';
  import { reducedMotion } from '$lib/motion';
  import Icon from './Icon.svelte';

  /**
   * Theme: dark by default, light on request (Sir, 10 Nov 2026). One toggle,
   * as Gemini puts it in its menu. The pre-paint script in app.html applies
   * the stored choice before the first paint.
   */
  interface Props {
    /** Show the text label (the side rail); icon only otherwise. */
    withLabel?: boolean;
  }
  let { withLabel = false }: Props = $props();

  type Theme = 'dark' | 'light';
  const KEY = 'amba-theme';
  let theme = $state<Theme>('dark');

  function apply(next: Theme) {
    theme = next;
    document.documentElement.dataset.theme = next;
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', next === 'dark' ? '#141413' : '#faf9f5');
    try {
      if (next === 'dark') localStorage.removeItem(KEY);
      else localStorage.setItem(KEY, next);
    } catch {
      // Session-only when storage is blocked.
    }
  }

  // The new theme spreads as a circle from the button, through the View
  // Transitions API; without it, or under reduced motion, it switches at once.
  function choose(next: Theme, e: MouseEvent) {
    const doc = document as Document & { startViewTransition?: (cb: () => void) => { ready: Promise<void>; finished: Promise<void> } };
    if (!doc.startViewTransition || reducedMotion()) {
      apply(next);
      return;
    }
    const x = e.clientX || innerWidth - 40;
    const y = e.clientY || 30;
    const r = Math.hypot(Math.max(x, innerWidth - x), Math.max(y, innerHeight - y));
    const root = document.documentElement;
    root.classList.add('theme-vt');
    const vt = doc.startViewTransition(() => apply(next));
    void vt.ready
      .then(() => {
        root.animate(
          { clipPath: [`circle(0px at ${x}px ${y}px)`, `circle(${r}px at ${x}px ${y}px)`] },
          { duration: 560, easing: 'cubic-bezier(0.2, 0, 0, 1)', pseudoElement: '::view-transition-new(root)' }
        );
      })
      .catch(() => {});
    void vt.finished.finally(() => root.classList.remove('theme-vt'));
  }

  onMount(() => {
    theme = document.documentElement.dataset.theme === 'light' ? 'light' : 'dark';
  });

  const label = $derived(theme === 'dark' ? t('theme.toLight') : t('theme.toDark'));
</script>

<button
  type="button"
  class={withLabel ? 'nav-link theme-toggle' : 'icon-btn theme-toggle'}
  aria-label={withLabel ? undefined : label}
  title={label}
  onclick={(e) => choose(theme === 'dark' ? 'light' : 'dark', e)}
>
  <Icon name={theme === 'dark' ? 'sun' : 'moon'} size={20} />
  {#if withLabel}<span class="lbl">{label}</span>{/if}
</button>

<style>
  .theme-toggle.nav-link {
    width: 100%;
    border: 0;
    background: transparent;
    font: inherit;
    font-size: var(--text-sm);
    font-weight: 500;
    cursor: pointer;
    text-align: left;
  }
</style>
