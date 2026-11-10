/**
 * Server-written copy. The API speaks Indonesian (TECH_SPEC §2) and the
 * interface is Indonesian only, so the server's own sentences are shown
 * unchanged; only the parts the page builds from codes go through t().
 */
import type { DossierParsed } from '$lib/api/schemas';
import { t, type MessageKey } from './index.svelte';

const has = (key: string): key is MessageKey => t(key as MessageKey) !== key;

/** TECH_SPEC §8.2 deterministic lead sentence (scoring.ExplanationSimple). */
export function simpleSentence(d: DossierParsed): string {
  // Old payloads can still carry INSUFFICIENT. It is shown as a neutral
  // conclusion, never as "not enough data", so the server's sentence is not used.
  if (d.verdict.status === 'INSUFFICIENT') return t('simple.INSUFFICIENT');
  return d.verdict.explanation_simple;
}

/** The server's sentence that lists what could not be checked is dropped: the page shows what the result is based on instead. */
export function withoutNotChecked(text: string): string {
  return text
    .split(/(?<=[.!?])\s+/)
    .filter((sentence) => !/tidak dapat diperiksa/i.test(sentence))
    .join(' ')
    .trim();
}

export function explanation(d: DossierParsed): string {
  return withoutNotChecked(d.verdict.explanation);
}

/** A link check reason, in the server's words. */
export function linkReason(serverText: string): string {
  return serverText;
}

export function caveat(serverText: string): string {
  return serverText;
}

export function reliabilityNote(serverText: string | null | undefined): string {
  return serverText ?? '';
}

type AIPillar = DossierParsed['pillars']['ai'];

/** The AI pillar's score bands, shared with the Telegram summary. */
export function aiLevel(score: number): 'high' | 'medium' | 'low' {
  return score >= 70 ? 'high' : score >= 40 ? 'medium' : 'low';
}

/**
 * The Jejak AI card in plain words: which image was checked, what the file
 * declares about its origin (or how likely it looks AI-made), and the
 * dedicated detector's answer when one ran. `fromVideo`: the score came from
 * the frames of an analysed social video, not from the link's preview image.
 */
export function aiLines(ai: AIPillar, inputType: DossierParsed['input_type'], fromVideo = false): string[] {
  const lines: string[] = [];
  if (inputType === 'URL') lines.push(t(fromVideo ? 'video.aiChecked' : 'ai.preview'));
  const p = ai.provenance;
  if (p && (p.kind === 'AI_GENERATED' || p.kind === 'AI_EDITED')) {
    const made = p.kind === 'AI_EDITED' ? 'edited' : 'generated';
    const by = p.generator ? ` (${p.generator})` : '';
    const key = `ai.prov.${made}.${p.source}`;
    lines.push(has(key) ? t(key, { by }) : t(`ai.prov.${made}.xmp` as MessageKey, { by }));
  } else {
    lines.push(t(`ai.level.${aiLevel(ai.score)}` as MessageKey));
    if (p?.kind === 'CAMERA') {
      lines.push(p.camera ? t('ai.prov.camera', { camera: p.camera }) : t('ai.prov.cameraC2pa'));
    } else if (p?.c2pa) {
      lines.push(t('ai.prov.c2paNoAI'));
    } else if (p) {
      lines.push(t('ai.prov.none'));
    }
  }
  if (ai.detector) {
    const name = ai.detector.name.charAt(0).toUpperCase() + ai.detector.name.slice(1);
    lines.push(t('ai.detector', { name, n: ai.detector.score }));
  }
  return lines;
}

/** "Berdasarkan: jejak AI, keamanan tautan." Only what ran. */
export function basisList(d: DossierParsed): string {
  return d.verdict.basis
    .filter((b) => b !== 'llm_untrusted')
    .map((b) => t(`basis.${b}` as MessageKey))
    .join(', ');
}
