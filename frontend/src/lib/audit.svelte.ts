import { readSSE, type SSEEvent } from './sse';
import { SchemaVersionError, ContractError } from './api/errors';
import type { DossierParsed } from './api/schemas';
import { API_BASE, isMockMode } from './api/config';
import type { MockKind, MockPayload } from './api/mock';
import { result } from './result.svelte';
import { t } from './i18n/index.svelte';

/**
 * Zod (about 25 KB gz) and the mock stream load on demand: neither is needed
 * to paint the form, and keeping them out of the first task keeps the page
 * responsive on a mid-range phone. The import starts with the audit, so the
 * parser is ready long before the verdict arrives.
 */
const loadSchemas = () => import('./api/schemas');

export type PillarKey = 'ela' | 'ai' | 'fact' | 'link';
export type StageStatus = 'idle' | 'running' | 'done' | 'skipped' | 'failed';

export type AuditErrorKind =
  | 'offline'
  | 'rate_limited'
  | 'timeout'
  | 'degraded'
  | 'unsupported'
  | 'schema'
  | 'network';

export interface AuditError {
  kind: AuditErrorKind;
  message: string;
  retryAfter?: number;
}

/**
 * One audit session: owns the abort controller, the stage map, partial pillars
 * and the final verdict. Nothing is persisted.
 */
export class AuditSession {
  stages = $state<Record<PillarKey, StageStatus>>({
    ela: 'idle',
    ai: 'idle',
    fact: 'idle',
    link: 'idle'
  });
  verdict = $state<DossierParsed | null>(null);
  error = $state<AuditError | null>(null);
  running = $state(false);
  /** Coalesced, human-readable stage line for the polite live region. */
  announcement = $state('');
  /**
   * Pillar payloads as they arrive, before the final verdict exists. Keyed by
   * pillar so a late partial never clobbers one already shown.
   */
  partials = $state<Partial<Record<PillarKey, unknown>>>({});

  #controller: AbortController | null = null;
  #announceTimer: ReturnType<typeof setTimeout> | null = null;
  /** Distinguishes a user cancellation from a real network failure. */
  #aborted = false;
  /** Held only so the verdict handler can hand it to the result store. */
  #imageFile: File | null = null;

  get done(): boolean {
    return this.verdict !== null || this.error !== null;
  }

  abort(): void {
    this.#aborted = true;
    this.#controller?.abort();
    this.#controller = null;
    this.running = false;
  }

  /** Runs an audit. Retries exactly once on a network error, never on 4xx. */
  async start(kind: 'url' | 'file', payload: { url?: string; file?: File }): Promise<void> {
    this.abort();
    this.#aborted = false;
    this.verdict = null;
    this.error = null;
    this.running = true;
    this.#imageFile = payload.file ?? null;
    this.stages = { ela: 'idle', ai: 'idle', fact: 'idle', link: 'idle' };
    this.partials = {};
    void loadSchemas().catch(() => {
      // Retried by the verdict handler; a failure there is a network error.
    });

    if (typeof navigator !== 'undefined' && navigator.onLine === false) {
      this.error = { kind: 'offline', message: t('err.offline') };
      this.running = false;
      return;
    }

    let attempt = 0;
    while (attempt < 2) {
      try {
        await this.#run(kind, payload);
        this.running = false;
        return;
      } catch (err) {
        if (this.#aborted) {
          this.running = false;
          return;
        }
        if (err instanceof SchemaVersionError) {
          this.error = { kind: 'schema', message: t('err.update') };
          this.running = false;
          return;
        }
        if (err instanceof ContractError) {
          // The server answered, but not in a shape this build understands.
          // Retrying cannot fix that, so say so instead of "try again".
          this.error = {
            kind: 'schema',
            message: t('err.format')
          };
          this.running = false;
          return;
        }
        if (err instanceof ApiFailure) {
          this.error = err.auditError;
          this.running = false;
          return;
        }
        if (attempt === 0) {
          attempt++;
          continue; // one retry on a network-level failure
        }
        this.error = {
          kind: 'network',
          message: t('err.stopped')
        };
        this.running = false;
        return;
      }
    }
  }

  async #run(kind: 'url' | 'file', payload: { url?: string; file?: File }): Promise<void> {
    this.#controller = new AbortController();
    const { signal } = this.#controller;

    // Frontend-only preview: same event contract, no server required.
    if (isMockMode) {
      const { mockAuditStream } = await import('./api/mock');
      for await (const event of mockAuditStream(kind as MockKind, payload as MockPayload, signal)) {
        await this.#apply(event);
        if (event.type === 'done') return;
      }
      return;
    }

    const init: RequestInit = {
      method: 'POST',
      headers: { Accept: 'text/event-stream' },
      signal
    };

    if (kind === 'file' && payload.file) {
      const form = new FormData();
      form.append('file', payload.file);
      init.body = form;
    } else {
      init.headers = { ...init.headers, 'Content-Type': 'application/json' };
      init.body = JSON.stringify({ url: payload.url });
    }

    const response = await fetch(`${API_BASE}/audit/${kind}`, init);

    if (!response.ok) {
      throw await this.#failure(response);
    }
    if (!response.body) {
      throw new ApiFailure({ kind: 'network', message: t('err.stopped') });
    }

    for await (const event of readSSE(response, signal)) {
      await this.#apply(event);
      if (event.type === 'done') return;
    }
  }

  async #failure(response: Response): Promise<ApiFailure> {
    let message = t('err.generic');
    try {
      const body = (await response.json()) as { error?: { message?: string } };
      // Server messages are Indonesian; in English keep the local copy.
      if (body.error?.message) message = body.error.message;
    } catch {
      // Keep the generic message; never surface raw server text.
    }

    if (response.status === 429) {
      const retry = Number(response.headers.get('Retry-After') ?? '0');
      return new ApiFailure({
        kind: 'rate_limited',
        message: t('err.rate', { n: retry || 30 }),
        retryAfter: retry || 30
      });
    }
    if (response.status === 504) {
      // No verdict exists on a 504; the UI shows retry only.
      return new ApiFailure({ kind: 'timeout', message: t('err.timeout') });
    }
    if (response.status === 415 || response.status === 413) {
      return new ApiFailure({ kind: 'unsupported', message });
    }
    return new ApiFailure({ kind: 'network', message });
  }

  async #apply(event: SSEEvent): Promise<void> {
    switch (event.type) {
      case 'stage':
        this.stages = { ...this.stages, [event.pillar]: event.status };
        this.#announce(event);
        break;
      case 'verdict': {
        const { parseDossier } = await loadSchemas();
        this.verdict = parseDossier(event.data);
        result.set(this.verdict, this.#imageFile);
        if (this.verdict.degraded) {
          this.announcement = t('announce.degraded');
        }
        break;
      }
      case 'error':
        this.error = { kind: 'network', message: t('err.stopped') };
        break;
      case 'partial':
        this.partials = { ...this.partials, [event.pillar]: event.data };
        break;
      case 'done':
        break;
    }
  }

  /** Coalesces stage announcements so a fast stream does not spam a screen reader. */
  #announce(event: Extract<SSEEvent, { type: 'stage' }>): void {
    if (this.#announceTimer) clearTimeout(this.#announceTimer);
    this.#announceTimer = setTimeout(() => {
      const name = t(`pillar.${event.pillar}`);
      if (event.status === 'running') {
        this.announcement = t('announce.running', { name: name.toLowerCase() });
      } else if (event.status === 'done') {
        this.announcement = t('announce.done', { name });
      } else if (event.status === 'skipped' || event.status === 'failed') {
        this.announcement = t('announce.failed', { name });
      }
    }, 250);
  }
}

class ApiFailure extends Error {
  auditError: AuditError;
  constructor(auditError: AuditError) {
    super(auditError.message);
    this.name = 'ApiFailure';
    this.auditError = auditError;
  }
}
