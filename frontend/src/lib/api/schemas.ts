import { z } from 'zod';
import { SchemaVersionError, ContractError } from './errors';

/**
 * Zod mirrors of api/dossier.schema.json. A contract change updates the OpenAPI
 * file, the JSON Schema, the Go structs and this file in the same commit.
 */

export const statusSchema = z.enum(['SAFE', 'SUSPICIOUS', 'TAMPERED', 'INSUFFICIENT']);
export const factStatusSchema = z.enum(['CONTRADICTS', 'SUPPORTS', 'UNVERIFIED']);
export const inputTypeSchema = z.enum(['URL', 'FILE']);

export const tacticCodeSchema = z.enum([
  'URGENCY',
  'REWARD_BAIT',
  'FALSE_AUTHORITY',
  'FEAR',
  'SECRECY',
  'OUTRAGE_BAIT',
  'IMPERSONATION',
  'FAKE_SOCIAL_PROOF'
]);

export const anomalySchema = z.object({
  bounding_box: z.tuple([z.number(), z.number(), z.number(), z.number()]),
  extracted_text: z.string().optional(),
  reason: z.string().optional(),
  // Timpa Teks: 0-100, null when it could not be measured; older payloads
  // omit it.
  evidence: z.number().int().min(0).max(100).nullish().transform((v) => v ?? null)
});

export const sourceSchema = z.object({
  title: z.string(),
  publisher: z.string(),
  url: z.string(),
  match: factStatusSchema
});

export const tacticSchema = z.object({
  code: tacticCodeSchema,
  phrase: z.string(),
  source: z.enum(['text', 'ocr'])
});

export const metadataStatusSchema = z.enum(['OK', 'BLOCKED', 'UNAVAILABLE']);

/** What the image declares about its origin (Jejak AI). */
export const provenanceSchema = z.object({
  kind: z.enum(['AI_GENERATED', 'AI_EDITED', 'CAMERA', 'NONE']),
  source: z.enum(['c2pa', 'xmp', 'png_text', 'exif', 'watermark', 'creator', 'web', '']),
  generator: z.string(),
  camera: z.string(),
  c2pa: z.boolean()
});

export const aiDetectorSchema = z.object({
  name: z.string(),
  score: z.number().int().min(0).max(100)
});

/** Public description of a submitted link (PRD F1). null for FILE. */
export const sourceMetadataSchema = z.object({
  status: metadataStatusSchema,
  platform: z.string(),
  via: z.string(),
  title: z.string(),
  author: z.string(),
  description: z.string(),
  site_name: z.string()
});

const videoSignalSchema = z.object({
  score: z.number().int().min(0).max(100),
  indicators: z.array(z.string()).nullish().transform((v) => v ?? [])
});

/** What the creator says about AI in the post (older servers omit it). */
const creatorDisclosureSchema = z
  .object({ level: z.enum(['none', 'weak', 'strong']), phrase: z.string() })
  .nullish();

/** A reputable page about the video; only https links may become an href. */
const webEvidenceSchema = z.object({
  title: z.string(),
  url: z.string().startsWith('https://'),
  publisher: z.string(),
  says: z.enum(['AI', 'REAL', 'HOAX'])
});

/** What the video shows, as 5W1H (Sir, 10 Nov 2026). Each field may be ''. */
const w5h1Schema = z.object({
  what: z.string().catch(''),
  who: z.string().catch(''),
  when: z.string().catch(''),
  where: z.string().catch(''),
  why: z.string().catch(''),
  how: z.string().catch('')
});

/** One trusted-source search result behind the conclusion. A bad field never
 * fails the whole result: it falls back to '' and the UI drops what it can't
 * show (only http(s) links become an href). */
const researchSourceSchema = z.object({
  title: z.string().catch(''),
  url: z.string().catch(''),
  domain: z.string().catch(''),
  snippet: z.string().catch(''),
  published: z.string().catch('')
});

/** The web search on trusted domains for what the video shows. */
const researchSchema = z.object({
  query: z.string().catch(''),
  status: z.enum(['FOUND', 'NONE', 'UNAVAILABLE']).catch('NONE'),
  sources: z.array(researchSourceSchema).nullish().catch([]).transform((v) => v ?? [])
});

export type VideoW5H1 = z.infer<typeof w5h1Schema>;
export type VideoResearch = z.infer<typeof researchSchema>;

/** Frame-by-frame analysis of a social video link. Absent for other input. */
export const videoSchema = z.object({
  status: z.enum(['OK', 'UNAVAILABLE']),
  reason: z.string(),
  platform: z.string(),
  duration_sec: z.number().min(0),
  upload_date: z.string(),
  platform_ai_label: z.boolean(),
  frames: z
    .array(
      z.object({
        at_sec: z.number().min(0),
        // Only our own JPEG thumbnails may become an <img src>.
        thumb: z.string().startsWith('data:image/jpeg;base64,')
      })
    )
    .nullish()
    .transform((v) => v ?? []),
  audio_analyzed: z.boolean(),
  summary: z.string(),
  transcript: z.string(),
  manipulation: videoSignalSchema,
  context: videoSignalSchema,
  creator_disclosure: creatorDisclosureSchema,
  web_evidence: z.array(webEvidenceSchema).nullish(),
  // Older servers omit both.
  w5h1: w5h1Schema.nullish().catch(null).transform((v) => v ?? null),
  research: researchSchema.nullish().catch(null).transform((v) => v ?? null)
});

export const verdictSchema = z.object({
  score: z.number().int().min(0).max(100).nullable(),
  status: statusSchema,
  short_title: z.string(),
  explanation: z.string().max(400),
  explanation_simple: z.string().max(140),
  basis: z.array(z.enum(['ela', 'ai', 'fact', 'link', 'llm_untrusted'])),
  not_checked: z.array(z.enum(['ela', 'ai', 'fact', 'link'])),
  floors_applied: z.array(z.string())
});

export const dossierSchema = z.object({
  schema_version: z.literal(2),
  id: z.string(),
  timestamp: z.string(),
  input_type: inputTypeSchema,
  source_reference: z.string(),
  degraded: z.boolean(),
  is_demo: z.boolean(),
  verdict: verdictSchema,
  pillars: z.object({
    ela: z.object({
      available: z.boolean(),
      score: z.number(),
      reliability: z.number().min(0).max(1),
      reliability_note: z.string().nullish(),
      // The server serialises an empty Go slice as null. Accept both and
      // normalise here so callers only ever see an array.
      anomalies: z.array(anomalySchema).nullish().transform((v) => v ?? [])
    }),
    ai: z.object({
      available: z.boolean(),
      score: z.number(),
      // Added in the same schema_version for the AI-image check: payloads
      // and share links made before it omit these fields.
      reliability: z.number().min(0).max(1).optional(),
      indicators: z.array(z.string()).nullish().transform((v) => v ?? []),
      caveat: z.string().optional(),
      provenance: provenanceSchema.nullish().transform((v) => v ?? null),
      detector: aiDetectorSchema.nullish().transform((v) => v ?? null)
    }),
    fact: z.object({
      available: z.boolean(),
      claim: z.string().optional(),
      // An unavailable fact pillar serialises status as "". Treat that as
      // UNVERIFIED rather than failing the whole dossier.
      status: z
        .union([factStatusSchema, z.literal('')])
        .transform((v) => (v === '' ? ('UNVERIFIED' as const) : v)),
      sources: z.array(sourceSchema).nullish().transform((v) => v ?? [])
    }),
    link: z.object({
      available: z.boolean(),
      score: z.number(),
      final_url: z.string().optional(),
      redirect_chain: z.array(z.string()).nullish().transform((v) => v ?? []),
      reasons: z
        .array(z.object({ code: z.string(), text: z.string() }))
        .nullish()
        .transform((v) => v ?? [])
    })
  }),
  tactics: z.array(tacticSchema),
  share_token: z.string(),
  // Added in the same schema_version: older payloads (and bundled demos built
  // before the field existed) simply omit them.
  source_metadata: sourceMetadataSchema.nullish().transform((v) => v ?? null),
  ocr_text: z.string().max(2000).nullish().transform((v) => v ?? ''),
  // Social video links only; older payloads and other input omit it.
  video: videoSchema.nullish().transform((v) => v ?? null)
});

export const errorEnvelopeSchema = z.object({
  error: z.object({
    code: z.string(),
    message: z.string(),
    retryable: z.boolean()
  })
});

export type DossierParsed = z.infer<typeof dossierSchema>;

export { SchemaVersionError, ContractError };

/**
 * Parses an unknown payload into a Dossier. A schema_version this build does
 * not know produces a distinct error so the UI can say "Perbarui aplikasi"
 * instead of showing a generic failure.
 */
export function parseDossier(payload: unknown): DossierParsed {
  if (
    typeof payload === 'object' &&
    payload !== null &&
    'schema_version' in payload &&
    (payload as { schema_version: unknown }).schema_version !== 2
  ) {
    throw new SchemaVersionError();
  }
  const parsed = dossierSchema.safeParse(payload);
  if (!parsed.success) {
    throw new ContractError(
      parsed.error.issues.map((i) => `${i.path.join('.') || '(root)'}: ${i.message}`)
    );
  }
  return parsed.data;
}
