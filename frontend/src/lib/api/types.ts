/**
 * Mirrors api/openapi.yaml. Keep in sync in the same commit as any contract change.
 */
export type Status = 'SAFE' | 'SUSPICIOUS' | 'TAMPERED' | 'INSUFFICIENT';
export type FactStatus = 'CONTRADICTS' | 'SUPPORTS' | 'UNVERIFIED';
export type InputType = 'URL' | 'FILE';
export type TacticCode =
  | 'URGENCY'
  | 'REWARD_BAIT'
  | 'FALSE_AUTHORITY'
  | 'FEAR'
  | 'SECRECY'
  | 'OUTRAGE_BAIT'
  | 'IMPERSONATION'
  | 'FAKE_SOCIAL_PROOF';

export interface Anomaly {
  bounding_box: [number, number, number, number];
  extracted_text?: string;
  reason?: string;
  /** Timpa Teks: 0-100; null when it could not be measured. */
  evidence?: number | null;
}

export interface Source {
  title: string;
  publisher: string;
  url: string;
  match: FactStatus;
}

export interface Tactic {
  code: TacticCode;
  phrase: string;
  source: 'text' | 'ocr';
}

export type MetadataStatus = 'OK' | 'BLOCKED' | 'UNAVAILABLE';

export interface SourceMetadata {
  status: MetadataStatus;
  platform: string;
  via: string;
  title: string;
  author: string;
  description: string;
  site_name: string;
}

export interface Verdict {
  score: number | null;
  status: Status;
  short_title: string;
  explanation: string;
  explanation_simple: string;
  basis: Array<'ela' | 'ai' | 'fact' | 'link' | 'llm_untrusted'>;
  not_checked: Array<'ela' | 'ai' | 'fact' | 'link'>;
  floors_applied: string[];
}

/** What the image declares about its origin (Jejak AI). */
export type ProvenanceKind = 'AI_GENERATED' | 'AI_EDITED' | 'CAMERA' | 'NONE';
export type ProvenanceSource = 'c2pa' | 'xmp' | 'png_text' | 'exif' | 'watermark' | 'creator' | 'web' | '';

export interface Provenance {
  kind: ProvenanceKind;
  source: ProvenanceSource;
  generator: string;
  camera: string;
  c2pa: boolean;
}

export interface AIDetector {
  name: string;
  score: number;
}

export interface Dossier {
  schema_version: 2;
  id: string;
  timestamp: string;
  input_type: InputType;
  source_reference: string;
  degraded: boolean;
  is_demo: boolean;
  verdict: Verdict;
  pillars: {
    ela: { available: boolean; score: number; reliability: number; reliability_note: string | null; anomalies: Anomaly[] };
    ai: {
      available: boolean;
      score: number;
      reliability?: number;
      indicators: string[];
      caveat: string;
      provenance?: Provenance | null;
      detector?: AIDetector | null;
    };
    fact: { available: boolean; claim: string; status: FactStatus; sources: Source[] };
    link: { available: boolean; score: number; final_url: string; redirect_chain: string[]; reasons: { code: string; text: string }[] };
  };
  tactics: Tactic[];
  share_token: string;
  source_metadata?: SourceMetadata | null;
  ocr_text?: string;
  video?: VideoAnalysis | null;
}

export interface VideoSignal {
  score: number;
  indicators: string[];
}

export type CreatorDisclosureLevel = 'none' | 'weak' | 'strong';

export interface WebEvidence {
  title: string;
  url: string;
  publisher: string;
  says: 'AI' | 'REAL' | 'HOAX';
}

/** Frame-by-frame analysis of a social video link; its AI score is in pillars.ai. */
export interface VideoAnalysis {
  status: 'OK' | 'UNAVAILABLE';
  reason: string;
  platform: string;
  duration_sec: number;
  upload_date: string;
  platform_ai_label: boolean;
  frames: { at_sec: number; thumb: string }[];
  audio_analyzed: boolean;
  summary: string;
  transcript: string;
  manipulation: VideoSignal;
  context: VideoSignal;
  /** Absent when the server predates the field. */
  creator_disclosure?: { level: CreatorDisclosureLevel; phrase: string } | null;
  web_evidence?: WebEvidence[] | null;
}

export interface ErrorEnvelope {
  error: { code: string; message: string; retryable: boolean };
}
