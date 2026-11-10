/**
 * The checks of this browser session, for the dashboard's list. Held in
 * memory only, like the result itself (CLAUDE.md rule 1: nothing is
 * persisted): a reload starts an empty list.
 */
import type { DossierParsed } from './api/schemas';
import type { Status } from './api/types';

export interface SessionCheck {
  id: string;
  at: number;
  kind: 'video' | 'link' | 'image';
  label: string;
  /** The checked link, for "check again"; empty for an upload. */
  url: string;
  status: Status;
  score: number | null;
  demo: boolean;
}

const MAX = 20;
let entries = $state<SessionCheck[]>([]);

function labelOf(d: DossierParsed, fileName: string | null): string {
  if (d.input_type === 'FILE') return fileName || 'Gambar';
  const md = d.source_metadata;
  if (md?.title) return md.title;
  try {
    return new URL(d.source_reference).hostname.replace(/^www\./, '');
  } catch {
    return d.source_reference;
  }
}

export const sessionHistory = {
  get list(): SessionCheck[] {
    return entries;
  },
  add(d: DossierParsed, fileName: string | null = null): void {
    if (entries.some((e) => e.id === d.id)) return;
    const kind = d.input_type === 'FILE' ? 'image' : d.video?.status === 'OK' ? 'video' : 'link';
    const entry: SessionCheck = {
      id: d.id,
      at: Date.now(),
      kind,
      label: labelOf(d, fileName),
      url: d.input_type === 'URL' && /^https:\/\//.test(d.source_reference) ? d.source_reference : '',
      status: d.verdict.status,
      score: d.verdict.score,
      demo: d.is_demo
    };
    entries = [entry, ...entries].slice(0, MAX);
  },
  clear(): void {
    entries = [];
  }
};
