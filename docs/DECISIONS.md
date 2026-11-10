# Decisions

Record of choices made where the spec left room, plus any conflict found between docs.
Format: date · decision · why · what it would take to revisit.

## 2026-10-31 · Workspace scoped to /home/riko/Projects/AMBA
Project instructions now restrict all file access to this directory. Global
configuration is out of bounds unless the user explicitly approves a one-off.
Revisit: only if the user widens scope.

## 2026-10-31 · TypeScript 6.0.3 (not 5.x)
`@sveltejs/kit@3.1.1` declares a peer requirement of `typescript@^6.0.0`, so 5.9
fails installation. Revisit: when Kit relaxes the peer range.

## 2026-10-31 · Tailwind 3.4 (not 4.x)
Tailwind 4 moves configuration into CSS (`@theme`), which conflicts with the
token contract in TECH_SPEC §8.0 that expects `tailwind.config.ts` mapping to
CSS custom properties. Tailwind 3 keeps that mapping explicit and testable.
Revisit: if the team wants Tailwind 4, the token mapping must be rewritten and
`tools/check-contrast.mjs` re-pointed at the `@theme` block.

## 2026-10-31 · `@lucide/svelte` instead of `lucide-svelte`
`lucide-svelte@1.0.1` is deprecated upstream; `@lucide/svelte` is the maintained
package with the same icon set and stroke control. TECH_SPEC §8 only requires
one icon set with a consistent stroke weight, which this satisfies.

## 2026-10-31 · Fonts self-hosted via @fontsource-variable
TECH_SPEC §8 requires self-hosted, OFL-licensed fonts with `font-display: swap`.
Geist and Geist Mono (OFL) are installed as packages rather than fetched at
runtime, so no network call leaves the browser for fonts.

## 2026-10-31 · SPA mode via `ssr = false` + adapter-static fallback
Per TECH_SPEC §8 routing. `prerender = false` because every route reads
in-memory session state or a URL fragment; nothing is statically generated.

## 2026-10-31 · SvelteKit 3 removed `$lib` — use `#lib` (or a relative path)
`@sveltejs/kit@3` throws `module_removed_lib` on any `$lib/...` import. The
supported alias is `#lib`. Phase 0 uses a relative import for the one CSS
import; new modules should use `#lib/...`. Revisit: if the team prefers to keep
`$lib` for familiarity, add `alias: { '$lib': 'src/lib' }` to the `sveltekit()`
plugin config.

## 2026-10-31 · CSP is emitted by SvelteKit, not hardcoded in security-headers.json
SvelteKit injects an inline bootstrap script. A static `script-src 'self'`
blocked it and the SPA rendered a blank page with no console error beyond the
CSP violation. The policy now lives in the `sveltekit({ csp: { mode: 'hash' } })`
config so each build's inline script hash is included automatically.
`security-headers.json` keeps only the directives that must arrive as real HTTP
headers: `frame-ancestors 'none'` is ignored inside `<meta>` and therefore
cannot be part of the SvelteKit meta policy. Two policies on one response
intersect, so the header policy carries `frame-ancestors` alone — adding
`script-src` there would re-block the hashed inline script.

## 2026-10-31 · `tokens.css` must be imported before any consumer
Design tokens are CSS custom properties; a rule using `var(--x)` before the
definitions load silently falls back to browser defaults (serif text, 16 px
headings). The layout imports `tokens.css` first, then `app.css`.

## 2026-10-31 · Go module pinned to 1.24, not the toolchain's 1.26
Adding `golang.org/x/time` ran `go mod tidy` with a local Go 1.26 toolchain,
which rewrote the directive to `go 1.26.0` — newer than CI's pinned 1.24, so CI
would have failed on a clean runner. Pinned back to `go 1.24`; the module
builds and tests clean at that version. Revisit: when CI moves to 1.26.

## 2026-10-31 · TAMPERED needs a fallback explanation_simple
TECH_SPEC §8.2 defines `explanation_simple` only for the three floors,
SUSPICIOUS, INSUFFICIENT and SAFE. A weighted score can reach TAMPERED without
any floor firing (for example ELA 69 + AI 100 + UNVERIFIED + link 89), so
`scoring.ExplanationSimple` falls back to "Beberapa tanda bahaya ditemukan. Cek
dulu sebelum menyebarkan." — calm, non-certain, and consistent with the
vocabulary contract. Revisit: if the team prefers to define this case in the
spec, update §8.2 and the test table together.

## 2026-10-31 · ELA_SOLO_CAP only applies when ELA is available
TECH_SPEC §3.3 floor 4 reads literally as "if no other pillar scored >= 50 and
no floor 1-3 applied, cap at 60" — with no mention of ELA availability. Read
literally, an input where ELA could not run at all (AI 100 + link 49) would be
capped to 60 and downgraded to SUSPICIOUS, suppressing two real signals because
a third was missing.

The implementation guards the cap with `ELA.Available`, matching the floor's
name and its intent (stop ELA acting alone from reaching TAMPERED). Covered by
the test "no cap when ela is unavailable". Revisit: if the team wants the
literal reading, remove the guard and update §3.3 wording and that test.

## 2026-11-01 · ELA is blind to images already saved at the analysis quality
ELA compares the submitted image against a q=90 re-encode of itself. If the
input is already a q=90 JPEG the re-encode is effectively a no-op, every
difference rounds to zero, and the pillar reports a score of 0 even for a
visibly composited image. Real screenshots and re-saved photos carry a
different compression history, so this mostly affects synthetic test fixtures
— but it is a real ceiling on the technique.

Consequences: (a) unit tests must encode fixtures at a quality other than 90,
(b) the UI must never present a 0 ELA score as "clean" — reliability 0.0 and
the honest-limits panel exist for this, (c) the demo assets must be generated
with a realistic compression history or their heatmaps will be empty.

## 2026-11-01 · Fact status is derived from verified sources, never claimed by the model
The model reports a `fact_status`, but the pipeline only keeps it when at least
one source survived grounding with the matching verdict. A `CONTRADICTS` claim
backed solely by a non-allowlisted or invented URL is downgraded to
`UNVERIFIED`, which caps the fact pillar at 40 and prevents the
FACT_CONTRADICTED floor from firing.

Found by a live smoke test: a text audit scored 97 / TAMPERED while its only
"source" was `https://example.org/contoh-cek-fakta` marked UNVERIFIED. The
score now lands at 47 / SUSPICIOUS, which is what the evidence supports.

Consequence for mock mode: the mock cites `example.org`, so it can never reach
TAMPERED through the fact pillar. That is intentional — inventing a real
publisher URL to make a test look better would break CLAUDE.md rule 5. TAMPERED
coverage in e2e comes from a synthetic lookalike domain in a URL audit
(`https://bca-login.xyz/...`), where the LINK_MALICIOUS floor fires on real
heuristics rather than a fabricated citation.

Demo data still shows CONTRADICTS with `example.org` sources because TECH_SPEC
§9 requires demo sources to be placeholders labelled "Sumber Contoh"; demo
dossiers are pre-baked and never pass through grounding.

## 2026-11-01 · RESOLVED 2026-11-08 — link-only audits always return INSUFFICIENT

Resolved by the `LINK_STRONG` floor; see the 2026-11-08 entry below.

Verified behaviour: a URL audit whose only available pillar is Link returns
`INSUFFICIENT` with `score: null`, even when the link score is 85 and the
reasons include a lookalike bank brand, a suspicious TLD and a verification
path. Cause: Link carries base weight 0.20, and W = 0.20 < 0.30, so the
formula yields INSUFFICIENT before any floor can apply. Only a link score
>= 90 reaches the LINK_MALICIOUS floor (spec §3.3 floor 2).

This is literally what §3.2/§3.3 specify, so it is NOT a bug in the code. But
two product questions are unresolved and must be decided before Phase 6:

1. Should a URL audit with a link score in the 85-89 band produce a verdict?
   Options: lower the floor threshold to 85, add a weight floor for Link, or
   accept INSUFFICIENT. Any change must update TECH_SPEC §3.3 and the test
   vectors together.
2. Trust UX: "we checked the link and found nothing wrong" currently renders
   as "Belum cukup data", which reads like a failure rather than a completed
   check. `amba-trust-ux` should get an explicit INSUFFICIENT-because-single-
   pillar case before the copy pass.

## 2026-11-01 · Mock mode cannot reach TAMPERED via the fact pillar
Follows from the verified-source rule above. Mock cites example.org, grounding
rejects it, fact stays UNVERIFIED. TAMPERED coverage comes from a URL audit
against a synthetic lookalike domain. Do not "fix" this by allowlisting
example.org or inventing a real publisher.

## 2026-11-01 · Session paused at Phase 2 complete, Phase 4 not started
Backend through the pipeline and all audit/verify/reply routes is implemented
and green. Frontend is at Phase 3. Next: Phase 4 (results experience), then
the open questions above.

## 2026-11-01 · OPEN · Server serialises empty slices as null, breaking the contract

Found while verifying the live audit flow, not by a test — which is why the
frontend now has a Playwright suite.

`backend/internal/models/dossier.go` declares `Anomalies []Anomaly`,
`RedirectChain []string`, `Reasons []LinkReason` and `Sources []Source` as
slices. Go marshals a nil slice as `null`, so every live audit emits:

```json
"ela":  {"anomalies": null}
"link": {"redirect_chain": null, "reasons": null}
"fact": {"status": "", "sources": null}
```

`api/dossier.schema.json` and `frontend/src/lib/api/schemas.ts` both require
arrays and a `FactStatus` enum, so `parseDossier` threw on every real audit and
the UI showed "Pemeriksaan terhenti sebelum selesai" — a network message for a
contract bug.

**Frontend mitigation shipped:** Zod now accepts `null` and normalises to `[]`,
and `""` to `UNVERIFIED`; `parseDossier` throws a distinct `ContractError` so
the UI no longer blames the network. This is a safety net, not a fix.

**Owner: backend.** The real fix is to initialise these slices to empty values
before marshalling (or mark them `omitempty` and make the contract optional),
then tighten the Zod schema back to plain arrays. Until then the contract
document and the wire format disagree, and any client that trusts the schema
strictly will break.

Do not "fix" this by loosening the JSON Schema alone — that hides the drift
instead of removing it.

## 2026-11-01 · OPEN · Cross-origin `Retry-After` is unreadable until the API exposes it

`internal/httpapi/audit.go` sends `Retry-After` on a 429, and the frontend reads
it with `response.headers.get('Retry-After')`. Cross-origin, that header is
**not readable** unless the response carries `Access-Control-Expose-Headers:
Retry-After` — `security/headers.go` currently sets only `Allow-Origin`,
`Allow-Methods`, `Allow-Headers` and `Max-Age`.

Consequence: the countdown in "Terlalu banyak pemeriksaan. Coba lagi dalam N
detik." falls back to the default 30 seconds, which may be wrong.

**Frontend behaviour is correct either way** — it degrades to a sensible
default rather than showing nothing — so this is not a blocker.

**Owner: backend.** Add `Access-Control-Expose-Headers: Retry-After` to the
CORS middleware and a test asserting it, then tighten the share spec to assert
the exact countdown instead of only the retry button.

## 2026-11-02 · Multi-stage Dockerization and Nginx Reverse Proxy Architecture

Containerized full stack for cross-platform team usage (Linux x86_64, macOS Apple Silicon arm64, Windows WSL2) with fast rebuilds and zero-setup onboarding.

**Decisions made:**
1. **Multi-stage Dockerfiles:**
   - `backend/Dockerfile`: Multi-stage Go build (`base` -> `dev` with Air hot-reload -> `build` with `CGO_ENABLED=0` static binary -> `prod` on `alpine:3.21` with `ca-certificates`, `tzdata`, and non-root user `10001:10001`).
   - `frontend/Dockerfile`: Multi-stage Node build (`base` with `node:22-alpine` -> `dev` with Vite dev server -> `builder` with static SPA export -> `prod` on `nginx:1.27-alpine`).
2. **Relative API Base & Nginx Reverse Proxy:**
   - Production frontend static build injects `VITE_API_BASE_URL=/api/v1` (relative).
   - Nginx reverse-proxies `/api/` to `http://backend:8080/api/`.
   - Why: Keeps frontend and API on the same origin (`http://localhost` or LAN IP). Completely eliminates CORS configuration errors and CSP `connect-src` violations when team members test on local networks or mobile devices.
   - SSE streaming: Nginx location block explicitly disables buffering (`proxy_buffering off; proxy_cache off; chunked_transfer_encoding off;`) and extends read timeout to 600s so real-time audit streaming works without delay.
3. **Hot-Reload Dev Architecture (`docker-compose.dev.yml`):**
   - Backend mounts `./backend:/app` and runs Air. Air's temporary build directory is pinned to `/tmp/air` in container filesystem, preventing root-owned build artifacts from polluting the host repository.
   - Frontend mounts `./frontend:/app` with an anonymous volume for `/app/node_modules`. This prevents host OS node binaries (e.g., Darwin or Windows) from overriding container Linux binaries.
   - Sets `CHOKIDAR_USEPOLLING=true` for reliable file-change detection across WSL2 and macOS VirtioFS virtualized mounts.
4. **BuildKit Layer Caching for Fast Rebuilds:**
   - Dependency manifests (`go.mod`, `go.sum`, `package*.json`) are copied and installed with cache mounts (`/go/pkg/mod`, `/root/.cache/go-build`, `/root/.npm`) before application source code.
   - Code changes rebuild in 2-5 seconds without re-downloading packages from the network.
5. **Team Command Ergonomics:**
   - Provided root `Makefile` (`make up`, `make dev`, `make build`, `make logs`, `make down`).
   - Standardized on `docker-compose.yml` (demo/prod-like stack) and `docker-compose.dev.yml` (dev override).

Revisit: if deploying to Kubernetes or multi-node clusters, replace docker-compose with Helm charts / K8s manifests and add ingress TLS termination.

## 2026-11-05 · v10.1 is the contract; the downloaded v9 PRD is superseded

The file supplied for this realignment is PRD **v9.0.0**. `instruction/PRD_AMBA_v10.md`
says it supersedes v9.0.0 and v10.0, and it reverses several v9 choices
deliberately: Svelte 5/SvelteKit instead of React 19, a warm technical sticker
system instead of blue glassmorphism, server-authoritative ELA instead of a
client score, degraded real verdicts instead of a golden-cache fallback, and a
signed fragment token instead of `/hasil/[id]`.

Decision: keep the repo on **v10.1** and close the gap to v10.1. Reverting to
v9 would undo working, tested architecture. Revisit: only if the competition
rules are changed to require the v9 stack.

## 2026-11-05 · Colour is role-based, never ink/white pairs

`--ink` is the text colour and flips between themes. Using it as a background
produced near-white panels with dark text in dark mode, and pairing it with
`--lime`/`--yellow`/`--coral` produced 1.5–1.7:1 contrast.

Decision: every surface maps to one of four roles — `--on-fill` (pastel fills),
`--on-panel-dark` (deliberately dark panels), `--on-brand` (brand fills),
`--on-ink` (the few light ink fills). `tools/check-contrast.mjs` checks both
themes. Revisit: if a fifth surface family is introduced, add a role rather
than a one-off hex.

## 2026-11-05 · SUPERSEDED 2026-11-08 — Demo case IDs differ from PRD F8

The IDs now follow PRD F8; see the 2026-11-08 entry below.


PRD F8 names `demo-tiktok`, `demo-berita-ig`, `demo-resmi`. The implementation
ships `contoh-hoax-bantuan`, `contoh-tautan-mencurigakan`, `contoh-informasi-aman`
(backend and frontend copies byte-identical, CI-diffed). The behaviour the
acceptance criteria describe is met: three synthetic cases, `is_demo: true`,
visible "CONTOH / SIMULASI" banner, zero network calls from the SPA, valid
against the Dossier schema.

Decision: keep the Indonesian IDs — they are the user-facing labels and the
demo is synthetic, so the v9 media-platform names would overclaim. Revisit: if
the jury deck or PRD must match literally, rename in one commit across
`backend/internal/demo/demo-cases.json`, `frontend/src/lib/data/demo-cases.json`,
and `backend/internal/httpapi/demo_test.go`.

## 2026-11-05 · Batch files are stored verbatim (`-text`), not normalised

Windows has no `make`, so `start.bat` / `start-dev.bat` / `stop.bat` mirror the
Makefile targets. They must reach a teammate as CRLF: `cmd.exe` expects it and
can mis-parse parenthesised `if errorlevel` blocks on bare LF.

`.gitattributes` therefore declares `*.bat -text`, i.e. **no conversion at
all**. `text eol=crlf` would store LF in the repository and convert only on
checkout, so any path that reads the blob without a checkout (an archive, a
copy from the web UI) would hand over LF. With `-text` the committed bytes are
the bytes every consumer receives.

`whitespace=cr-at-eol` is needed alongside it: without it `git diff --check`
reports every CR as trailing whitespace and the pre-commit gate fails.

Revisit: if the team standardises on a Windows-only toolchain, `eol=crlf`
would also work; `-text` stays correct either way.

## 2026-11-06 · Maru is the engine; the model never supplies a source

The gateway is an OpenAI-compatible LLM gateway with the `Maru` combo. Four behaviours found by probing
it directly shaped the client, and each one is now covered by a test:

1. **The gateway streams by default.** A request without `stream:false` returns
   `text/event-stream`, which the old client could not parse. Every call now
   sends `stream:false` explicitly *and* decodes an SSE body if the gateway
   streams anyway.
2. **Maru ignores `response_format`.** It returns its own keys (`patterns`,
   `reasoning`, `skor`) and sometimes wraps JSON in a ```json fence. The
   decoder accepts both the requested schema and these aliases; only a missing
   numeric score fails the pillar. A strict unmarshal discarded a usable
   assessment on every image audit.
3. **Reasoning is always on.** A one-word answer costs ~5–14 s at default
   effort. `reasoning_effort:"low"` brings a realistic audit to 2–4 s, so it
   is the default and `HARD_TIMEOUT_MS` rose from 12 s to 20 s to leave image
   audits room. Images are downscaled to 1600 px for the model (`internal/imaging`);
   ELA still runs on the original bytes.
4. **`fact_status` was removed from the model's output.** The model cannot
   verify anything, and asking it for a verdict produced confident
   `CONTRADICTS` with an invented URL. Retrieval now owns the fact pillar:
   a fixed news endpoint (Bing News RSS) → allowlist filter → claim/number
   match → deterministic debunk/support markers. The model contributes only a
   search query and a claim.

A retrieved item must also share a **number** with the claim. Without that
gate, a fact-check about "bantuan Rp 900.000" was cited as contradicting a
claim about "Rp 500.000" — a false citation with a real, allowlisted URL.

## 2026-11-06 · Mock mode reports an honest UNVERIFIED fact pillar

Retrieval is skipped in mock mode (previews and e2e must stay offline). The
fact pillar is still `available:true` with `status:"UNVERIFIED"` and no
sources: the claim exists, nothing was checked. Marking the pillar unavailable
instead would have pushed text audits to `INSUFFICIENT` and broken the e2e
contract test for no product benefit.

## 2026-11-06 · Every unconfigured integration is a labelled placeholder

`GET /api/v1/ops/status` reports each dependency as live, disabled or
placeholder, with no key, host or model output. Safe Browsing without
`GOOGLE_SAFE_BROWSING_KEY` is a placeholder; retrieval without a live model is
a placeholder; mock mode labels the AI pillar a placeholder. The alternative —
silently skipping a check — is the dishonesty the trust contract forbids.

## 2026-11-06 · `/reply` drafts with the model, validates, then falls back

`internal/reply` asks Maru for a ≤ 280-character draft using only the verdict
summary already in the share token (never raw user content). The draft is
rejected wholesale if it contains certainty words or shaming, and every URL
outside `{token sources, share link}` is stripped. A rejected or unavailable
draft returns the deterministic status × tone template, and `source` says
which one the user received. The template matrix has a parity test against
`configs/reply_templates.json`.

## 2026-11-06 · Netlify for static hosting, not Vercel

The frontend is a static SPA (`adapter-static`); either host can serve it. The
decision is licence-based, not technical:

- **Netlify Free/Starter** permits commercial use, so the project does not need
  a paid plan if it later ships as a product.
- **Vercel Hobby** is restricted to non-commercial personal projects. Moving to
  Pro to satisfy that is a cost the project does not need.

The Go API is **not** deployed to either: it holds 20 s SSE streams and makes
outbound fetches, which do not fit serverless function limits. `netlify.toml`
therefore ships the SPA only, and the API base is configurable at build time
(`VITE_API_BASE_URL`). A preview-only deployment can build with
`VITE_API_MODE=mock`; it is honest because the app labels every mock result.

Two deploy bugs were fixed on the way: `connect-src` hardcoded
`http://localhost:8080` (a deployed build would block every request), and the
production API base defaulted to localhost instead of same-origin `/api/v1`.

## 2026-11-06 · Palette: three distinct hues, no pink

`--coral` was `#ff879c` (pink) and `--lime` was `#ffad87` (peach). The home
page also ran a `--copal-1/2/3` ramp that turned lime, amber and vermilion into
three shades of orange. Result: every accent collapsed into one peach family
and the danger state read as decoration.

Restored the spec's intent — a strong warm trio — while keeping the warm-orange
concept: `--lime #c8f751`, `--yellow #ffc53d`, `--coral #ff4b3e`, `--brand
#ff5a1f`. All 82 contrast pairs pass in both themes. Layout is untouched.

## 2026-11-06 · Home page: fabricated progress and badges removed

The "Pelajaran singkat" card showed made-up completion percentages (100/60/25/5)
for lessons that did not exist, and "Kumpulkan lencananya" advertised badges
with no storage. Both are gone: lessons are links to real `/edukasi` anchors,
and badges are real next-step links. "Buka panduan keluarga" now opens a new
`/panduan-keluarga` page with a client-only safe word and a printable card
(PRD F15), instead of pointing at the six-question self-check.

## 2026-11-06 · Safe accent: warm ivory instead of green

The electric lime (`#c8f751`) used for SAFE status, the hero highlight and
step 2 clashed with the copal orange. Replaced with warm ivory
(`--safe #efe0c9` dark / `#ecd9b8` light) and renamed the token from `--lime`
to `--safe`: a cream token called "lime" would be a lie, and the name now says
what it means.

Ivory was chosen over teal and cobalt because it keeps the entire palette warm
— no cool hue is left to clash with copal. Trade-off: SAFE reads calmer than a
saturated green, which is acceptable because status is never colour alone
(icon + label always ship).

Cream is a **surface, never text**: cream on `--brand` is 2.5:1. Three
hover/link rules that used it as text were moved to `--on-brand` / `--yellow`,
and `.btn--safe` was removed from the brand-filled control list.

## 2026-11-06 · Rebuilding one compose service recreates the other

Running `docker compose build frontend && docker compose up -d frontend` after
editing `docker-compose.yml` recreated `backend` with the file's defaults,
which are `LLM_MOCK=true` and an empty key. The app silently dropped to mock
mode. When restarting either service, pass the live env inline (or export it)
so the backend keeps `LLM_MOCK=false` and `NINEROUTER_API_KEY`:

```bash
LLM_MOCK=false NINEROUTER_API_KEY=... docker compose up -d --force-recreate backend
```

`GET /api/v1/ops/status` reports `llm.mode`, so this is detectable rather than
silent.

**Fix:** a gitignored root `.env` now holds the live values. Docker Compose
reads it automatically, so any `docker compose up` / `restart` keeps
`LLM_MOCK=false` and the key. `chmod 600`; it is never committed
(`.gitignore` line 2).
## 2026-11-08 · Strong link evidence may decide on its own (`LINK_STRONG`)

Team decision ("bukti tautan kuat cukup"). When no weighted score exists
(W < 0.30, typically a bare URL) and the link pillar scored at least 50, the
link score becomes the score, capped at 85 so it never exceeds what a
Safe Browsing hit or a link score >= 90 earns. It never produces SAFE and runs
after the first three floors (TECH_SPEC §3.3 floor 3b). Vectors 9-12 cover 85,
weak AI + 55, 45 (still INSUFFICIENT) and the 89 cap. Lead sentence: "Tautan
ini punya tanda penipuan. Sebaiknya jangan diklik." Revisit: the threshold of
50 if field data shows false alarms on ordinary links.

## 2026-11-08 · Contract additions: `source_metadata` and `ocr_text`

Both optional and nullable, added contract-first (OpenAPI → JSON Schema → Go
→ Zod) in one commit. `source_metadata` carries the link's public preview
(status OK | BLOCKED | UNAVAILABLE, platform, title, author, description,
site name) so the result can say "Isi halaman tautan tidak dapat dibaca" and
offer the screenshot route instead of pretending the content was checked.
`ocr_text` is the model's reading of a screenshot; tactics found in it are
labelled `source: "ocr"`.

## 2026-11-08 · Search provider: Google Fact Check Tools, verified in Go

The model never searches and never states a fact status. Go sends the
model's `search_query` to Fact Check Tools (`claims:search`, Indonesian),
keeps at most 5 allowlisted https results, and a second structured call
classifies each one against the claim. Only URLs from that result set can be
verified; with no result set nothing verifies (fail closed). This replaces
the model-driven tool rounds sketched in TECH_SPEC §6. Revisit: a news search
API could be added behind the same `Searcher` interface.

## 2026-11-08 · Progressive SSE and per-step budgets

Each pillar sends exactly one terminal `stage` event and its `partial` as soon
as it finishes; a `: ping` heartbeat keeps proxies from closing a quiet
stream. Budgets: ELA 4 s (a 12 MP photo takes about 1.4 s on a dev machine
and longer on a shared-CPU host, so the spec's 0.8 s would drop it), link 2.5 s,
metadata 2.5 s, model 9 s, search 2.5 s, classify 3 s, each bounded by the 12 s
hard deadline minus a 300 ms reserve so the verdict always gets out.

## 2026-11-08 · Demo IDs follow PRD F8, demos are generated

`demo-tiktok`, `demo-berita-ig`, `demo-resmi`, plus `demo-tautan` for the link
flow. The old `contoh-*` IDs remain as aliases. `go run ./cmd/gendemo` draws
the synthetic screenshot, runs the real forensics and scorer, and writes both
JSON copies, the screenshot and the Edukasi quiz images; CI regenerates them
and fails on any diff, so a demo can never show a verdict the code would not
give.

## 2026-11-08 · Balas Sopan: model draft, validated, template fallback

The model gets only the verified token payload (status, title, claim,
sources). Drafts over 280 characters, with a URL outside the sources, or with
certainty, score or shaming words are rejected and the status x tone
template is used. The share link is not given to the model: a full token
does not fit in 280 characters, and the share button already carries it.

## 2026-11-08 · Proof card keeps the short disclaimer line

PRD §10 asks for the full disclaimer "in footer and proof card", while F6 asks
for an "INDIKATIF — bukan alat bukti hukum" line legible in a WhatsApp
preview. A 100-word paragraph on a 1200x630 card is unreadable at preview
size, so the card keeps the short line and its QR code opens the shared
report, whose footer carries the full text verbatim. The footer of every page
now shows the full §10 text in Indonesian (`lang="id"`), in both languages.

## 2026-11-08 · English toggle, Indonesian by default

P1 EN/ID toggle. Indonesian stays the default and the reference copy
(STATUS.md rule 6 still holds for the default experience). English verdict
text is rebuilt from the verdict codes rather than machine-translated from
the server's Indonesian sentences, so nothing is invented. Polite replies
stay Indonesian (they are sent to Indonesian chats) and the English UI says
so. The choice is stored in `amba.lang` and applied before first paint.

## 2026-11-08 · Deployment in scope: Vercel + Fly.io or Railway

Team decision. The SPA stays static on Vercel; the API runs on Fly.io
(Singapore, one warm machine so a cold start cannot eat the 12 s budget) or
Railway. CSP `connect-src` is generated from `VITE_API_BASE_URL` at build
time. Production refuses to boot with the mock model, without a signing key,
or without an https `PUBLIC_APP_URL`. Client identity comes from the host's
edge header (`Fly-Client-IP`, Railway's `X-Real-IP`), never from a header a
client can append to. Guide: `docs/DEPLOY.md`.

## 2026-11-08 · Telegram bot shares the audit pipeline

P2. Webhook with Telegram's secret header (or long polling locally), the same
pipeline and verdict as the web, a per-chat limit (3 then 1 per 10 s), photos
and documents through the same upload validation, and a plain-text reply with
the report link. Nothing is stored and chat IDs are never logged. Safe
Browsing is free for non-commercial use only; commercial use would need Web
Risk.

## 2026-11-08 · PARTLY RESOLVED 2026-11-09 — the ELA score is a global mean

TECH_SPEC §3.4 defines the ELA score as the image-wide mean difference x 3.5.
A small pasted region lights up clearly in the heat map but barely moves the
mean: the edited demo screenshot scores 3/100 although its price line glows
as a bright box. Consequences: `ELA_CORROBORATED` (needs >= 70) almost never
fires on ordinary photos, and the ELA pillar pulls a weighted score down even
when the heat map shows an edit. Today the AI pillar (vision model) and the
fact pillar carry such cases. Options: score the contrast between the hottest
grid cells and the image's typical cells, or weight hotspots that overlap
model anomaly boxes. Either changes §3.4 and needs new forensics tests with
false-positive cases (text on flat backgrounds, PNG screenshots) before it
ships.

## 2026-11-09 · Default model: OpenRouter's free Nemotron 3.5 Super VL

Team request: a free multimodal model through OpenRouter. Chosen:
`nvidia/nemotron-3.5-super-vl:free` (released 15 Oct 2026): it takes images,
lists `structured_outputs`/`response_format`, has a 1M-token context and showed
about 1.6 s median latency and 99.8 % availability on OpenRouter. The free
Gemma 4 endpoints also take images, but a third-party benchmark saw them
answer none of its free-tier requests.

`LLM_EXTRA_BODY` (a JSON object merged into every request; the contract
fields always win) carries the OpenRouter specifics instead of hard-coding a
provider: `models: [primary, "openrouter/free"]` falls back to OpenRouter's
free router, which picks another free model with the needed features, and
`reasoning: {enabled: false}` keeps answers inside the 9 s model budget. Only
`:free` models are listed, so a fallback can never be billed.

Limits to know: 20 requests per minute and 50 per day on free models (1,000
per day once the account has bought $10 of credit); one audit uses up to two
calls. Not verified against the live API: this development environment
cannot reach openrouter.ai, so the first real check is the boot readiness log
and the smoke tests in `docs/DEPLOY.md` §6. Revisit: when the free model is
retired, when the quota is too small, or if the free provider's data policy
conflicts with the privacy copy on `/tentang`.

## 2026-11-09 · Link heuristics tuned on real phishing and legitimate URLs

Tested the static link pillar (no Safe Browsing, no model) on real data: a
public phishing feed (mitchellkrogza/Phishing.Database, 789,052 active links;
the 1,053 Indonesian-themed ones, the 10,406 on `.id` domains and a random
3,000) and on legitimate URLs (the top 10,000 domains plus 60 official
Indonesian bank, marketplace, government and news URLs).

Findings and changes:
- 56 % of the Indonesian-themed phishing links scored exactly 45 (a brand in
  a non-official host, nothing else) and read "belum cukup data". The
  `LINK_STRONG` threshold moves from 50 to 45; no legitimate URL in either set
  reached 45 before the change.
- TikTok and YouTube links showed "Tautan memuat karakter @ yang
  menyesatkan" because of the `/@handle` segment. A handle segment no longer
  counts.
- `.my.id` and `.biz.id` hosted a third of the Indonesian-themed phishing and
  none of the legitimate sites: now `SUSPICIOUS_TLD`.
- Brands impersonated in the feed were added (BTN, BRImo, SeaBank, Akulaku,
  Kredivo, Pegadaian, Kemensos, Prakerja, PLN, Telkomsel, IndiHome, Gojek),
  with their official domains; Lazada's and Shopee's regional domains and the
  word "telegraf" stopped being flagged.

Result: a warning verdict for 83.6 % of the Indonesian-themed phishing links
(was 25.4 %), 13.1 % of the `.id` ones (was 2.2 %) and 4.5 % of the random
global sample (was 2.4 %); 4 of 10,060 legitimate URLs (dana.ir, bsi.ir,
instabam.gr, online-instagram.com). Generic phishing on compromised or
free-hosting domains has no tell-tale in the URL: that is what Safe Browsing
is for, so `GOOGLE_API_KEY` matters in production.

The same session ran server ELA on 7 real images from public forensics
repositories (2 authentic, 5 edited, CASIA and TruFor samples). Scores were
0 and 0 for the authentic images and 0-14 for the edited ones, so the global
mean does not separate them (see the OPEN ELA entry above); the heat map lit
the edited region of one CASIA sample fully, the other only at chance level.

## 2026-11-09 · SUPERSEDED the same day — Deterministic scam rules and the `SCAM_PATTERN` floor

A scam message has no claim to fact-check, so only the model read its
wording, and the model scored "AI-generated likelihood". When the free
gateway was down or out of its 50 daily requests, "kirimkan kode OTP itu ke
saya" came back as "belum cukup data". `internal/scam` (TECH_SPEC §6.3) now
recognises ten patterns of the scams sent most often to Indonesian phones,
without the model: OTP/PIN/card requests, APK lures, wrong-transfer refunds,
task jobs and job fees, implausible returns, prizes or aid with a fee or an
unofficial link, loans with a fee up front, money requests from a "relative"
or "officer", account-blocking notices pointing to an unofficial link or
number, and a bank service line on a number the bank does not use.

- Floor 2b `SCAM_PATTERN` = 80: inside `TAMPERED`, below the 85 of a fact
  checker's verdict or a malicious link, because wording is weaker evidence.
  It also applies when no weighted score exists. `verdict.scam_pattern`
  (contract change) names the pattern; each has one plain sentence in the
  explanation (Indonesian from the server, English rebuilt from the code).
- The rules run before the model and again on the model's OCR; their cues
  become highlighted tactics, so a scam still gets a verdict and highlights
  during an outage.
- Links written without `https://` (`bit.ly/x`, `bri-verifikasi.my.id`) were
  never checked in text audits; they now get the link pillar, read as https.
  An e-mail address is not a link.
- Banks' own WhatsApp numbers look like ordinary mobile numbers, so they are
  listed and checked (November 2026): Halo BCA 0811-1500-998 (bca.co.id),
  BRI Sabrina 0812-1214-017 (Bisnis.com, 14 August 2024), Mandiri MITA
  0811-8414-000 (bankmandiri.co.id). Only these three banks' service lines
  are checked for `UNOFFICIAL_CONTACT`.

Measured honestly: each new set of messages was written before the rule
changes it led to and measured once on first sight. Set 3 (account threats,
new fees, customs) caught 0 of 15 and raised 5 false alarms in 25 ordinary
messages (registration fees, a utility reminder), which led to the
account-threat pattern and to the rule that a threat with a payment needs an
unofficial destination. Sets 4-6 then caught 11/20, 10/20 and 14/25 on first
sight, with 0, 0 and 1 false alarms (a stand job paid per day). Everyday
probes found code requests that were not secrets: a location pin, a Wi-Fi or
Zoom password, a card-data warning. All sets are regression tests now
(`internal/scam/scam_test.go`).

So the rules catch about half of the scams they have not seen, with about 1
false alarm in 65 ordinary messages; the model and the link check have to do
the rest. Revisit: when real user reports show a pattern the rules miss or a
false alarm, add the message to the tests first.

## 2026-11-09 · One-sided image analysis; the AI score measures manipulation risk

Two pillars pulled verdicts the wrong way on real inputs.

ELA: on real images the global mean was 0 for untouched photos and 0-14 for
edited ones (the demo's pasted price line scores 3), yet it carried weight
0.30 in the mean. A low ELA therefore dragged scores down and, with a clean
link, could make an image SAFE with no fact check at all. Now ELA only
counts as evidence of editing, from 50 up (`scoring.ELAEvidenceMin`). Below
that it is reported as checked, carries no weight, and only counts as
coverage for SAFE; SAFE also needs an available fact check (the ELA+Link
route is gone). The evidence card says that a low average is not proof of an
original and that small edits often show only in the heat view. A localised
score was tried on the same images: the hottest-cell-to-mean ratio was 64
and 9.8 for the two untouched photos and 2.7 for an edited one, so contrast
alone measures texture, not edits. A texture-normalised score stays open
until there is a labelled set large enough to test it on.

AI: the model scored how likely content was made or changed by AI, which is
near 0 for a hand-typed scam, so the pillar pulled scam messages towards
SAFE. `ai_score` now follows a written rubric of manipulation risk (scam,
hoax and edit signs, with AI generation as one of them; warnings about scams
are not scams), and the UI calls the pillar "Penilaian AI". Its weight and
reliabilities are unchanged, and it never produces a verdict on its own.

Vectors 15-17 (TECH_SPEC §3.5) pin the new behaviour. Revisit: with a
labelled image set, or if the rubric scores drift on real traffic.

## 2026-11-09 · The link check reads the destination page

The phishing feed showed that phishing on hacked or free-hosting sites has
nothing suspicious in its address. The page gives it away: a bank's name
over a password field. When the redirect chain ends on a host that is not
official, the pipeline now reads that page once (SSRF-safe client, at most
512 KB, inside the 2.5 s link budget) and looks only at its title, site name
and input fields. A field for a password, PIN, OTP, card number or CVV adds
`PAGE_ASKS_SECRET` (20); with a brand named in the title it is
`PAGE_IMPERSONATION` (45, high severity, as strong as a lookalike domain).
Official domains are never fetched for this, and nothing of the page is
kept. "dana", "pajak", "qris" and "permata" are ordinary words in titles, so
they are not brands there.

Trade-off: the server now loads a phishing page the user has not opened.
Unrolling the link already requested it, so a scammer could already see a
visit; the page is never rendered or stored. Not measured on live phishing
pages: this environment cannot reach them. Revisit: after the first
production week, with the reason counts from the metrics endpoint.

## 2026-11-09 · SUPERSEDED the same day — Scam rules measured on real SMS

The synthetic sets above are mine. To measure on real messages, the rules
ran on a public corpus of Indonesian SMS from 2013-2016
(`dataset_sms_spam_v1.csv` in the GitHub repository
Andikazidanef15/Sentiment-Analysis-on-Indonesian-SMS-Dataset; 569 personal
messages, 239 operator promos, 335 fraud and spam). The corpus is not copied
into the repository.

- First sight: 0 of the 808 personal and promo messages flagged, 79 of the
  335 fraud/spam messages.
- The corpus was split by row (even rows to tune, odd rows held out). Tuning
  added SMS shorthand for prizes ("M-dptkan hadiah", "PIN pemenang",
  "hadiah ke-2", "cek Rp 27 jt"), top-ups for a "new number", payments
  "sudah saya kuasakan" to someone else, and a new pattern `CALL_FORWARD`
  for dial codes that carry a phone number (`*886*…*08…#`, `**21*…#`), which
  divert calls and SMS, OTP codes included. A dot followed by a letter or
  digit ("No.anda", "Rp.1.500.000") no longer ends a sentence.
- After tuning: still 0 of 808 flagged; the held-out half went from 40 to 80
  of 168 fraud/spam messages (172 of 335 overall).

The fraud/spam class also holds plain ads (loans, gadgets, gambling), the
account numbers scammers send after a call (indistinguishable from an
ordinary transfer request), fake buyers for a house, and summonses to call
an "official". Those stay with the model. SMS-style regression tests use
placeholder numbers, not the corpus text.

## 2026-11-09 · Scope: links and images only

Team decision (supersedes the two scam-rule entries above): AMBA checks two
inputs, a **link** and an **uploaded image**, for three things:

1. whether an image was generated by AI or is authentic (Jejak AI);
2. whether an image is a hoax whose text was pasted over (Timpa Teks);
3. whether a link is genuine or a fake or hoax (Keamanan Tautan, plus a fact
   check of the claim the page or image carries).

Removed: the text audit (`POST /audit/text`, `input_type: TEXT`, the Teks tab),
the SMS and phone scam rules (`internal/scam`, the `SCAM_PATTERN` floor and
`verdict.scam_pattern`), and Cek Cepat with Sandi Keluarga (PRD F3 and F15).
Shared text without a link, a pasted text, and a Telegram message without a
link are answered with "AMBA memeriksa tautan dan gambar". Links inside an
image are still checked, also when written without `https://`. The demos are
now two images and three links; the home, education and about copy talk about
images and links instead of messages and calls. The pillars are named after
the detections: "Jejak AI", "Teks ditimpa (ELA)", "Pengecekan fakta",
"Pemeriksaan tautan". The PRD carries a scope note at the top.

## 2026-11-09 · Jejak AI: what the file declares comes first

Scope (see above): "is this image AI-generated or authentic?". The AI pillar
now answers only that question, for the uploaded image or a link's preview
image. It supersedes the "Penilaian AI" manipulation-risk rubric decided
earlier the same day.

Evidence, strongest first (TECH_SPEC §3.6):

1. **What the file declares** (`internal/provenance`): Content Credentials
   (C2PA in JPEG APP11, PNG `caBX`, WebP `C2PA`), IPTC `DigitalSourceType` in
   XMP, a generator as XMP `CreatorTool` or EXIF `Software`, generator
   settings in PNG text (Stable Diffusion, Fooocus, ComfyUI, InvokeAI,
   NovelAI, Midjourney) or the EXIF user comment, and camera make and model.
   Only metadata blocks are read, never pixels. Checked by hand on real
   files: the c2pa-rs test image (algorithmicMedia) and its edited copy
   (opened, no AI), three ComfyUI outputs (FLUX, SDXL), a Canon photo whose
   EXIF ImageMagick copied into PNG text, and a Pixel 7 Pro photo; the unit
   tests use crafted bytes of the same structures, and a fuzz target guards
   against hostile files. A visible AI label ("Imagined with AI", "Dibuat
   dengan AI", "AI生成") counts only when the model reports it and the
   image's OCR text carries one.
2. **An optional dedicated detector**: Sightengine's `genai` model, off
   without credentials. The image goes to a third party, so the privacy
   notice says so.
3. **The model's visible clues**, with a written rubric: 0-15 an ordinary
   photo, screenshot or design; 71-100 clear signs such as malformed hands,
   gibberish lettering, impossible objects or an AI label. An ordinary edit
   (pasted text, crop, filter) is not AI generation; it belongs to Timpa Teks.

Missing metadata proves nothing (WhatsApp and social networks strip it), so it
never lowers the score; camera data is only a hint and caps a visual judgement
at 50. Floors apply to uploads only: `AI_PROVENANCE` (85), `AI_EDITED` (70) and
`AI_LIKELY` (45, from a pillar score of 80). They run after `LINK_STRONG`, so a
risky link in the same image keeps its score. A link's preview image is judged
too, without floors: an AI illustration does not make a link a hoax.

Open: the `AI_LIKELY` threshold (80) is set from the rubric, not yet measured
against the live model on a labelled set, because no model key was available
when this was built. Revisit with real traffic or a labelled set.

Found on the way: WebP uploads were rejected because the standard library has
no WebP decoder, although the UI accepts them. The size now comes from the
VP8/VP8L/VP8X header; WebP is passed to the model and detector as is, and ELA
is skipped for it (re-compressing as JPEG says nothing about a WebP file).

## 2026-11-09 · Timpa Teks: compare the marked text with the image's other text

The global ELA mean does not separate edited from authentic images (see the
one-sided ELA entry above), and a localised hot-cell contrast measured
texture, not edits. What gives a pasted line away is that it was compressed
fewer times than the text around it: a news graphic's lines are all drawn at
once, a retyped price is added after the graphic circulated.

`forensics.TextEvidence` therefore compares, for each box the model marks as
looking pasted, the text's reaction to one more JPEG compression (q75 and
q90, normalised by edge strength) with the reaction of the other text-like
blocks of the same image. Benchmark: `tools/textbench/gen.py` builds, from 34
real camera photos (ianare/exif-samples), a caption-bar graphic saved by a
publisher and recompressed by a messenger, and the same graphic with its price
covered and retyped in an editor, forwarded again about half the time. Two
sets with different seeds: the thresholds were set on the first and checked on
the second.

| | set 1 | set 2 |
|---|---|---|
| original lines with evidence ≥ 50 | 0 / 34 | 0 / 34 |
| retyped lines ≥ 50 (≥ 70) | 22 / 34 (21) | 14 / 34 (11) |
| ... not forwarded after the edit | 15 / 21 | 9 / 18 |
| ... forwarded once more | 7 / 13 | 5 / 16 |

Ratios over the median of the other text: originals 0.87-1.34, retyped lines
0.89-7.0. Boxes 30 % too large or shifted by a tenth gave the same picture. A
box without lettering (photo texture) is not measured, since it would always
stand out against text.

So evidence 70 is a strong sign and becomes a hotspot for `ELA_CORROBORATED`
(the model and the measurement point at the same text); evidence 50 counts in
the ELA weight; low evidence says little, because a forward or a resize after
the edit can erase the difference. The demo screenshot's pasted price now
measures 100, and its verdict carries `ELA_CORROBORATED`. Revisit with real
hoax images from fact-checkers once there is a labelled set.

## 2026-11-09 · Domain age from RDAP

Throwaway phishing and hoax sites are typically registered days or weeks
before they are used; the sites people trust are years old. The link check
now asks the domain's registry over RDAP (no key, IANA's bootstrap file finds
the registry) when the registrable domain was registered, for the first and
the final host of the redirect chain, beside the redirect check and inside
the link budget. `NEW_DOMAIN` (≤ 30 days, 25 points) and `YOUNG_DOMAIN`
(≤ 180 days, 10) take the youngest domain on the chain, so an old shortener
or redirector does not vouch for a new destination. Official domains and
shorteners are not looked up.

A young domain alone never reaches `LINK_STRONG` (45): new shops and campaign
sites exist. With a lookalike brand (45) it adds up to a clear warning.

Privacy: only the registrable domain goes to the registry (never the path,
query or the user's text), answers are not kept, failures are logged with a
status code only, and `DOMAIN_AGE_LOOKUP=false` switches it off. The privacy
notice on /tentang says so. Tested with a fake registry; the development
environment cannot reach real RDAP servers, so live coverage per TLD (.id in
particular) still has to be checked after deploy.

## 2026-11-10 · Video thresholds from 33 labelled clips, creator disclosure and web provenance

Data: three runs of the old pipeline over 33 social clips (11 real news, 10 AI,
12 hoax; labels verified by a second reviewer), pooled to 33 real, 30 AI and 36
hoax readings.

| AI score line | real flagged | AI flagged | hoax flagged |
|---|---|---|---|
| 45 | 2/33 | 18/30 | 6/36 |
| 50 to 65 | 1/33 | 16/30 | 2/36 |
| 70, 75 | 1/33 | 14/30 | 2/36 |
| 80 (old) | 0/33 | 7/30 | 2/36 |

- `VideoAILikelyMin = 55` (video only). The plateau is 50 to 65. At 80 only 23%
  of AI clips were flagged; at 55, 53%. The image line `AILikelyMin` stays 80:
  this set has no image rows. `AI_LIKELY` lifts the score to 45 only, never to
  `TAMPERED`.
- `VideoManipulationMin = 70`, unchanged: real news never exceeded 60.
- `VideoContextMin = 85` (was 70): real news scored 75 and 80 for context twice
  (same clip). Context mismatch is the noisiest signal, so it trades recall for
  precision.
- Combined (55, 70, 85): real 1/33 (3.0%), AI 16/30 (53%), hoax 7/36 (19%). Old
  combined (80, 70, 70): real 2/33, AI 10/30, hoax 10/36.
- Creator disclosure (deterministic, `video/disclosure.go`): a strong
  disclosure sets provenance source `creator` and the `AI_PROVENANCE` floor (85);
  a weak one adds 8 to the AI score when the model answered. On the same URLs:
  strong on 9/10 AI clips, 0/11 real, 0/12 hoax. The AI class is official
  generator channels, so this is an upper bound; anonymous reposts carry no such
  text.
- Web provenance (`pipeline/webprov.go`): the title or first description
  sentence, with links, hashtags and emoji removed (3 to 10 words), plus
  " video AI hoaks", searched on Tavily over a 26-host allowlist. Hits pass
  `SameTopic`, then a model classification (AI, REAL, HOAX, UNRELATED); HOAX also
  needs a fact-checker host. Any failure returns no evidence and never marks the
  audit degraded. This signal is not measured by the offline set.
- 90% overall accuracy is not claimed. The web search needs a live run.
