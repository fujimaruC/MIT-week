import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import Quiz from './Quiz.svelte';
import { QUESTIONS } from '$lib/data/belajar';

async function answerCurrent(optionId: string, last = false): Promise<void> {
  const radios = screen.getAllByRole('radio') as HTMLInputElement[];
  await fireEvent.click(radios.find((r) => r.value === optionId)!);
  await fireEvent.click(screen.getByRole('button', { name: 'Jawab' }));
  await fireEvent.click(screen.getByRole('button', { name: last ? 'Lihat nilai' : 'Pertanyaan berikutnya' }));
}

describe('Quiz', () => {
  it('has between five and eight questions, each with a valid answer and an explanation', () => {
    expect(QUESTIONS.length).toBeGreaterThanOrEqual(5);
    expect(QUESTIONS.length).toBeLessThanOrEqual(8);
    for (const q of QUESTIONS) {
      expect(q.options.some((o) => o.id === q.answer)).toBe(true);
      expect(q.explain.id.length).toBeGreaterThan(10);
    }
  });

  it('asks for an answer before it accepts one', async () => {
    render(Quiz);
    await fireEvent.click(screen.getByRole('button', { name: 'Jawab' }));
    expect(screen.getByRole('alert').textContent).toContain('Pilih satu jawaban dulu.');
    expect(screen.getByText(`Pertanyaan 1 dari ${QUESTIONS.length}`)).toBeTruthy();
  });

  it('explains a wrong answer, names the right one, and scores at the end', async () => {
    render(Quiz);
    const wrong = QUESTIONS[0].options.find((o) => o.id !== QUESTIONS[0].answer)!;
    await fireEvent.click((screen.getAllByRole('radio') as HTMLInputElement[]).find((r) => r.value === wrong.id)!);
    await fireEvent.click(screen.getByRole('button', { name: 'Jawab' }));
    const status = screen.getByRole('status');
    expect(status.textContent).toContain('Kurang tepat.');
    expect(status.textContent).toContain('Jawaban yang tepat:');
    expect(status.textContent).toContain(QUESTIONS[0].explain.id);
    await fireEvent.click(screen.getByRole('button', { name: 'Pertanyaan berikutnya' }));

    for (let i = 1; i < QUESTIONS.length; i++) {
      await answerCurrent(QUESTIONS[i].answer, i === QUESTIONS.length - 1);
    }
    expect(screen.getByTestId('quiz-result').textContent).toContain(
      `${QUESTIONS.length - 1} dari ${QUESTIONS.length} benar`
    );
    await fireEvent.click(screen.getByRole('button', { name: 'Ulangi kuis' }));
    expect(screen.getByText(`Pertanyaan 1 dari ${QUESTIONS.length}`)).toBeTruthy();
  });
});
