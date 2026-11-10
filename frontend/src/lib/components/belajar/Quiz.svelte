<script lang="ts">
  import { tick } from 'svelte';
  import { resolve } from '$app/paths';
  import { QUESTIONS, UI, type Text } from '$lib/data/belajar';
  import Icon from '$lib/components/ui/Icon.svelte';

  const uid = $props.id();
  const total = QUESTIONS.length;

  let index = $state(0);
  let picked = $state('');
  let submitted = $state(false);
  let missing = $state(false);
  let score = $state(0);
  let done = $state(false);

  let legend: HTMLElement | null = $state(null);
  let resultHeading: HTMLElement | null = $state(null);

  const tx = (text: Text, params?: Record<string, string | number>): string => {
    let out = text.id;
    for (const [k, v] of Object.entries(params ?? {})) out = out.replaceAll(`{${k}}`, String(v));
    return out;
  };

  const q = $derived(QUESTIONS[index]);
  const correct = $derived(submitted && picked === q.answer);
  const answerText = $derived(q.options.find((o) => o.id === q.answer)?.text);
  const band = $derived(score / total >= 0.75 ? UI.quiz.high : score / total >= 0.5 ? UI.quiz.mid : UI.quiz.low);

  async function press(): Promise<void> {
    if (!submitted) {
      if (!picked) {
        missing = true;
        return;
      }
      missing = false;
      submitted = true;
      if (picked === q.answer) score += 1;
      return;
    }
    if (index === total - 1) {
      done = true;
      await tick();
      resultHeading?.focus();
      return;
    }
    index += 1;
    picked = '';
    submitted = false;
    await tick();
    legend?.focus();
  }

  async function restart(): Promise<void> {
    index = 0;
    picked = '';
    submitted = false;
    missing = false;
    score = 0;
    done = false;
    await tick();
    legend?.focus();
  }
</script>

<div class="quiz card">
  {#if !done}
    <p class="progress">
      <span>{tx(UI.quiz.progress, { n: index + 1, total })}</span>
      <span class="bar" aria-hidden="true"><span style:transform="scaleX({(index + (submitted ? 1 : 0)) / total})"></span></span>
    </p>

    <fieldset class="q" disabled={submitted}>
      <legend bind:this={legend} tabindex="-1" id="{uid}-legend">{tx(q.question)}</legend>
      <div class="options">
        {#each q.options as option (option.id)}
          <label
            class="option"
            class:right={submitted && option.id === q.answer}
            class:miss={submitted && picked === option.id && option.id !== q.answer}
          >
            <input type="radio" name="{uid}-q{index}" value={option.id} bind:group={picked} onchange={() => (missing = false)} />
            <span class="text">{tx(option.text)}</span>
            {#if submitted && option.id === q.answer}
              <span class="tag"><Icon name="check" size={14} /> {tx(UI.quiz.tagRight)}</span>
            {:else if submitted && picked === option.id}
              <span class="tag">{tx(UI.quiz.tagYours)}</span>
            {/if}
          </label>
        {/each}
      </div>
    </fieldset>

    <p class="missing" role="alert">
      {#if missing}<Icon name="alert" size={16} /> {tx(UI.quiz.pick)}{/if}
    </p>

    <div class="feedback" role="status" aria-live="polite" class:filled={submitted}>
      {#if submitted}
        <p class="fb-head"><strong>{correct ? tx(UI.quiz.right) : tx(UI.quiz.wrong)}</strong>
          {#if !correct && answerText}{tx(UI.quiz.correctIs, { answer: tx(answerText) })}{/if}</p>
        <p class="explain">{tx(q.explain)}</p>
      {/if}
    </div>

    <button class="btn btn--primary" type="button" onclick={press}>
      {!submitted ? tx(UI.quiz.answer) : index === total - 1 ? tx(UI.quiz.finish) : tx(UI.quiz.next)}
      <Icon name="arrow-right" size={18} />
    </button>
  {:else}
    <div class="result" data-testid="quiz-result">
      <h3 tabindex="-1" bind:this={resultHeading}>{tx(UI.quiz.scoreTitle)}</h3>
      <p class="big" role="status">{tx(UI.quiz.scoreOf, { score, total })}</p>
      <p class="msg">{tx(band)}</p>
      <div class="row">
        <button class="btn btn--primary" type="button" onclick={restart}>
          <Icon name="refresh" size={18} /> {tx(UI.quiz.again)}
        </button>
        <a class="btn btn--quiet" href={resolve('/deteksi')}>{tx(UI.quiz.check)}</a>
      </div>
    </div>
  {/if}
</div>

<style>
  .quiz { padding: clamp(1.25rem, 3vw, 2rem); max-width: 46rem; }
  .progress { display: grid; gap: var(--space-2); margin: 0 0 var(--space-4); font-weight: 700; font-size: var(--text-sm); color: var(--body-2); }
  .bar { display: block; height: 6px; border-radius: var(--radius-pill); background: var(--border); overflow: hidden; }
  .bar span { display: block; height: 100%; background: var(--brand); border-radius: inherit; transform-origin: left; transition: transform var(--dur-base) var(--ease-out); }

  .q { border: 0; margin: 0; padding: 0; min-width: 0; }
  legend {
    padding: 0;
    margin-bottom: var(--space-4);
    font-family: var(--font-display);
    font-weight: 800;
    font-size: var(--text-xl);
    line-height: 1.25;
  }
  legend:focus { outline: none; }
  legend:focus-visible { outline: var(--focus-ring); outline-offset: 4px; }

  .options { display: grid; gap: var(--space-2); }
  .option {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-height: 56px;
    padding: var(--space-3) var(--space-4);
    border: 2px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--paper);
    color: var(--ink);
    cursor: pointer;
  }
  .option:hover { border-color: var(--border-strong); background: var(--indigo-tint); }
  .option:has(input:checked) { border-color: var(--brand); }
  .option:has(input:focus-visible) { outline: var(--focus-ring); outline-offset: 3px; }
  .option input { width: 22px; height: 22px; accent-color: var(--brand); flex: none; }
  .option .text { flex: 1; min-width: 0; }
  .option.right { background: var(--safe); color: var(--on-fill); border-color: var(--border-strong); }
  .option.miss { background: var(--coral-tint); border-style: dashed; border-color: var(--err); }
  fieldset:disabled .option { cursor: default; }
  .tag {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 10px;
    border: 1px solid currentColor;
    border-radius: var(--radius-pill);
    font-size: var(--text-xs);
    font-weight: 700;
    white-space: nowrap;
  }

  .missing { min-height: 1.6rem; margin: var(--space-2) 0 0; display: flex; align-items: center; gap: var(--space-2); color: var(--err); font-weight: 700; font-size: var(--text-sm); }
  .feedback { margin: 0 0 var(--space-4); }
  .feedback.filled { padding: var(--space-3) var(--space-4); background: var(--glass-bg-strong); border: 1px solid var(--glass-edge); border-radius: var(--radius-md); box-shadow: var(--glass-highlight); }
  .feedback p { margin: 0; }
  .fb-head { margin-bottom: var(--space-1) !important; }
  .explain { color: var(--body-2); }

  .result h3 { font-size: var(--text-xl); margin-bottom: var(--space-2); }
  .result h3:focus { outline: none; }
  .big { margin: 0; font-family: var(--font-display); font-weight: 800; font-size: var(--text-2xl); letter-spacing: -0.02em; }
  .msg { margin: var(--space-2) 0 var(--space-5); color: var(--body-2); max-width: 36em; }
  @media (max-width: 40rem) {
    .btn, .row .btn { width: 100%; }
  }
</style>
