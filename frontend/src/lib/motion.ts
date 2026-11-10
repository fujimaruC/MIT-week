/**
 * Motion helpers for direction A. Nothing is ever hidden unless its reveal is
 * certain to run: `inview` sets data-motion="wait" only when script runs and
 * reduced motion is off, then "in" once the element enters the viewport.
 */
import type { Action } from 'svelte/action';

export function reducedMotion(): boolean {
  return typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
}

export const inview: Action<HTMLElement, { threshold?: number } | undefined> = (node, options) => {
  if (reducedMotion() || typeof IntersectionObserver !== 'function') return;
  node.dataset.motion = 'wait';
  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        node.dataset.motion = 'in';
        observer.disconnect();
      }
    },
    { threshold: options?.threshold ?? 0, rootMargin: '0px 0px -12% 0px' }
  );
  observer.observe(node);
  return { destroy: () => observer.disconnect() };
};

/** Splits authored text into lines for the redaction reveal ("\n" marks a break). */
export function lines(text: string): string[] {
  return text.split('\n').map((l) => l.trim()).filter(Boolean);
}
