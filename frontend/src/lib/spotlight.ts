/**
 * The pointer's light (Sir, seq 1118): while the mouse is over a card, its
 * position is written to --mx / --my on that card, and premium.css paints a
 * soft spot there. One listener on the window, one write per event, mouse
 * only (never on touch), and nothing at all under reduced motion.
 */
import { reducedMotion } from '$lib/motion';

const CARDS = '.member, .card, .ex, .learn, .f-item, .cue, .panel, .data, .legal';

export function trackPointer(e: PointerEvent): void {
  if (e.pointerType !== 'mouse' || reducedMotion()) return;
  const card = (e.target as Element | null)?.closest?.(CARDS) as HTMLElement | null | undefined;
  if (!card) return;
  const { clientX, clientY } = e;
  requestAnimationFrame(() => {
    const r = card.getBoundingClientRect();
    card.style.setProperty('--mx', `${clientX - r.left}px`);
    card.style.setProperty('--my', `${clientY - r.top}px`);
  });
}
