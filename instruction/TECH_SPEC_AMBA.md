# AMBA — Technical Specification v10.2
Companion to `PRD_AMBA_v10.md`. This file is the source of truth for contracts, algorithms and security. If it conflicts with v9 or v10.0, **this file wins**. v10.2 (10 Nov 2026) matches the `amba-v2` code: social video links with creator disclosure and web provenance, the one-gate link validator, recalibrated video thresholds, and the removal of Balas Sopan (`/reply`), share reports, proof cards, text size and presentation modes. Deployment is Docker Compose on a host behind a Cloudflare Tunnel (`docs/DEPLOY.md`).

---

## 1. Repository layout (monorepo)

```
amba/
├── CLAUDE.md
├── PRD_AMBA_v10.md  TECH_SPEC_AMBA.md  README.md
├── api/openapi.yaml                    # contract-first, written in Phase 0
├── configs/trusted_sources.yaml        # fact-check/news domain allowlist
├── backend/
│   ├── go.mod                          # module github.com/<org>/amba/backend
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── config/        # env parsing, validation
│   │   ├── httpapi/       # chi router, handlers, middleware, SSE writer
│   │   ├── models/        # Dossier, events (mirror api/openapi.yaml)
│   │   ├── scoring/       # PURE functions + table tests
│   │   ├── security/      # ssrf.go, urlcheck.go (link validator), headers, ratelimit, upload validation
│   │   ├── linkcheck/     # URL heuristics, redirect unrolling, safe browsing
│   │   ├── forensics/     # server-side ELA (image/jpeg re-encode diff)
│   │   ├── llm/           # OpenAI-compatible gateway client (primary + fallback, breaker), JSON-schema output, mock
│   │   ├── grounding/     # source verification vs allowlist
│   │   ├── pipeline/      # errgroup orchestration, deadlines, degradation
│   │   ├── token/         # HMAC signed verdict tokens
│   │   ├── video/         # yt-dlp + ffmpeg fetcher, platform recognition, creator disclosure (disclosure.go)
│   │   ├── retrieval/ pagefetch/ provenance/ aidetect/ telegram/ metrics/   # see README for each
│   │   ├── tactics/       # tactic enum, phrase substring verification
│   │   └── demo/          # //go:embed demo-cases.json
│   └── Makefile
└── frontend/
    ├── package.json  svelte.config.js  vite.config.ts  tailwind.config.ts
    ├── security-headers.json                      # single source for CSP etc. (host-specific emit deferred)
    ├── static/manifest.webmanifest  src/service-worker.ts  # PWA + share target (§8.1)
    └── src/
        ├── routes/ (+layout.svelte, +page.svelte, deteksi/, hasil/, edukasi/, tentang/)  # SvelteKit file routing; ssr=false, adapter-static fallback
        ├── lib/components/ (*.svelte: layout/, home/, deteksi/, hasil/, edukasi/, shared/, a11y/ReadAloud)
        ├── lib/ (api/, audit.svelte.ts [runes state + SSE], link-input.ts, sse.ts, forensics/ela.worker.ts, token.ts, a11y.ts, styles/tokens.css)
        ├── lib/data/demo-cases.json    # identical copy of backend demo data (CI diff check)
        ├── lib/data/quickcheck.json       # Cek cepat questions
        ├── lib/data/tactics.json          # static tactic labels + explanations (Indonesian)
        └── tests/
```

Use `internal/` (not `pkg/`) — nothing here is a public library.

---

## 2. API contract

Base: `/api/v1`. All JSON UTF-8. Errors use one envelope:

```json
{ "error": { "code": "INVALID_URL", "message": "Tautan tidak valid.", "retryable": false } }
```
Codes: `INVALID_URL, BLOCKED_TARGET, UNSUPPORTED_MEDIA, PAYLOAD_TOO_LARGE, RATE_LIMITED, UPSTREAM_TIMEOUT, INTERNAL`. `BLOCKED_TARGET` message is always generic (never reveals which rule fired). HTTP: 400/413/415/429/504/500 respectively.

| Route | Method | Body | Notes |
|---|---|---|---|
| `/audit/url` | POST | `{"url": string}` (≤2048) | |
| `/audit/file` | POST | multipart: `file`; optional `ocr_text` | ≤10 MB; client ELA score is **ignored** |
| `/verify` | POST | `{"token": string}` | returns `{valid, payload?}`. The token is still issued as `share_token`, but the UI no longer shows share reports |
| `/ops/status` | GET | – | integration status (live, disabled, placeholder); never returns a key, host or model output |
| `/demo/{id}` | GET | – | `^[a-z0-9-]{4,40}$` |
| `/healthz` | GET | – | no auth, no metrics data |

**Response modes** (same endpoint): `Accept: application/json` → single final `Dossier`; `Accept: text/event-stream` → SSE.

### SSE events
```
event: stage    data: {"pillar":"ela|ai|fact|link","status":"running|done|skipped|failed"}
event: partial  data: {"pillar":"fact", "data":{...pillar payload...}}
event: verdict  data: {Dossier}          # final, signed token included
event: error    data: {error envelope}
event: done     data: {}
```
Heartbeat comment `: ping` every 10 s. Frontend uses `fetch` + `ReadableStream` (POST SSE), not `EventSource`.

### Dossier v2 (Go structs ↔ JSON ↔ Zod; keep names stable)
```jsonc
{
  "schema_version": 2,
  "id": "AMBA-20261030-7F3K",          // date + 4 base32 chars
  "timestamp": "2026-10-30T07:22:15Z", // UTC ISO; UI formats to WITA
  "input_type": "URL|FILE",              // AMBA checks a link or an uploaded image (scope 2026-11-09)
  "source_reference": "string",         // display only; truncated 200 chars
  "degraded": false,
  "is_demo": false,
  "verdict": {
    "score": 92, "status": "TAMPERED|SUSPICIOUS|SAFE|INSUFFICIENT",
    "short_title": "string", "explanation": "string (<=400 chars, Indonesian)",
    "explanation_simple": "string (<=140 chars; deterministic template by status/floor, see §8.2)",
    "basis": ["ela","fact"], "not_checked": ["ai"],   // honest-limits data
    "floors_applied": ["FACT_CONTRADICTED"]
  },
  "pillars": {
    "ela":  {"available":true,"score":88,"reliability":1.0,"reliability_note":null,
             "anomalies":[{"bounding_box":[ymin,xmin,ymax,xmax],"extracted_text":"","reason":"","evidence":91}]},  // evidence: Timpa Teks 0-100, null if unmeasured
    "ai":   {"available":true,"score":95,"reliability":1.0,"indicators":["..."],"caveat":"Indikatif, bukan bukti pasti",
             "provenance":{"kind":"AI_GENERATED|AI_EDITED|CAMERA|NONE","source":"c2pa|xmp|png_text|exif|watermark|",
                           "generator":"OpenAI","camera":"","c2pa":true},
             "detector":{"name":"sightengine","score":97}},   // detector null unless configured and answered
    "fact": {"available":true,"claim":"...","status":"CONTRADICTS|SUPPORTS|UNVERIFIED",
             "sources":[{"title":"","publisher":"","url":"","match":"CONTRADICTS|SUPPORTS|UNVERIFIED"}]},
    "link": {"available":true,"score":71,"final_url":"","redirect_chain":["..."],
             "reasons":[{"code":"LOOKALIKE_BRAND","text":"Domain meniru nama BRI"}]}
  },
  "tactics": [ {"code":"URGENCY","phrase":"hari ini terakhir","source":"text|ocr"} ],  // label/explain come from static tactics.json
  "share_token": "v1.<payload>.<sig>"
}
```
Bounding boxes are normalized 0–1000 in `[ymin, xmin, ymax, xmax]` (Gemini convention); frontend converts using the **image's own** aspect ratio.

**Video (`video`, link audits of TikTok, YouTube Shorts, Instagram, Facebook and X only; absent otherwise).** `status` OK or UNAVAILABLE with `reason` (`unsupported, private, too_long, too_big, timeout, download_failed, no_tools, model_failed`), `platform`, `duration_sec`, `upload_date`, `platform_ai_label`, `frames[]` (≤ 8 JPEG thumbnails, longest side ≤ 240 px), `audio_analyzed`, `summary`, `transcript`, `manipulation` and `context` (`{score 0–100, indicators[]}`), and two fields added on 2026-11-10 (both optional for the client, so older results still parse):
- `creator_disclosure {level: none|weak|strong, phrase}`: what the creator says about AI in the title, description, tags or account name (§6.3).
- `web_evidence[] {title, url (https), publisher, says: AI|REAL|HOAX}`, at most 5: news and fact-check pages judged to be about this video (§6.3).

`pillars.ai.provenance.source` is one of `c2pa | xmp | png_text | exif | watermark | creator | web | ""`. `creator` is the creator's own AI disclosure and `web` is news coverage of the video; both occur for video links only. The platform's AI label outranks `creator`, which outranks `web`.

---

## 3. Scoring specification (`internal/scoring`)

Pure package, no I/O, 100 % table-test coverage.

### 3.1 Inputs
Each pillar → `Signal{Available bool, Score int(0-100), Reliability float64(0-1)}`.

| Pillar | Base weight | Score definition | Reliability |
|---|---|---|---|
| ELA | 0.30 | server ELA variance normalized 0–100 (see §3.4); **one-sided**: counts only from 50 (below that it is reported as checked but carries no weight) | 1.0 JPEG-origin; 0.5 PNG/WebP-lossless or screenshot-like; 0.0 if image < 64 px |
| AI ("Jejak AI") | 0.15 | likelihood 0–100 that the image (the upload, or a link's preview image) was made with AI, computed in Go (§3.6) | from the strongest evidence (§3.6): 1.0 declared in the file, 0.9 visible AI label confirmed by OCR, 0.8 detector, 0.5 model only; unavailable for a link without a preview image |
| Fact | 0.35 | CONTRADICTS=100, UNVERIFIED=40, SUPPORTS=0; **unavailable if no checkable claim**. Since 2026-11-09 (team decision) an `UNVERIFIED` claim, one no fact-checker has ruled on, is shown but enters the score as unavailable (`pipeline.Verdict`): counted as 40 it made all 11 real news videos and 16/20 untouched graphics `SUSPICIOUS`. The vectors below keep testing the formula with 40. | 1.0 |
| Link | 0.20 | `linkcheck` capped sum | 1.0 |

### 3.2 Formula
```
eff_w_i  = base_w_i * reliability_i      (only for Available pillars)
eff_w_ELA = 0 when S_ELA < 50            (one-sided ELA, 2026-11-09; its weight only counts as SAFE coverage)
W        = Σ eff_w_i
if W < 0.30  → status INSUFFICIENT, score = null
raw      = Σ(eff_w_i * score_i) / W
score    = round_half_up(raw)            # Go: int(math.Floor(raw + 0.5))
```

### 3.3 Floors, caps, status
Apply in this order, then clamp 0–100:
1. `FACT_CONTRADICTED`: ≥1 source with `match=CONTRADICTS`, host on allowlist, URL came from search tool → `score = max(score, 85)`.
2. `LINK_MALICIOUS`: Safe Browsing hit or `S_Link ≥ 90` → `score = max(score, 85)`.
3. `ELA_CORROBORATED`: `S_ELA ≥ 70` **and** an LLM text anomaly box overlapping an ELA hotspot (IoU ≥ 0.1 or centre inside) → `score = max(score, 70)`. Since 2026-11-09 a model box whose Timpa Teks evidence (§3.4) is ≥ 70 is itself a hotspot and raises `S_ELA` to its evidence, so the model and the measurement pointing at the same text corroborate each other.
3b. `LINK_STRONG` (added 2026-11-08, team decision "strong link evidence is enough"): if there is still **no** score (W < 0.30, e.g. a bare-URL audit) and the link pillar is available with `S_Link ≥ 45` → `score = min(S_Link, 85)`. 45 is a brand-impersonating host on its own; tuned on a public phishing feed on 2026-11-09 (was 50). The cap keeps a link-only verdict at or below the `LINK_MALICIOUS` floor, so a score never rises as evidence gets thinner. It can never produce `SAFE`. Lead sentence: "Tautan ini punya tanda penipuan. Sebaiknya jangan diklik."
3c. AI floors (added 2026-11-09), **uploaded images and downloaded videos** (a link's preview image may be an honest illustration), applied after `LINK_STRONG` so a risky link in the image keeps its score, at most one of:
   - `AI_PROVENANCE`: the image declares AI generation (provenance `AI_GENERATED`, from metadata or a visible AI label) → `score = max(score, 85)`. Lead sentence: "Ada tanda jelas gambar ini dibuat dengan AI. Jangan anggap sebagai foto asli."
   - `AI_EDITED`: the image declares an AI edit (`AI_EDITED`) → `score = max(score, 70)`. "Ada tanda jelas gambar ini diedit dengan AI. Jangan langsung percaya."
   - `AI_LIKELY`: nothing declared, but `S_AI ≥ 80` (`scoring.AILikelyMin`; the prompt reserves 71–100 for clear visible signs, a detector shares the score) → `score = max(score, 45)`. "Gambar ini tampak buatan AI. Cek dulu sebelum menyebarkan."
   - Since 2026-11-09 the same AI floors apply to a **downloaded social video** (`video` in the Dossier): its AI pillar is the model's judgement of the keyframes, and `AI_GENERATED` comes from the platform's AI label, a strong creator disclosure (`provenance.source = creator`, §6.3), or news coverage saying the video is AI (`web`), in that order of precedence, or a visible AI mark. The sentences say "video" and "rekaman asli" instead of "gambar" and "foto asli". `AI_LIKELY` uses a lower line for video: `scoring.VideoAILikelyMin = 55` (images keep `AILikelyMin = 80`).
3d. Video floors (added 2026-11-09, thresholds recalibrated 2026-11-10), social videos only, from the model's frame-by-frame findings, so they stop at `SUSPICIOUS`: `VIDEO_MANIPULATED` (manipulation signal ≥ `VideoManipulationMin` = 70) → `score = max(score, 45)`, "Video ini tampak disunting untuk menyesatkan. Cek dulu sebelum menyebarkan."; `VIDEO_CONTEXT_MISMATCH` (context-mismatch signal ≥ `VideoContextMin` = 85, was 70) → `score = max(score, 45)`, "Keterangan video ini tidak cocok dengan isi rekamannya. Cek dulu sebelum menyebarkan." Calibration: three runs over 33 labelled clips (11 real news, 10 AI, 12 hoax); at 55/70/85 real news was flagged in 1 of 33 readings (3%) and AI clips in 16 of 30 (53%); at the old 80/70/70, 2 of 33 and 10 of 30. Numbers and caveats: `docs/DECISIONS.md`. A **weak** creator disclosure adds 8 to the AI score, only when the model answered.
4. `ELA_SOLO_CAP`: if **no** other pillar has score ≥ 50 and no floor 1–3c applied → `score = min(score, 60)`.
5. `INJECTION_GUARD`: if LLM output was flagged by the injection detector → `score = max(score, 40)` and `basis` adds `"llm_untrusted"`.

Status: 0–20 `SAFE`, 21–60 `SUSPICIOUS`, 61–100 `TAMPERED`. `SAFE` additionally requires coverage `W + eff_w` of an inconclusive ELA `≥ 0.50` (else downgrade to `INSUFFICIENT`) and an available `Fact` pillar — "no evidence of risk" ≠ "verified safe" (UI text: "Tidak ditemukan tanda bahaya", never "pasti asli"). Until 2026-11-09 `ELA`+`Link` could also reach `SAFE`; on real images the ELA mean was 0 for untouched and 0–14 for edited photos, so a low ELA cannot vouch for an image.

### 3.4 Server-side ELA (`internal/forensics`)
Decode (limit dimensions via `image.DecodeConfig` first; reject > 4096 px side or > 16 MP) → re-encode JPEG q=90 → mean absolute RGB difference → `variance = mean * 3.5` → clamp 0–100. Also compute a coarse 16×16 grid of cell means for hotspot overlap. Mark `reliability` by source decoder format. Add a unit test with a synthetic image (flat gray + pasted noisy patch) asserting score ordering. WebP uploads skip ELA.

**Timpa Teks (`forensics.TextEvidence`, 2026-11-09).** The global mean cannot see a retyped price: it is dominated by the photo. For each text box the model marks (at most 4, 1.5 s after the model answers, on the image the ELA step decoded), compare how the text inside reacts to one more JPEG compression with how the image's **other text** reacts: in a grid-aligned band of rows around the box (or the whole image up to 6 MP when the band holds too little text), re-encode at q75 and q90, take `Σ ELA / Σ |∇luma|` inside the box over the median of the same ratio on 16×16 "text-like" blocks outside it (strong edges, two-tone histogram), keep the larger ratio. Evidence = `(ratio − 1.2) / 0.5 × 100`, clamped; −1 (null) when the box holds no lettering or fewer than 4 reference blocks exist. `S_ELA = max(global, evidence)`; evidence ≥ 70 makes the box a hotspot (`pipeline.TextHotspotMin`). Benchmark (`tools/textbench/gen.py` on 34 real camera photos, two independently generated sets, `TEXTBENCH=… go test ./internal/forensics -run TestTextBenchmark`): no original line reached 50; retyped lines reached 50 in 65 % and 41 % of pairs (71 % and 50 % when not forwarded after the edit, 54 % and 31 % when forwarded). High evidence is a strong sign; low evidence says little.

### 3.5 Test vectors (must pass)
| # | ELA (s,r) | AI (s,r) | Fact | Link | Expected score / status |
|---|---|---|---|---|---|
| 1 | 88, 1.0 | 95, 0.7 | CONTRADICTS (allowlisted) | n/a | raw = (26.4+9.975+35)/0.755 ≈ 94.5 → 95 → TAMPERED |
| 2 | n/a | 20, 0.5 | UNVERIFIED | n/a | (0.075·20 + 0.35·40)/0.425 ≈ 36.5 → 36 → SUSPICIOUS |
| 3 | 80, 0.5 | 10, 0.7 | SUPPORTS | n/a | (12+1.05+0)/0.605 ≈ 21.6 → 22 → SUSPICIOUS |
| 4 | 10, 1.0 | 10, 0.7 | CONTRADICTS (allowlisted) | n/a | ELA 10 carries no weight: W=0.455, raw ≈ 79.2 → 79 → floor 85 → TAMPERED |
| 5 | 95, 1.0 | n/a | n/a | n/a | W=0.30 → raw 95 → ELA_SOLO_CAP → 60 → SUSPICIOUS |
| 6 | n/a | n/a | n/a | n/a | W=0 → INSUFFICIENT |
| 7 | n/a | n/a | SUPPORTS | 5 | (0+1.0)/0.55 ≈ 1.8 → 2 → W=0.55 → SAFE |
| 8 | n/a | n/a | n/a | 95 | S_Link ≥ 90 → floor 85 → TAMPERED (W=0.20 → floor still applies; status TAMPERED) |
| 9 | n/a | n/a | n/a | 85 | W=0.20 → no weighted score; S_Link 85 < 90 → LINK_STRONG → 85 → TAMPERED |
| 10 | n/a | 20, 0.5 | n/a | 55 | W=0.075+0.20=0.275 < 0.30 → LINK_STRONG → 55 → SUSPICIOUS |
| 11 | n/a | n/a | n/a | 40 | S_Link < 45 → no floor → INSUFFICIENT |
| 11b | n/a | n/a | n/a | 45 | LINK_STRONG → 45 → SUSPICIOUS (a brand lookalike alone) |
| 12 | n/a | n/a | n/a | 89 | LINK_STRONG → min(89, 85) = 85 → TAMPERED |
| 15 | 5, 1.0 | 30, 0.7 | n/a | 10 | ELA no weight: W=0.305, raw ≈ 16.9 → 17, but no fact check → INSUFFICIENT (was SAFE) |
| 16 | 5, 1.0 | 80, 0.7 | UNVERIFIED | n/a | W=0.455, raw=(8.4+14)/0.455 ≈ 49.2 → 49 → SUSPICIOUS (the two-sided mean gave 32) |
| 17 | 3, 1.0 | 0, 0.7 | SUPPORTS | n/a | W=0.455, raw 0; coverage 0.755 ≥ 0.50 and fact available → SAFE |
| 18 | 10, 0.7 | 95, 1.0, declared AI_GENERATED, upload | n/a | n/a | W=0.15, no weighted score → AI_PROVENANCE → 85 → TAMPERED |
| 19 | 0, 0.7 | 85, 0.5, upload | n/a | n/a | W=0.075 → AI_LIKELY → 45 → SUSPICIOUS (79 would stay INSUFFICIENT) |
| 20 | n/a | 85, 0.5, upload | n/a | 75 | W=0.275 → LINK_STRONG 75, then AI_LIKELY keeps 75 → TAMPERED |
| 21 | n/a | 95, 1.0, declared, link preview | 40 (UNVERIFIED) | 30 | W=0.70, raw ≈ 48.9 → 49 → SUSPICIOUS, no AI floor for a link |

Row 8 note: floors may rescue an otherwise-`INSUFFICIENT` input to `SUSPICIOUS/TAMPERED` (never to `SAFE`).
**Agent must recompute vectors 1–7 in tests and fix the table here if arithmetic differs.**

### 3.6 Jejak AI: is the image AI-made? (`internal/provenance`, `internal/aidetect`, `pipeline/ai.go`)
Three kinds of evidence, strongest first, for the uploaded image or a link's preview image:
1. **What the file declares** (`provenance.Scan`, metadata blocks only, no decoding, microseconds): Content Credentials (C2PA JUMBF in JPEG APP11, PNG `caBX`, WebP `C2PA`) and IPTC/XMP `DigitalSourceType` (`trainedAlgorithmicMedia` and `algorithmicMedia` → `AI_GENERATED`; `compositeWithTrainedAlgorithmicMedia`, `compositeSynthetic` → `AI_EDITED`; `digitalCapture`, `computationalCapture` → `CAMERA`), a generator as XMP `CreatorTool` or EXIF `Software`, generator settings in PNG text (Stable Diffusion/Fooocus `parameters`, ComfyUI `prompt`/`workflow`, InvokeAI, NovelAI, Midjourney) or in the EXIF user comment, and camera make/model (`CAMERA`, a hint, not proof). A visible AI label ("Imagined with AI", "AI generated", "Dibuat dengan AI", "AI生成") counts as declared (`source: watermark`) only when the model reports it in `ai_watermark` **and** the image's OCR text contains one. Missing metadata proves nothing: WhatsApp and social networks strip it.
2. **A dedicated detector** (optional, `SIGHTENGINE_API_USER`/`_SECRET`, model `genai`, 4 s budget, runs beside the model): `score = round(0.7·detector + 0.3·model)`; a failed call is not evidence and shows no detector line.
3. **The model's visible clues** (`ai_score`, `ai_indicators`, §6).

Score: model alone (reliability 0.5), detector (0.8), then `AI_GENERATED` → `max(score, 95)` (1.0), `AI_EDITED` → `max(score, 80)` (1.0), a confirmed label → `max(score, 90)` (0.9), `CAMERA` without an AI marker → `min(score, 50)`. The pillar is unavailable when nothing could judge (model down, no detector answer, nothing declared). Uploading the bundled demo picture to the live app reaches the demo's verdict from its own bytes (Playwright `ai.spec.ts`).

---

## 4. Pipeline orchestration (`internal/pipeline`)

```
request ctx (`HARD_TIMEOUT_MS`, default 20 s; a video link gets `VIDEO_TIMEOUT_MS`, default 60 s)
 ├─ validate + hash (SHA-256) → cache hit? replay stage/partial events, then verdict (TTL 10 min, max 500 entries, LRU)
 ├─ errgroup (each step has its own budget, bounded by the hard deadline minus a 300 ms reserve; no step fails the group):
 │    ela      : 4 s    (JPEG and PNG uploads; server ELA on up to 16 MP; WebP skips it: re-compressing as JPEG says nothing about a WebP file's history)
 │    link     : 2.5 s  (URL, or the first link in an image: the client's OCR hint, then the model's OCR; also one written without a scheme such as `bit.ly/x` or `bri-verifikasi.my.id`, read as https; an e-mail address is not a link) → unroll ≤ 3 hops → heuristics on first and final host → Safe Browsing on the chain
 │    metadata : 2.5 s  (URL only; oEmbed for YouTube/TikTok/X, else OpenGraph, via the SSRF client) → `source_metadata`
 │    preview  : 1.5 s  (URL only; the og:image/twitter:image/oEmbed thumbnail, ≤ 5 MB, via the SSRF client, validated like an upload) → Jejak AI of the preview image
 │    provenance       (FILE, or the preview image: metadata blocks only, synchronous) → Jejak AI
 │    detector : 4 s    (optional, beside the model, same image) → Jejak AI
 │    llm      : 9 s    (HARD_TIMEOUT_MS - 3 s when that is larger; with LLM_FALLBACK_* the first gateway gets all but the last 9 s, never under half, and a failure hands the call to the fallback and skips the first gateway for 60 s when the failure says the gateway itself is unwell (breaker policy, §6); one structured call: claim, search_query, ai score, anomalies, OCR, tactics)
 │    video    : ≤ VIDEO_TIMEOUT_MS  (social video links, not in mock mode: yt-dlp download (≤ 180 s of footage, ≤ 60 MB), ffmpeg 6 frames at ≤ 640 px and ≤ 60 s of audio, one structured model call over frames, audio when the gateway takes it, title and description; creator disclosure read from metadata, §6.3)
 │    webprov  : 6 s search + 8 s classify (starts as soon as the clip exists, beside the video call; Tavily over the web allowlist, only when TAVILY_API_KEY is set; failure yields no evidence and never marks the audit degraded, §6.3)
 │    text     : 1.5 s  (after the model, on its anomaly boxes: Timpa Teks evidence, §3.4)
 │    search   : 6 s    (Fact Check Tools `claims:search` with the model's `search_query`, or Tavily restricted to the allowlisted hosts when only TAVILY_API_KEY is set; allowlist-filtered)
 │    classify : 10 s   (second structured call: CONTRADICTS/SUPPORTS/UNVERIFIED per search result; 3 s timed out on every gateway we use)
 ├─ every pillar emits exactly one terminal `stage` event; finished pillars → SSE `partial`; `: ping` heartbeat while waiting
 └─ scoring.Compute → sign token → SSE `verdict` → `done`
```
- A pillar error/timeout → `available=false` + entry in `verdict.not_checked`; if any pillar degraded, `degraded=true`.
- Never return cached/demo data for a live request. Cache stores only final Dossier JSON (no raw input).
- Concurrency limiter: max 32 in-flight audits (`MaxInFlight`) → excess gets `RATE_LIMITED` with `Retry-After`.

---

## 5. Link analysis (`internal/linkcheck`)

Deterministic, each rule adds capped points; final `S_Link = min(100, Σ)`.

| Code | Rule | Points |
|---|---|---|
| `IP_HOST` | host is an IP literal | 35 |
| `PUNYCODE` | `xn--` label or mixed-script homoglyphs | 40 |
| `LOOKALIKE_BRAND` | a host token within Levenshtein ≤ 1 of a brand (≤ 2 for names of 9+ letters), a brand plus affix (`bri-verifikasi`), or a leetspeak reading (`faceb00k`, `tiktok` with digits for letters), but not the brand's official registrable domain. Brands include banks, e-wallets, marketplaces, couriers, agencies and, since 2026-11-10, `youtube` and `twitter` with their official domains. The reason is **flagged high severity**; a lookalike is not rejected by the validator (§5.1), it is shown to the user | 45 |
| `SUSPICIOUS_TLD` | `.xyz .top .click .icu .cfd .sbs .shop .live` etc. (config list) | 15 |
| `SCAM_KEYWORDS` | bansos, undian, hadiah, saldo, klaim, gratis, verifikasi-akun in host/path | 20 |
| `SHORTENER_HIDES` | shortener expanded; final host differs & is rated ≥ 30 | +15 |
| `AT_SIGN` / `DEEP_SUBDOMAIN` | `@` in authority; > 4 labels | 20 / 10 |
| `NO_HTTPS` | scheme http | 10 |
| `SAFE_BROWSING` | Google Safe Browsing v4 Lookup hit (optional, env key) | → 100 |
| `PAGE_ASKS_SECRET` | the destination page (not on an official domain) has a field for a password, PIN, OTP, card number or CVV | 20 |
| `PAGE_IMPERSONATION` | the same form on a page whose title or site name names a brand from the list (as a word or inside a product name such as "KlikBCA") | 45 |
| `NEW_DOMAIN` / `YOUNG_DOMAIN` | the youngest registrable domain on the redirect chain was registered ≤ 30 / ≤ 180 days ago (RDAP) | 25 / 10 |

Redirect unrolling: max 3 hops through the SSRF-safe client, `HEAD` then `GET` with `Range: bytes=0-0` fallback, record chain. Official-domain allowlist (e.g. `kompas.com`, `antaranews.com`, `go.id`) reduces `LOOKALIKE_BRAND` false positives. Table tests with ≥ 25 URLs (good and bad).

Implementation notes (2026-11-08; the weights in `internal/linkcheck` and its table tests are the reference):
- `LOOKALIKE_BRAND` matches hostname **tokens** after folding Cyrillic and Greek look-alike letters to Latin (a short reviewed table): an exact brand token, a brand plus affix (`bri-verifikasi`), or Levenshtein ≤ 1 (≤ 2 for brand names of 9+ letters). It never fires on a brand's official registrable domain, nor under suffixes only vetted institutions can register (`.go.id .ac.id .sch.id .mil.id .desa.id`). A folded host never gets the official-domain exemption.
- Brands include the banks, e-wallets, marketplaces, couriers and agencies impersonated in Indonesian phishing (BRI/BRImo, BCA, BNI, BTN, Mandiri, BSI, DANA, OVO, GoPay, Shopee, Tokopedia, Kemensos, Prakerja, PLN, Telkomsel, J&T, JNE and others). Ordinary words that resemble a brand are deliberately absent.
- `.my.id` and `.biz.id` are reported as `SUSPICIOUS_TLD`: in the phishing feed they hosted a third of the Indonesian-themed links and none of the popular legitimate sites.
- `AT_SYMBOL` fires for an `@` outside the authority except a social handle path segment (`/@kompascom`): TikTok, YouTube and Medium links are not tricks; a pre-filled victim email is.
- `UNPARSEABLE` (100): a URL that does not parse is never treated as safe.
- `SHORTENER_HIDES` (+15) is added when the unrolled final host is a different site rated ≥ 30; the score is computed on the final URL.
- A Safe Browsing hit on any URL of the redirect chain sets `S_Link = 100` and the `LINK_MALICIOUS` floor. A failed lookup is not a hit and is not reported as one.
- A URL found in the model's OCR of a screenshot gets the same link pillar as a pasted URL.
- Domain age (2026-11-09, `DOMAIN_AGE_LOOKUP`, default on): the registrable domain of the first and of the final host (not shorteners, not official domains) is looked up over RDAP, using IANA's bootstrap file (`data.iana.org/rdap/dns.json`, fetched once a day) to find the registry, through the SSRF-safe client and beside the redirect check, inside the link budget. Only the registrable domain is sent; answers are not kept. A registry without RDAP, a missing registration event or a timeout adds nothing. A young domain is never enough on its own for a link-only verdict (25 < `LinkStrongMin`).
- Page check (2026-11-09): when the redirect chain ends on a host that is not official, the destination page is read once through the SSRF-safe client (≤ 512 KB, inside the link budget) and only its title, site name and input fields are looked at. It catches phishing on hacked or free-hosting sites, whose address has no tell-tale. "dana", "pajak", "qris" and "permata" are ordinary words in titles and do not count as brands there; "bri" and "ovo" only count at the end of a word. `PAGE_IMPERSONATION` is high severity. Not measured on live phishing pages yet (the development environment cannot reach them).

### 5.1 Link validator (`security.ValidateInputURL`, 2026-11-10)
The one gate for `/audit/url`, run before anything is fetched (the SSRF guard, §7.1, still runs afterwards). It trims the input and returns the canonical link or an error with code `INVALID_URL` and a specific Indonesian message. Rules, in order:
- empty → `EMPTY`; longer than 2048 → `TOO_LONG`;
- any control, space or Unicode format character (zero-width, bidi) → `CONTROL_CHARS`;
- unparsable → `UNPARSABLE`; scheme other than `http` or `https` → `SCHEME`;
- userinfo, an opaque URL, or an `@` anywhere in the authority (even where `url.Parse` forgives it) → `USERINFO`; empty host → `HOST`;
- a port other than none, 80 or 443 → `PORT`;
- `< > " '` or a backtick in the host, `< > " `` ` `` in the path, or the percent-encodings `%3c %3e %22 %60` (any case) → `DANGEROUS_CHARS`. An apostrophe in the path is allowed: news slugs carry it (`it%27s`);
- the host is converted to punycode (IDNA lookup); one that cannot be → `HOST`.
Platform recognition (`video.Platform`) is separate and stricter: exact host, `https` only, no userinfo, no port, no IP literal, and per-platform paths (TikTok any path; YouTube `/shorts/<id>` or `/watch?v=`; `youtu.be/<id>`; Instagram `/reel`, `/reels`, `/p`, `/tv`; X `/<user>/status/<id>`; Facebook `/watch`, `/reel`, `/videos`, `/share/v|r`, `fb.watch`). `tiktok.com.evil.xyz`, `youtu.be.evil.com`, a Cyrillic "і" and userinfo tricks are not recognised as a platform. `yt-dlp` is started with `exec.CommandContext` (no shell), `--ignore-config` and `--` before the URL.
The frontend repeats the cheap checks inline (`lib/link-input.ts`): it extracts the first `http(s)` link from pasted share text, adds `https://` to a bare domain, and refuses `javascript:`, `data:`, `vbscript:`, `file:`, long links and plain words before anything is sent.

---

## 6. LLM integration (`internal/llm`)

- Client speaks the OpenAI-compatible API to `NINEROUTER_BASE_URL` (a historical variable name; any OpenAI-compatible gateway works) using a **separate** `http.Client` (it must reach a LAN or tunnel address, so it is NOT the SSRF client). Model from `NINEROUTER_MODEL`; at startup call `GET /models` and log (without secrets) whether the model exists. Deployments default to OpenRouter's free vision model `nvidia/nemotron-3.5-super-vl:free`; `LLM_EXTRA_BODY` (a JSON object) adds gateway-specific fields to every request, such as OpenRouter's `models` fallback list and `reasoning: {enabled: false}`, and can never replace `model`, `messages`, `response_format` or `temperature`. `NINEROUTER_REASONING_EFFORT` (default `low`) is sent as `reasoning_effort` on the primary client unless `LLM_EXTRA_BODY` already sets `reasoning` or `reasoning_effort`.
- **Breaker policy** (with `LLM_FALLBACK_*` set; otherwise there is nothing to switch to): the first gateway gets all of the model budget except a reserve for the fallback (9 s, or a third of a longer budget such as a video's), never under half. A failed first call opens the breaker for 60 s, and requests go straight to the fallback, only when the failure says the gateway itself is unwell: no connection, HTTP 401, 403, 429 or ≥ 500, or a timeout of a budget ≥ 15 s. A garbled answer, any other 4xx, or a timeout of a shorter budget (a fact-check classification gets about 5 s) concerns that request only and leaves the breaker closed. A success closes it. If the request itself ended, the gateway is not blamed.
- **Analyze** call per audit, `response_format` = strict JSON schema `LLMFindings`:
  `{claim, search_query, ai_score, ai_indicators[], ai_watermark, text_anomalies[{bounding_box, extracted_text, reason}], ocr_text, tactics[{code, phrase}], injection_suspected}`. `ai_score` (since 2026-11-09) rates how strongly the **image** looks made or altered with generative AI, judging the picture, not the topic: 0–15 a camera photo with natural detail, a screenshot or ordinary graphic design; 16–40 one weak sign that a filter could also cause; 41–70 several signs together (waxy skin, melting detail, odd blur, typical AI style); 71–100 clear signs (malformed hands, teeth, eyes or ears; gibberish lettering on signs or clothes; merged or impossible objects; odd repetition; an AI label). An ordinary edit (pasted text, crop, filter) is not AI: it goes to `text_anomalies`. `ai_watermark` copies a visible AI label or watermark ("Imagined with AI", "Meta AI", "Grok", "Gemini" for the star logo) or is "". For a link, the image is its preview image and `ocr_text`/`text_anomalies` stay empty; without an image `ai_score` is 0 and Go ignores it. The UI calls this pillar "Jejak AI" (§3.6). Images are sent as an `image_url` data URI: a small JPEG as is, anything larger or PNG re-encoded as JPEG q85 with the longest side ≤ 1600 px (normalised 0–1000 boxes are unaffected), WebP passed through. The model never returns a fact status or sources.
- Go runs the "tools" itself, before and after the model, instead of model-driven tool rounds: the link's public metadata (oEmbed/OG via the SSRF client, ≤ 2 KB) is fetched first and passed inside `<untrusted_content>`; the model's `search_query` goes to the search provider (Google Fact Check Tools `claims:search`, `languageCode=id`, max 5 allowlisted results); a second **Classify** call labels each returned result `CONTRADICTS | SUPPORTS | UNVERIFIED` against the claim. In mock mode Classify maps the fact-checker's own rating.
- **Prompt-injection containment:** all external text wrapped in `<untrusted_content>…</untrusted_content>`; system prompt states content inside is data; model never emits a verdict/score (those fields do not exist in the schema); on schema violation → treat LLM pillars as failed (`available=false`), not retry loops; `injection_suspected` or regex hits on ("ignore previous", "system prompt", "set verdict") → flag `INJECTION_GUARD`.
- **Source verification (`internal/grounding`):** accept a source only if (a) URL appears in the search provider's own result set (no result set → nothing verifies: fail closed), (b) host (eTLD+1) is in `configs/trusted_sources.yaml`, (c) scheme https. Else downgrade that source to `UNVERIFIED`. Starter allowlist (`grounding.DefaultAllowlist`, mirrored in `configs/trusted_sources.yaml`): turnbackhoax.id, cekfakta.com, komdigi.go.id, kominfo.go.id, antaranews.com, kompas.com, detik.com, tempo.co, cnnindonesia.com, liputan6.com, tirto.id (reviewed by team).
- **`LLM_MOCK=true`:** deterministic fake responses keyed by input hash prefix so frontend/e2e work without a gateway. Required for CI.

### 6.1 Tactics (`internal/tactics`) — informational, never scored
Fixed enum (Indonesian label and one-line "kenapa mencurigakan" live only in static `tactics.json`):
`URGENCY` (mendesak), `REWARD_BAIT` (iming-iming hadiah), `FALSE_AUTHORITY` (mengaku otoritas), `FEAR` (menakut-nakuti), `SECRECY` (minta dirahasiakan), `OUTRAGE_BAIT` (memancing emosi/marah), `IMPERSONATION` (menyamar sebagai orang/lembaga), `FAKE_SOCIAL_PROOF` (klaim "sudah dipercaya banyak orang").
Server verification: normalize (NFKC, lowercase, collapse whitespace) both phrase and source text; keep a tactic only if the phrase is a substring of the input text/caption (`source:"text"`) or of the LLM `ocr_text` for FILE input (`source:"ocr"`). Drop unknown codes, duplicates, phrases > 80 chars; keep max 5. Tactics never enter `scoring.Compute`.

### 6.2 Video analysis (`internal/video`, `internal/llm/video.go`, `pipeline/video.go`)
- Only for a link whose platform is recognised (§5.1), when `yt-dlp` and `ffmpeg` are on the PATH and `LLM_MOCK=false`. The fetcher downloads at most 180 s of footage and 60 MB, takes 6 frames (longest side 640 px) and up to 60 s of audio, and reads title, description, tags, uploader and the platform's AI label from the metadata.
- One structured call returns `summary`, `transcript`, `claim`, `search_query`, `ai_score`, `ai_indicators`, `ai_watermark`, `manipulation_score`, `context_mismatch_score` (with indicators), `ocr_text`, `tactics`, `injection_suspected`. The prompt names 8 cues for AI video (physics and object permanence, identity drift, garbled text, hands and teeth, light and reflection, over-smooth camera, generator logos, silent 5–10 s clips). Audio is sent only when `LLM_AUDIO_INPUT` (or `LLM_FALLBACK_AUDIO_INPUT` for the fallback) is true; otherwise `audio_analyzed` is false.
- Scoring: `AI_PROVENANCE` and the other AI floors, `VIDEO_MANIPULATED` and `VIDEO_CONTEXT_MISMATCH` (§3.3). None of them reaches `TAMPERED` except an AI declaration (platform label, strong creator disclosure, or web coverage).

### 6.3 Creator disclosure and web provenance (2026-11-10)
- **Creator disclosure** (`video/disclosure.go`, deterministic, no model). Reads title, description, tags and uploader. `strong`: AI hashtags (`#aigenerated`, `#aivideo`, `#aiart`), generator hashtags (`#sora`, `#veo3`, `#klingai`), phrases ("ai generated", "dibuat dengan/pakai ai", "buatan ai", "hasil ai"), a generator name next to a making word, an ambiguous name (Sora, Veo, Kling) only with a making verb, "AI" or a version, or an uploader whose name holds a generator brand. `weak`: `#ai`, `#artificialintelligence`, `#genai`. `none`: a news outlet as uploader (kompas, detik, bbc and similar), a loose phrase ("video AI") when the title is about AI itself (waspada, aturan, hoaks, how to spot), and words that are also names (Wan, Luma, Pika, Runway) without AI or a version. Strong → provenance source `creator` → `AI_PROVENANCE` floor (85, `TAMPERED`). Weak → +8 to the AI score when the model answered. On the 33 labelled clips (metadata fetched on 2026-11-10, not unit tests) strong fired on 9 of 10 AI clips and on none of the 23 real or hoax clips; the AI class is official generator channels, so this is an upper bound.
- **Web provenance** (`pipeline/webprov.go`; needs `TAVILY_API_KEY`). The query is the core phrase of the title (or, without a usable title, the first description sentence; links, hashtags, mentions and emoji removed; 3 to 10 words) plus " video AI hoaks", sent to Tavily with no "cek fakta" prefix over `grounding.WebAllowlist` (26 hosts: Indonesian and international newsrooms and fact-checkers). Filter chain: https host on the allowlist → `SameTopic(phrase, title + snippet)` → `llm.ClassifyWeb` (AI, REAL, HOAX or UNRELATED); a HOAX hit also needs a fact-checker host (`IsFactCheck`). At most 5 are kept. AI evidence becomes provenance (`AI_GENERATED`, source `web`) unless the platform label or a strong creator disclosure already set it; HOAX evidence enters the fact pillar as `CONTRADICTS` (a fact pillar is created with the phrase as the claim if the model found none). Budgets: search 6 s, classify 8 s, both capped by the request deadline. The claim fact check keeps the narrower `DefaultAllowlist`.
- **Indonesian topic matching** (`grounding.SameTopic`, used by both). A light stemmer strips the suffixes `nya kan lah kah an i` then the prefixes `meny peny meng peng mem pem men pen ber ter me pe di ke se`, leaving at least 4 letters (5 for `-an`/`-i`), and a synonym table maps phrases ("wakil presiden" = "wapres", "mengundurkan diri" = "pengunduran" = "mundur", "meninggal" = "wafat" = "tewas", "hoax" = "hoaks"). A match is 2 shared stems (all of them when the claim has fewer than 2), or 1 shared proper noun plus 1 other shared stem.

---

## 7. Security specification

### 7.1 SSRF-safe client (`security/ssrf.go`) — replaces v9 code
```go
var blockedCIDRs = mustCIDRs(
  "0.0.0.0/8","10.0.0.0/8","100.64.0.0/10","127.0.0.0/8","169.254.0.0/16",
  "172.16.0.0/12","192.0.0.0/24","192.0.2.0/24","192.168.0.0/16","198.18.0.0/15",
  "198.51.100.0/24","203.0.113.0/24","224.0.0.0/4","240.0.0.0/4",
  "::/128","::1/128","64:ff9b::/96","100::/64","2001:db8::/32","fc00::/7","fe80::/10","fec0::/10","ff00::/8",
)

func IsBlocked(ip net.IP) bool {
  if ip == nil { return true }
  if v4 := ip.To4(); v4 != nil { ip = v4 }            // handles ::ffff:a.b.c.d
  for _, n := range blockedCIDRs { if n.Contains(ip) { return true } }
  return false
}

func NewSafeClient(timeout time.Duration, maxHops int) *http.Client {
  d := &net.Dialer{
    Timeout: timeout,
    // Control runs on the *actual* address being connected, after DNS -> defeats rebinding & multi-A tricks
    Control: func(network, address string, _ syscall.RawConn) error {
      host, port, err := net.SplitHostPort(address)
      if err != nil { return err }
      if port != "80" && port != "443" { return errors.New("port not allowed") }
      if IsBlocked(net.ParseIP(host)) { return errors.New("target not allowed") }
      return nil
    },
  }
  return &http.Client{
    Timeout: timeout,
    Transport: &http.Transport{
      Proxy: nil,                                   // never honour HTTP(S)_PROXY env
      DialContext: d.DialContext,
      ResponseHeaderTimeout: timeout,
      MaxResponseHeaderBytes: 64 << 10,
      DisableKeepAlives: true,
    },
    CheckRedirect: func(r *http.Request, via []*http.Request) error {
      if len(via) >= maxHops { return errors.New("too many redirects") }
      if r.URL.Scheme != "http" && r.URL.Scheme != "https" { return errors.New("scheme not allowed") }
      return nil
    },
  }
}
// Every body read MUST use io.LimitReader(resp.Body, 2<<20) and check Content-Type.
```
Tests (table, `httptest` + custom resolver): 127.0.0.1, `localhost`, 10.x, 172.16.x, 192.168.x, 169.254.169.254, `[::1]`, `::ffff:127.0.0.1`, `2130706433` (decimal IP), `0x7f.1`, redirect-to-private, DNS name resolving to private, port 20128/22/6379, `file://`, `gopher://`, 3+ redirects. ≥ 15 cases.

### 7.2 Upload validation
Read at most 10 MB + 1; verify magic bytes (JPEG `FFD8FF`, PNG `89504E47`, WebP `RIFF….WEBP`), ignore client MIME; `image.DecodeConfig` for dimensions before full decode (WebP: the VP8/VP8L/VP8X header, since the standard library has no WebP decoder; AMBA never decodes WebP, it passes it to the model and the detector); strip nothing server-side (never persisted); reject animated/oversized. Never write to disk (use `bytes.Reader`). A link's preview image goes through the same validation.

### 7.3 Rate limiting & proxy trust
`golang.org/x/time/rate`, 30 req/min/client, burst 10 (judges in one room often share a campus NAT), plus a global cap of 32 audits in flight. Client key = forwarded-client header **only if** the immediate peer is in `TRUSTED_PROXY_CIDRS` (empty by default; `*` on Fly.io/Railway, whose machines are only reachable through their proxy), else `RemoteAddr`. The header is `CLIENT_IP_HEADER` when set (`Fly-Client-IP` on Fly.io, `X-Real-IP` on Railway: both overwritten by the edge), otherwise the **rightmost** `X-Forwarded-For` entry (the one the trusted proxy appended). `/healthz` is exempt. Hash key with a salt rotated at boot; limiter map swept every 5 min (idle > 10 min evicted). Never log the key.

### 7.4 Headers, CSP, CORS
- Go API (JSON only): `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Cache-Control: no-store`, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`.
- **Frontend, nginx** (`frontend/nginx.conf`, the production edge): `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Permissions-Policy: camera=(), microphone=(), geolocation=()`, `X-Frame-Options: DENY`, `Content-Security-Policy: frame-ancestors 'none'` and `Strict-Transport-Security: max-age=31536000`. nginx drops every server-level `add_header` in a `location` that sets its own, so the `location /_app/` block (hashed assets, `Cache-Control: public, immutable`) repeats all six; `frontend/src/lib/api/nginx.test.ts` guards this. `/api/` is proxied with `proxy_buffering off` and long read timeouts for SSE.
- **Frontend, CSP and other hosts** (authored once in `frontend/security-headers.json`; applied by a Vite dev/preview plugin and mirrored in `frontend/vercel.json` and `netlify.toml`, CI-checked by `npm run headers`; the full CSP is emitted by SvelteKit with script hashes, and `connect-src` is generated from `VITE_API_BASE_URL` at build time): `default-src 'self'; script-src 'self' <hash>; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self' <API_ORIGIN>; worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'; object-src 'none'` + the same `Permissions-Policy`. (No `https:` wildcards; external images are never hotlinked.)
- CORS: allow only `PUBLIC_APP_URL` and the exact origins in `CORS_EXTRA_ORIGINS` (+ `http://localhost:5173` in dev); methods GET/POST; no credentials.

### 7.5 Signed tokens (`internal/token`)
`v1.` + base64url(JSON payload) + `.` + base64url(HMAC-SHA256(key=`REPORT_SIGNING_KEY`, msg=`v1.`+payload)). Payload: `{id, ts, status, score, title(≤80), claim?(≤120), srcs:[{pub,url,m}] (≤3)}` ≤ 2 KB. Constant-time compare (`hmac.Equal`). Key ≥ 32 bytes, required in production (server refuses to start otherwise).
The share-report UI was removed on 2026-11-10; `share_token` is still issued and `/verify` still answers. Mini-token for QR is no longer used. The old design put the token in the **URL fragment** (`/hasil#t=…`) so servers/CDNs never see it; it POSTs to `/verify` in the body.

### 7.6 Logging / metrics
Only the anonymous line format from v9 §14.2 plus counters; no URLs, titles, IPs. Metrics on a separate internal listener (`METRICS_ADDR`, default off; Prometheus text at `/metrics`, labels are fixed enums): audits by input type and status, degraded audits, durations, rate limiting, replies, verifications, Telegram updates, and per-rule counts by code (`amba_floors_total`, `amba_link_reasons_total`). A failed upstream call (LLM gateway, Fact Check Tools, Safe Browsing) logs only the service, a reason (`timeout | transport | http_status | unreadable_response`) and the HTTP status: never the request URL, which carries keys and user queries.

---

## 8. Frontend specification

- **Routing:** SvelteKit file-based routes (`ssr = false`, `adapter-static` with SPA fallback; code-split per route automatically): `/`, `/deteksi`, `/hasil`, `/cek-cepat`, `/edukasi`, `/panduan-keluarga`, `/tentang`. `/hasil` reads the in-memory `AuditSession` (the live result). Nothing is persisted, so a page reload of a live result shows "Hasil tidak tersimpan — periksa lagi" with a one-tap re-check. There is one primary action above the fold on `/` and `/deteksi`: a link field, a paste button and "Periksa"; home hands the cleaned link to `/deteksi?q=`.
- **API client:** typed with Zod schemas generated/copied from `api/openapi.yaml`; unknown `schema_version` → friendly "Perbarui aplikasi" error. `AuditSession` (`$lib/audit.svelte.ts`, Svelte 5 runes) handles SSE, aborts on component destroy (`$effect` cleanup) and retries once on network error; stage changes are announced through a polite `aria-live` region. Frontend tests use Vitest + `@testing-library/svelte`.
- **ELA worker (`ela.worker.ts`):** receives `ImageBitmap`, uses `OffscreenCanvas`; processes in row bands (yield every ~50 ms); returns heatmap `ImageBitmap`/Blob URL. Max processed size 4096 px longest side (above → downscale with notice). **Revoke all object URLs** on unmount; terminate worker on route leave.
- **Split slider fixes:**
  - Render bounding boxes in a layer **outside** the clip-path container so they remain visible on both sides.
  - Container uses `aspect-ratio` from decoded image; boxes positioned by percentage against that same box (no letterbox mismatch).
  - Use a real `<input type="range" aria-label="Geser untuk membandingkan gambar asli dan hasil ELA" aria-valuetext="{n}% gambar asli">` visually hidden but focusable, with a visible focus ring on the handle; arrow keys step 2%, PageUp/Down 10%; pointer events via `touch-action: pan-y`.
  - Provide non-slider alternative tabs (Asli / ELA) and list anomalies as text under the viewer.
- **Honest-limits panel:** under verdict show "Berdasarkan: …" from `verdict.basis` and "Tidak dapat diperiksa: …" from `not_checked`, plus ELA reliability note when < 1.
- **Read-aloud:** `speechSynthesis.speak` with `lang="id-ID"`; feature-detect, hide button if unsupported; cancel on route change.
- **Perf:** code-split per route; no icon library barrel imports (import individual `lucide-svelte` icons); fonts self-hosted, `font-display: swap`; measure with `vite build --report`/`rollup-plugin-visualizer` in CI (fail > 250 KB gz initial).
- **Visual system:** see §8.0 and `DESIGN.md` (authoritative). Status colours are tokens, always paired with an icon and a text label.
- **Error/empty states** for: offline, 429 (show countdown), 504/degraded, unsupported file, no checkable claim, platform-blocked link.

### 8.0 Visual system (authoritative: `DESIGN.md`)
- **Direction:** dark, atmospheric, technical (fine borders, subtle grid, small status indicators), restrained warm primary accent, sparing cool secondary accent, optional pixel/character details that stay secondary. No glassmorphism, heavy glow, blur or gradients.
- **Tokens** live in `frontend/src/lib/styles/tokens.css` as CSS custom properties; the Tailwind theme maps to them (no raw hex in components). Groups: colour roles, type scale/roles, spacing, border widths/colours, radii, elevation, motion durations/easing, focus ring, interactive states (hover/focus/active/disabled/selected).
- **Provisional palette** (must be validated against the approved visual reference before it is frozen, per `DESIGN.md` §3): `--bg #0b0f14`, `--surface #131a22`, `--border #2a3542`, `--text #e6edf3`, `--text-2 #9aa7b4`, `--accent #ffb454` (warm), `--accent-2 #5cc8ff` (cool, sparing), `--ok #3fb950`, `--warn #d29922`, `--err #f85149`. Never convey meaning by colour alone.
- **Contrast gate:** `tools/check-contrast.mjs` reads the tokens and fails CI if any text/background pair is < 4.5:1 (< 3:1 for large text, UI borders and the focus ring).
- **Type:** a highly readable sans for body and key information; monospace only for labels, metadata and IDs; a pixel/display face only for short headings; no essential information in tiny uppercase. Self-host OFL-licensed fonts, `font-display: swap`.
- **Shapes and icons:** one radius scale, fine borders over heavy containers, not everything is a card or pill; one icon set and stroke weight (`lucide-svelte`); action icons have accessible names; decorative graphics are `aria-hidden` and never resemble controls.
- **Motion:** `--dur-*` tokens; `html[data-motion="calm"]` (forced when `prefers-reduced-motion: reduce`) sets non-essential durations to ~0, removes looping/ambient effects and swaps `ScanningRadar` for a static labelled progress list. No flashing, no autoplay audio.

### 8.1 PWA share target (F11)
- `static/manifest.webmanifest`: `display: standalone`, icons, and
  `share_target: {action:"/share-target", method:"POST", enctype:"multipart/form-data", params:{title:"title", text:"text", url:"url", files:[{name:"media", accept:["image/jpeg","image/png","image/webp"]}]}}`.
- `src/service-worker.ts` (auto-registered by SvelteKit) intercepts `POST /share-target`: read `formData`; reject files > 10 MB or non-image types; if `url` is empty, extract the first `http(s)` URL from `text` (Android often puts links there); store `{url?, text?, file?}` in Cache Storage (`amba-share-v1`, key `/__share/<random id>`); respond `303` to `/deteksi?share=<id>`.
- Client reads the entry, **deletes it immediately**, auto-starts the audit with a visible "Batalkan" button. On every app boot sweep entries older than 5 min. This is the **only** client-side persistence of user input and is transient by design.
- Fallback for unsupported browsers (notably iOS Safari): "Tempel dari clipboard" button using `navigator.clipboard.readText()` on user gesture; detect URL vs text automatically.
- Limitations to state in README/About: works only for the installed PWA on supporting browsers (Android Chrome); installability needs HTTPS when hosted (`localhost` OK in dev).
- Tests: unit-test the SW handler with a fake `FormData`/Cache; Playwright cannot exercise the OS share sheet, so add a manual Android checklist to `docs/PROGRESS.md`.

### 8.2 Plain-language sentence and read-aloud
- Removed on 2026-11-10: Mode Jelajah and Mode Esensial, Ukuran teks (A / A+ / A++), and their `data-info-key` parity contract. There is one presentation, a flat version of the dark and light token system.
- `explanation_simple` is produced **in Go by deterministic templates** (no LLM): `FACT_CONTRADICTED` → "Kabar ini dibantah sumber terpercaya. Jangan disebarkan."; `LINK_MALICIOUS` → "Tautan ini mirip penipuan. Jangan diklik."; `LINK_STRONG` → "Tautan ini punya tanda penipuan. Sebaiknya jangan diklik."; `ELA_CORROBORATED` → "Gambar ini terindikasi diedit. Jangan langsung percaya."; other `SUSPICIOUS` → "Ada tanda yang perlu diwaspadai. Cek dulu sebelum menyebarkan."; `INSUFFICIENT` → "Informasinya belum cukup untuk dipastikan."; `SAFE` → "Tidak ditemukan tanda bahaya, tapi tetap bijak sebelum menyebarkan." Unit-tested. It is the lead sentence of every result.
- Read-aloud (`speechSynthesis`, `lang="id-ID"`) speaks `explanation_simple` first, then the explanation.
- `/hasil` order: verdict hero (status, one sentence, risk meter, read-aloud), "Kenapa?" (2 to 4 reasons built only from dossier fields, `lib/reasons.ts`), the video card or image viewer (for video: frames, creator disclosure, web evidence), details (link preview, tactics, the four pillars), limits, "Periksa lagi".

### 8.3 Tactic highlighter (F13)
`HighlightedText` renders the input/caption (or OCR text) with `<mark>` around each verified phrase; each mark has an icon + label from `tactics.json` (not colour only) and a focusable "Kenapa ini mencurigakan?" popover. Render via Svelte text interpolation (never `{@html}` on user- or LLM-derived content).

---

## 9. Demo data rules
- `backend/internal/demo/demo-cases.json` is canonical; CI copies/diffs to `frontend/src/lib/data/demo-cases.json`.
- Images: generated by `backend/cmd/gendemo` (`go run ./cmd/gendemo`), which draws the synthetic screenshot, runs the real forensics and scorer on it, and writes both demo-cases.json copies, `frontend/static/demo/berita-ig.jpg` and the Edukasi quiz images with their **real** ELA heat maps (`frontend/static/edukasi/`). Static files instead of data URIs keep the JSON small; CI regenerates them and fails on any diff.
- Case IDs follow PRD F8 (`demo-tiktok`, `demo-berita-ig`, `demo-resmi`) plus `demo-tautan` for the link flow and `demo-gambar-ai` for Jejak AI; the earlier `contoh-*` IDs remain as aliases. `frontend/static/demo/gambar-ai.jpg` carries an unsigned, simulated C2PA segment (`claim_generator AMBA-contoh-simulasi`, `trainedAlgorithmicMedia`), so uploading it gives the demo's verdict; a Go test scans the bundled file.
- Every demo Dossier: `is_demo: true`, visible watermark "CONTOH / SIMULASI", no real person/brand logos, no invented URLs for real publishers — use `https://example.org/contoh-cek-fakta` style placeholders with `publisher: "Sumber Contoh"`.
- Validate demo JSON against the Dossier JSON Schema in both Go and Vitest tests.
- Demo Dossiers include `tactics` (phrases must appear in the synthetic caption/OCR text) and `explanation_simple`.

---

## 10. Config (`.env.example`)
```bash
SERVER_HOST=0.0.0.0
SERVER_PORT=8080                   # falls back to a PaaS-injected PORT, then 8080
ENVIRONMENT=development            # production requires REPORT_SIGNING_KEY, an https PUBLIC_APP_URL and LLM_MOCK=false
PUBLIC_APP_URL=http://localhost:5173
CORS_EXTRA_ORIGINS=                # optional exact origins, comma-separated
TRUSTED_PROXY_CIDRS=               # "*" on Fly.io/Railway
CLIENT_IP_HEADER=                  # Fly-Client-IP (Fly.io), X-Real-IP (Railway); empty = rightmost X-Forwarded-For
NINEROUTER_BASE_URL=https://openrouter.ai/api/v1   # any OpenAI-compatible gateway (historical variable name)
NINEROUTER_API_KEY=                # secret: host secret or .env, never in git
NINEROUTER_MODEL=nvidia/nemotron-3.5-super-vl:free  # verify against /models at boot
LLM_EXTRA_BODY=                    # JSON object merged into model requests, e.g. OpenRouter {"models":[...],"reasoning":{"enabled":false}}
LLM_MOCK=true                      # refused in production: the mock invents findings
NINEROUTER_REASONING_EFFORT=low    # minimal|low|medium|high, sent as reasoning_effort
LLM_FALLBACK_BASE_URL=            # second gateway; set base URL, key and model together
LLM_FALLBACK_API_KEY=
LLM_FALLBACK_MODEL=
LLM_FALLBACK_EXTRA_BODY=
LLM_AUDIO_INPUT=false             # the gateway takes audio parts (video soundtrack); LLM_FALLBACK_AUDIO_INPUT likewise
VIDEO_TIMEOUT_MS=60000            # budget of a social video link audit
TAVILY_API_KEY=                   # claim fact check without a Google key, and the video web provenance search
FACT_RETRIEVAL=true               # keyless news search, used only when neither GOOGLE_API_KEY nor TAVILY_API_KEY is set
FACT_SEARCH_URL=https://www.bing.com/news/search
REPORT_SIGNING_KEY=                # >=32 random bytes, base64
GOOGLE_API_KEY=                    # Fact Check Tools + Safe Browsing
GOOGLE_FACTCHECK_KEY=              # optional override
GOOGLE_SAFE_BROWSING_KEY=          # optional override
DOMAIN_AGE_LOOKUP=true             # RDAP domain age for links (NEW_DOMAIN / YOUNG_DOMAIN); false to switch off
SIGHTENGINE_API_USER=              # optional AI-image detector (Jejak AI); set both or neither
SIGHTENGINE_API_SECRET=            # the image is sent to Sightengine: say so in the privacy notice
TELEGRAM_BOT_TOKEN=                # optional (PRD P2); empty = disabled
TELEGRAM_MODE=webhook              # webhook | polling | off
TELEGRAM_WEBHOOK_SECRET=           # webhook: >=16 chars of A-Z a-z 0-9 _ -
TELEGRAM_WEBHOOK_URL=              # webhook: https://<api>/telegram/webhook, registered at boot
MAX_UPLOAD_SIZE_MB=10
HARD_TIMEOUT_MS=20000
CACHE_TTL_SECONDS=600
METRICS_ADDR=                      # empty = disabled
```
Frontend (build time): `VITE_API_BASE_URL`, `VITE_API_MODE=http|mock`.

## 11. Test matrix
| Layer | Tool | Must cover |
|---|---|---|
| Scoring | `go test` table tests | vectors §3.5, floors/caps, rounding, W thresholds |
| SSRF | `go test` | ≥ 15 cases §7.1 |
| Link heuristics | `go test` | ≥ 25 URLs |
| ELA | `go test` | synthetic patch ordering |
| Token | `go test` | tamper, truncation, key rotation, size |
| Handlers | `httptest` | each route, error envelope, SSE order, 429 |
| Race | `go test -race ./...` | whole backend |
| FE unit | Vitest + @testing-library/svelte | Zod parsing, token parse, bbox math, a11y helpers, `link-input.ts` (incl. attack strings), quick-check, reasons |
| FE e2e | Playwright | demo (offline), link flow w/ `LLM_MOCK`, upload flow, rate limit, language switch, axe |
| A11y | `@axe-core/playwright` | 0 serious/critical on all routes |
| Perf | `tools/lighthouse.mjs` (CI job) | ≥ 90 performance/best-practices/SEO, ≥ 95 accessibility, CLS ≤ 0.1, median of 5 mobile runs per route; bundle ≤ 250 KB gz |
| Design tokens | `tools/check-contrast.mjs` | all text/background token pairs ≥ 4.5:1 (UI/large ≥ 3:1) |
| Security | headers test | CSP/CORS values present on SPA + API |
| Tactics | `go test` | phrase-substring verification, unknown codes dropped |
| Explanation templates | `go test` | each status/floor → exact simple sentence |
| Share target | Vitest + manual | SW handler unit test (URL extraction, size/type rejection, delete-on-read, 5-min sweep); manual Android Chrome checklist |
| Link validator | `go test` (`security/urlcheck_test.go`, `video/video_test.go`) | every rule of §5.1 with attack strings; platform lookalikes (`tlktok`, `tiktok.com.evil.xyz`, Cyrillic "і", userinfo, `youtu.be.evil.com`) |
| Creator disclosure | `go test` (`video/disclosure_test.go`) | positive and negative cases plus official-channel cases |
| Web provenance, SameTopic | `go test` (`pipeline/webprov_test.go`, `grounding/match_test.go`) | filter chain, budgets, failure never degrades; Indonesian morphology, synonyms, negatives |
| Headers | `frontend/src/lib/api/nginx.test.ts` | the six headers at server level and in `/_app/` |
