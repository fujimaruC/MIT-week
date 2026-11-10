/**
 * Contract errors, kept apart from the Zod schemas so code that only needs to
 * recognise them (the audit session) does not load Zod on first paint.
 */

/** Raised when the server speaks a newer contract than this build understands. */
export class SchemaVersionError extends Error {
  constructor() {
    super('Perbarui aplikasi');
    this.name = 'SchemaVersionError';
  }
}

/**
 * Raised when a payload does not match the contract at all. Kept separate from
 * a network failure so the UI can say "this is our problem" instead of
 * "try again" for something retrying will never fix.
 */
export class ContractError extends Error {
  readonly issues: string[];
  constructor(issues: string[]) {
    super('Payload does not match the Dossier contract');
    this.name = 'ContractError';
    this.issues = issues;
  }
}
