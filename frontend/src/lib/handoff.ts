/**
 * Hands an image picked in the prompt bar on one page to /deteksi, which
 * runs the check. A module variable is enough: the hand-over happens inside
 * one client-side navigation and the file is taken (and cleared) on arrival.
 */
let pendingFile: File | null = null;

export function handOver(file: File): void {
  pendingFile = file;
}

export function takeHandedFile(): File | null {
  const file = pendingFile;
  pendingFile = null;
  return file;
}

/** The upload limits the server enforces, checked before the hand-over. */
export const MAX_IMAGE_BYTES = 10 * 1024 * 1024;
export const ACCEPTED_IMAGES = ['image/jpeg', 'image/png', 'image/webp'];

export function imageProblem(file: File): 'unsupported' | 'tooLarge' | null {
  if (!ACCEPTED_IMAGES.includes(file.type)) return 'unsupported';
  if (file.size > MAX_IMAGE_BYTES) return 'tooLarge';
  return null;
}
