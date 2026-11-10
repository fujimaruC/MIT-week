<script lang="ts">
  import type { Tactic } from '$lib/api/types';

  interface Props {
    text: string;
    tactics: Tactic[];
    labels: Record<string, { label: string; why: string }>;
    /** Shown above the quote, e.g. "Dibaca dari gambar" for OCR text. */
    sourceLabel?: string;
  }

  let { text, tactics, labels, sourceLabel = '' }: Props = $props();

  /**
   * Splits text into plain and marked segments. Matching is case-insensitive
   * and index-based; every piece renders through Svelte interpolation, so no
   * user or model content is ever injected as HTML.
   *
   * The explanation is NOT rendered inline: a word chip inside the sentence
   * broke the line ("Akun Anda [Menakut-nakuti] akan diblokir"). Each mark
   * carries a superscript number instead, and the numbered legend sits under
   * the quote, reusing the same numbered-circle language as the scam-school
   * cards. That also makes the explanation reachable by keyboard and screen
   * reader, which a hover-only popover was not.
   */
  const segments = $derived.by(() => {
    type Mark = { start: number; end: number; key: string };
    const marks: Mark[] = [];
    const haystack = text.toLowerCase();

    for (const tactic of tactics) {
      if (!labels[tactic.code]) continue;
      const needle = tactic.phrase.toLowerCase();
      if (!needle) continue;
      let from = 0;
      while (from < haystack.length) {
        const idx = haystack.indexOf(needle, from);
        if (idx === -1) break;
        if (!marks.some((m) => idx < m.end && idx + needle.length > m.start)) {
          marks.push({ start: idx, end: idx + needle.length, key: tactic.code });
        }
        from = idx + needle.length;
      }
    }

    marks.sort((a, b) => a.start - b.start);

    // Number by first appearance so the legend order matches the reading order.
    const order: string[] = [];
    for (const m of marks) if (!order.includes(m.key)) order.push(m.key);
    // A verified phrase that is not in the visible text (for example beyond
    // the part shown) is still listed, quoted, so no finding is hidden.
    const unplaced: Array<{ code: string; phrase: string }> = [];
    for (const t of tactics) {
      if (labels[t.code] && !order.includes(t.code) && !unplaced.some((u) => u.code === t.code)) {
        unplaced.push({ code: t.code, phrase: t.phrase });
      }
    }

    const out: Array<{ text: string; key?: string; n?: number }> = [];
    let cursor = 0;
    for (const m of marks) {
      if (m.start > cursor) out.push({ text: text.slice(cursor, m.start) });
      out.push({ text: text.slice(m.start, m.end), key: m.key, n: order.indexOf(m.key) + 1 });
      cursor = m.end;
    }
    if (cursor < text.length) out.push({ text: text.slice(cursor) });

    return { segments: out, order, unplaced };
  });
</script>

<figure class="quote">
  {#if sourceLabel}<p class="source-label">{sourceLabel}</p>{/if}
  <blockquote class="text">
    {#each segments.segments as seg, i (i)}{#if seg.key}<mark class="tactic"
          >{seg.text}<sup class="n" aria-hidden="true">{seg.n}</sup></mark
        >{:else}{seg.text}{/if}{/each}
  </blockquote>

  <figcaption>
    <ol class="legend">
      {#each segments.order as code, i (code)}
        <li>
          <span class="num" aria-hidden="true">{i + 1}</span>
          <span>
            <strong>{labels[code].label}.</strong>
            {labels[code].why}
          </span>
        </li>
      {/each}
      {#each segments.unplaced as item, i (item.code)}
        <li>
          <span class="num" aria-hidden="true">{segments.order.length + i + 1}</span>
          <span>
            <strong>{labels[item.code].label}:</strong> “{item.phrase}”.
            {labels[item.code].why}
          </span>
        </li>
      {/each}
    </ol>
  </figcaption>
</figure>

<style>
  .quote { margin: 0; }
  .source-label {
    margin: 0 0 var(--space-2);
    font-family: var(--font-mono, monospace);
    font-size: var(--text-sm);
    color: var(--muted);
  }
  .text {
    margin: 0;
    white-space: pre-wrap;
    line-height: 1.8;
    font-size: var(--text-lg);
  }
  .tactic {
    background: var(--yellow);
    color: var(--on-fill);
    border-radius: 6px;
    padding: 1px 3px;
    box-decoration-break: clone;
    -webkit-box-decoration-break: clone;
  }
  .n {
    font-family: var(--font-display);
    font-weight: 800;
    font-size: 0.62em;
    line-height: 0;
    vertical-align: super;
    margin-left: 1px;
  }

  .legend {
    display: grid;
    gap: var(--space-3);
    margin: var(--space-5) 0 0;
    padding: var(--space-4) 0 0;
    border-top: var(--border-width) dashed var(--border);
    list-style: none;
  }
  .legend li {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    font-size: var(--text-base);
    line-height: 1.45;
    color: var(--body-2);
  }
  .num {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    flex: none;
    background: var(--yellow);
    border: 2px solid var(--border);
    border-radius: 50%;
    font-family: var(--font-display);
    font-weight: 800;
    font-size: var(--text-xs);
    color: var(--on-fill);
  }
  strong { color: var(--ink); }
</style>
