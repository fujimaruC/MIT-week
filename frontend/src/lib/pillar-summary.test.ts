import { describe, it, expect } from 'vitest';
import { pillarSummary } from './pillar-summary';

describe('pillarSummary', () => {
  it('says unavailable rather than showing a zero score', () => {
    expect(pillarSummary('ela', { available: false, score: 0 })).toBe(
      'Tidak dipakai untuk konten ini.'
    );
  });

  it('reports an AI score', () => {
    expect(pillarSummary('ai', { available: true, score: 74 })).toBe('Skor indikatif 74.');
  });

  it('reports link score', () => {
    expect(pillarSummary('link', { available: true, score: 88 })).toBe('Skor tautan 88.');
  });

  it('reports ELA reliability as a percentage', () => {
    expect(pillarSummary('ela', { available: true, reliability: 0.55 })).toBe('Keandalan 55%.');
  });

  it('maps each fact status to plain language', () => {
    expect(pillarSummary('fact', { available: true, status: 'CONTRADICTS' })).toBe(
      'Klaim dibantah sumber terverifikasi.'
    );
    expect(pillarSummary('fact', { available: true, status: 'UNVERIFIED' })).toBe(
      'Belum ada sumber terverifikasi.'
    );
  });

  it('returns null rather than guessing', () => {
    expect(pillarSummary('ai', { available: true })).toBeNull();
    expect(pillarSummary('fact', { available: true, status: 'WEIRD' })).toBeNull();
    expect(pillarSummary('ela', null)).toBeNull();
    expect(pillarSummary('ela', 'not an object')).toBeNull();
  });
});
