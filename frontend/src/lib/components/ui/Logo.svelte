<!--
  The AMBA mark (Sir, seq 1141: concept C, flat vector, friendly on the web).
  Solid fills only: no gradient, no filter. Under 28 px the small cut is used
  (circle, A and check), which stays readable as a favicon-size mark. The
  clip path id is made from the component's own id, so several marks on one
  page never clash.
-->
<script lang="ts">
  interface Props {
    size?: number;
  }
  let { size = 34 }: Props = $props();
  const uid = $props.id();
</script>

{#if size < 28}
  <svg viewBox="0 0 96 96" width={size} height={size} aria-hidden="true" focusable="false">
    <defs><clipPath id="lg-{uid}"><circle cx="48" cy="48" r="46" /></clipPath></defs>
    <circle cx="48" cy="48" r="46" fill="#1f1e1d" />
    <g clip-path="url(#lg-{uid})"><path d="M0 74 C18 64 30 82 48 74 S80 64 96 72 V96 H0 Z" fill="#b4532f" /></g>
    <path d="M30 66 L48 21 L66 66" fill="none" stroke="#f0b08d" stroke-width="11" stroke-linecap="round" stroke-linejoin="round" />
    <path d="M38.5 52 L45.5 59 L58 44" fill="none" stroke="#faf9f5" stroke-width="8" stroke-linecap="round" stroke-linejoin="round" />
  </svg>
{:else}
  <svg viewBox="0 0 96 96" width={size} height={size} aria-hidden="true" focusable="false">
    <defs><clipPath id="lg-{uid}"><circle cx="48" cy="48" r="44" /></clipPath></defs>
    <circle cx="48" cy="48" r="44" fill="#1f1e1d" />
    <g clip-path="url(#lg-{uid})">
      <path d="M0 71 C18 61 30 79 48 71 S80 61 96 69 V96 H0 Z" fill="#b4532f" />
      <path d="M0 83 C20 75 32 91 50 83 S80 75 96 81 V96 H0 Z" fill="#d97757" />
    </g>
    <circle cx="48" cy="48" r="44" fill="none" stroke="#d97757" stroke-width="3.5" />
    <path d="M33 62 L48 25 L63 62" fill="none" stroke="#f0b08d" stroke-width="8" stroke-linecap="round" stroke-linejoin="round" />
    <path d="M40 50.5 L45.5 56 L55.5 44.5" fill="none" stroke="#faf9f5" stroke-width="5.5" stroke-linecap="round" stroke-linejoin="round" />
  </svg>
{/if}
