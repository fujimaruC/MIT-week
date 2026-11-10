<!--
  The aurora behind every page (Sir, 10 Nov 2026: animated, but light on the
  visitor's device). Three soft ribbons drawn with gradients only: no filter,
  no canvas, no script. Each moves by transform alone, so the compositor runs
  it without repainting; the layer is fixed, contained and ignores the
  pointer. Reduced motion holds it still; print drops it.
-->
<div class="aurora" aria-hidden="true">
  <span class="r r1"></span>
  <span class="r r2"></span>
  <span class="r r3"></span>
  <span class="stars s1"></span>
  <span class="stars s2"></span>
</div>

<style>
  .aurora {
    position: fixed;
    inset: 0;
    z-index: -1;
    overflow: hidden;
    pointer-events: none;
    contain: strict;
  }
  .r {
    position: absolute;
    border-radius: 50%;
    will-change: transform;
    opacity: var(--aur-o);
  }
  /* Wide, tilted ellipses read as ribbons rather than blobs. */
  .r1 {
    left: -25vmax;
    top: -28vmax;
    width: 95vmax;
    height: 48vmax;
    background: radial-gradient(closest-side, var(--aur-1), transparent 72%);
    transform: rotate(-12deg);
    animation: drift-1 28s var(--ease-in-out) infinite alternate;
  }
  .r2 {
    right: -30vmax;
    top: -12vmax;
    width: 85vmax;
    height: 40vmax;
    background: radial-gradient(closest-side, var(--aur-2), transparent 70%);
    transform: rotate(18deg);
    animation: drift-2 34s var(--ease-in-out) infinite alternate;
  }
  .r3 {
    left: 5vmax;
    top: 42vh;
    width: 80vmax;
    height: 36vmax;
    background: radial-gradient(closest-side, var(--aur-3), transparent 70%);
    transform: rotate(-6deg);
    animation: drift-3 40s var(--ease-in-out) infinite alternate;
  }
  /* Tiny stars: one element each, many dots as shadows, and only opacity
   * changes, so they cost nothing to animate. Two layers twinkle out of step. */
  .stars {
    position: absolute;
    left: 0;
    top: 0;
    width: 2px;
    height: 2px;
    border-radius: 50%;
    background: transparent;
    color: var(--star, rgba(255, 226, 205, 0.9));
    opacity: 0.2;
    animation: twinkle 5.5s var(--ease-in-out) infinite alternate;
  }
  .s1 {
    box-shadow:
      6vw 9vh 0 0 currentColor, 17vw 31vh 0 0 currentColor, 29vw 14vh 0 1px currentColor,
      41vw 47vh 0 0 currentColor, 53vw 6vh 0 0 currentColor, 64vw 28vh 0 1px currentColor,
      77vw 12vh 0 0 currentColor, 88vw 39vh 0 0 currentColor, 95vw 7vh 0 1px currentColor,
      11vw 62vh 0 0 currentColor, 36vw 78vh 0 0 currentColor, 71vw 66vh 0 1px currentColor,
      92vw 84vh 0 0 currentColor, 58vw 91vh 0 0 currentColor;
  }
  .s2 {
    animation-duration: 7.5s;
    animation-delay: -2.7s;
    box-shadow:
      3vw 24vh 0 0 currentColor, 22vw 5vh 0 1px currentColor, 34vw 38vh 0 0 currentColor,
      47vw 21vh 0 0 currentColor, 60vw 52vh 0 1px currentColor, 82vw 24vh 0 0 currentColor,
      97vw 55vh 0 0 currentColor, 14vw 83vh 0 1px currentColor, 44vw 70vh 0 0 currentColor,
      67vw 81vh 0 0 currentColor, 85vw 93vh 0 1px currentColor, 25vw 56vh 0 0 currentColor;
  }
  @keyframes twinkle {
    from { opacity: 0.1; }
    to { opacity: 0.85; }
  }

  @keyframes drift-1 {
    to { transform: translate3d(14vmax, 8vmax, 0) rotate(4deg) scale(1.12); }
  }
  @keyframes drift-2 {
    to { transform: translate3d(-16vmax, 10vmax, 0) rotate(-6deg) scale(1.08); }
  }
  @keyframes drift-3 {
    to { transform: translate3d(10vmax, -14vmax, 0) rotate(10deg) scale(1.18); }
  }

  @media (prefers-reduced-motion: reduce) {
    .r { animation: none; }
    .stars { animation: none; opacity: 0.35; }
  }
  @media print {
    .aurora { display: none; }
  }
</style>
