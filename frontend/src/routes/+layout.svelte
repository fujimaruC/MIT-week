<script lang="ts">
  import { t, type MessageKey } from '$lib/i18n/index.svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { onNavigate } from '$app/navigation';
  import '../lib/styles/tokens.css';
  import '../app.css';
  import '../lib/styles/fonts.css';
  import '../lib/styles/refinement.css';
  import '../lib/styles/premium.css';
  import ThemeSwitch from '$lib/components/ui/ThemeSwitch.svelte';
  import PreviewBadge from '$lib/components/ui/PreviewBadge.svelte';
  import Aurora from '$lib/components/ui/Aurora.svelte';
  import Icon from '$lib/components/ui/Icon.svelte';
  import Logo from '$lib/components/ui/Logo.svelte';
  import { reducedMotion } from '$lib/motion';
  import { trackPointer } from '$lib/spotlight';

  let { children } = $props();

  // One shell for every page (Sir, 10 Nov 2026: parents need few pages and
  // few buttons): a top bar with the three destinations, always visible, with
  // no hidden menu on phones, and a one-line footer. Tentang is in the bar
  // again at Sir's request (seq 1029).
  const path = $derived(page.url.pathname);
  const NAV: Array<{ href: '/deteksi' | '/edukasi' | '/tentang'; label: MessageKey; icon: 'search' | 'book' | 'info' }> = [
    { href: '/deteksi', label: 'nav.check', icon: 'search' },
    { href: '/edukasi', label: 'nav.learn', icon: 'book' },
    { href: '/tentang', label: 'nav.about', icon: 'info' }
  ];
  const current = (href: string): boolean =>
    href === '/deteksi' ? path === '/deteksi' || path === '/hasil' : path.startsWith(href);
  const wide = $derived(path === '/deteksi' || path === '/hasil' || path.startsWith('/edukasi') || path === '/tentang');

  // Page changes run through the View Transitions API where it exists.
  onNavigate((navigation) => {
    if (!document.startViewTransition || reducedMotion()) return;
    return new Promise((done) => {
      document.startViewTransition(async () => {
        done();
        await navigation.complete;
      });
    });
  });

  // Move focus to the main heading on NAVIGATION only.
  let previousPath: string | null = null;
  $effect(() => {
    const now = page.url.pathname;
    if (previousPath === null) {
      previousPath = now;
      return;
    }
    if (now === previousPath) return;
    previousPath = now;
    const heading = document.getElementById('main')?.querySelector('h1');
    if (heading) {
      heading.setAttribute('tabindex', '-1');
      heading.focus({ preventScroll: true });
    }
  });
</script>

<svelte:window onpointermove={trackPointer} />

<a class="skip-link" href="#main">{t('layout.skip')}</a>

<PreviewBadge />

<Aurora />

<header class="top">
  <div class="top-inner">
    <a class="brand" href={resolve('/')} aria-label={t('layout.home')}>
      <span class="logo" aria-hidden="true"><Logo size={36} /></span>
      <span class="wordmark">AMBA</span>
    </a>
    <nav aria-label={t('layout.nav')}>
      <ul>
        {#each NAV as link (link.href)}
          <li>
            <a class="top-link" href={resolve(link.href)} aria-current={current(link.href) ? 'page' : undefined}>
              <Icon name={link.icon} size={20} /><span>{t(link.label)}</span>
            </a>
          </li>
        {/each}
      </ul>
    </nav>
    <ThemeSwitch />
  </div>
</header>

<main id="main" tabindex="-1">
  {#if path === '/'}
    {@render children()}
  {:else}
    <div class="wrap page-wrap" class:wrap--wide={wide}>
      {@render children()}
    </div>
  {/if}
</main>

<footer class="foot">
  <p>
    {t('foot.note')}
    <a href={resolve('/tentang')}>{t('foot.more')}</a>
  </p>
</footer>

<style>
  .top {
    position: sticky;
    top: 0;
    z-index: 40;
    /* Nearly solid and without backdrop blur: a blur over the moving aurora
     * would be recomputed every frame. */
    background: color-mix(in srgb, var(--paper) 93%, transparent);
    border-bottom: 1px solid var(--border-soft);
    view-transition-name: top;
  }
  .top-inner {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    max-width: var(--maxw-wide);
    min-height: 60px;
    margin-inline: auto;
    padding-inline: var(--gutter);
  }
  .brand {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    min-width: 44px;
    min-height: 44px;
    margin-right: auto;
    color: var(--ink);
    text-decoration: none;
  }
  .brand:hover { color: var(--ink); }
  .logo {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    flex: none;
    border-radius: 50%;
  }
  .wordmark { font-size: 1.15rem; font-weight: 500; letter-spacing: 0.03em; }
  nav ul { display: flex; gap: 4px; margin: 0; padding: 0; list-style: none; }
  .top-link {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    min-height: 44px;
    padding: 0 14px;
    border-radius: 999px;
    color: var(--body-2);
    font-size: var(--text-sm);
    font-weight: 500;
    text-decoration: none;
  }
  .top-link:hover { background: var(--hover-overlay); color: var(--ink); }
  .top-link[aria-current='page'] { background: var(--surface-2); color: var(--ink); }

  main { min-width: 0; }
  main:focus { outline: none; }
  .page-wrap { padding-block: clamp(1.25rem, 3.5vw, 2.5rem) clamp(2.5rem, 6vw, 4rem); }

  .foot {
    margin-top: var(--space-7);
    padding: var(--space-5) var(--gutter);
    border-top: 1px solid var(--border-soft);
    text-align: center;
  }
  .foot p { margin: 0 auto; max-width: var(--maxw-wide); color: var(--muted); font-size: var(--text-sm); }
  .foot a { display: inline-flex; align-items: center; min-height: 44px; padding-inline: 4px; color: var(--body-2); }
  .foot a:hover { color: var(--ink); }

  /* Phones: each destination is its icon over its word, as in a tab bar, so
   * all three stay visible with their names. */
  @media (max-width: 30rem) {
    nav ul { gap: 2px; }
    .top-link {
      flex-direction: column;
      justify-content: center;
      gap: 2px;
      min-width: 60px;
      min-height: 52px;
      padding: 4px 6px;
      border-radius: 14px;
      font-size: var(--text-xs);
      line-height: 1.1;
    }
  }
  /* The narrowest phones keep the mark and drop the written name. */
  @media (max-width: 23.75rem) {
    .wordmark { display: none; }
  }
</style>
