/**
 * Interface copy. AMBA speaks Indonesian only (Sir, 10 Nov 2026): the English
 * interface was removed, so there is one dictionary and no language switch.
 * `t()` stays the single entry point so copy lives in one file.
 */
import { id, type MessageKey } from './id';

/**
 * Looks a key up. `{name}` placeholders are filled from params.
 */
export function t(key: MessageKey, params?: Record<string, string | number>): string {
  let text: string = id[key] ?? key;
  if (params) {
    for (const [name, value] of Object.entries(params)) {
      text = text.replaceAll(`{${name}}`, String(value));
    }
  }
  return text;
}

/** BCP 47 tag for Intl formatting and speech synthesis. */
export function bcp47(): string {
  return 'id-ID';
}

export type { MessageKey };
