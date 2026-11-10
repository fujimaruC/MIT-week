import { bcp47 } from './i18n/index.svelte';

/**
 * Accessibility helpers that are not tied to any visual mode: read-aloud.
 */

/** Read-aloud uses the browser's own speech synthesis, Indonesian voice. */
export function speechAvailable(): boolean {
  return typeof window !== 'undefined' && 'speechSynthesis' in window;
}

export function speak(text: string): void {
  if (!speechAvailable()) return;
  window.speechSynthesis.cancel();
  const utterance = new SpeechSynthesisUtterance(text);
  utterance.lang = bcp47();
  utterance.rate = 0.95;
  window.speechSynthesis.speak(utterance);
}

export function stopSpeaking(): void {
  if (!speechAvailable()) return;
  window.speechSynthesis.cancel();
}

/**
 * Auto-read: when on, a new result speaks its lead sentence and explanation
 * as soon as it appears (PRD F14). Independent of presentation mode and off
 * by default: unexpected audio is not acceptable (DESIGN.md).
 */
const AUTO_READ_KEY = 'amba.autoRead';
let autoReadOn = $state(false);
let autoReadLoaded = false;

export const autoRead = {
  get value(): boolean {
    if (!autoReadLoaded) {
      autoReadLoaded = true;
      try {
        autoReadOn = localStorage.getItem(AUTO_READ_KEY) === '1';
      } catch {
        autoReadOn = false;
      }
    }
    return autoReadOn;
  },
  set(next: boolean): void {
    autoReadOn = next;
    autoReadLoaded = true;
    try {
      localStorage.setItem(AUTO_READ_KEY, next ? '1' : '0');
    } catch {
      // Session-only when storage is blocked.
    }
  }
};
