# AMBA — Visual Design System (implementation record)

> Status: **implemented** in `frontend/`. Source of truth for the look:
> `docs/waspada-design-spec.md`. This file records how that spec maps onto the
> real SvelteKit app, and the decisions taken where the spec left room.
>
> Update 2026-11-10 (v2): the text-size toggle (A / A+ / A++) and the Jelajah and
> Esensial modes were removed. Mentions of them below describe the earlier design
> and no longer apply: there is one presentation, flat (no stickers, tilt, marquee
> or gradients), with the same dark and light tokens. Reduced motion is still
> honoured through `prefers-reduced-motion`.

---

## 1. Concept in one line

A **warm, sticker-inspired technical surface**: flat saturated fills, fine
borders, soft ambient light, and generous type. Bold enough for a feed, plain
enough for a grandparent. The earlier indigo neo-brutalist spec was replaced by
this warm palette during the November redesign.

**Motion verb: press.** Things lift slightly on hover and settle on press;
the hero stickers bob and the marquee runs, both stopping under reduced motion
or in Esensial mode.

**Signature moment:** the scanner card in the hero. Two tabs (message / link),
each a real scam pattern, with a risk meter that fills and a verdict panel that
retints. It teaches the shape of a verdict before the visitor submits anything.

---

## 2. Tokens

All tokens live in `frontend/src/lib/styles/tokens.css`. Components reference
names only; no raw hex outside that file.

### Colour

The implemented palette is **warm orange**, not the indigo palette in the
earlier source spec. Values live in `frontend/src/lib/styles/tokens.css`.

| Token | Dark | Light | Role |
|---|---|---|---|
| `--paper` | `#070302` | `#fff8f4` | page ground |
| `--white` | `#171210` | `#ffffff` | cards, inputs |
| `--surface-2` | `#211a17` | `#f7eee8` | recessed surface |
| `--ink` | `#fff8f4` | `#170e09` | **text only** (flips between themes) |
| `--brand` | `#ff5a1f` | `#c2410c` | primary action |
| `--on-brand` | `#140800` | `#fff8f4` | text on a brand fill |
| `--safe` | `#efe0c9` | `#ecd9b8` | safe / positive fill (warm ivory) |
| `--yellow` | `#ffc53d` | `#ffc93c` | suspicious fill |
| `--coral` | `#ff4b3e` | `#ff4b3e` | danger fill |
| `--on-fill` | `#120a04` | `#170e09` | text on a saturated fill |
| `--panel-dark` | `#171210` | `#171210` | deliberately dark accent panel |
| `--on-panel-dark` | `#fff8f4` | `#fff8f4` | text on a dark panel |
| `--text-2` | `#d4c9c3` | `#51463f` | paragraph text |
| `--text-3` | `#bfb4ae` | `#665950` | secondary text |

**Three values, not three oranges.** The semantic trio must be readable at a
glance: warm ivory (safe), amber (suspicious), vermilion (danger). An earlier
"copal" refinement mapped all three onto one orange ramp, so a SAFE badge and
a DANGER badge looked like the same state; it also replaced the danger fill
with pink. Both are gone.

The safe fill was briefly an electric lime (`#c8f751`); it was replaced by warm
ivory because a cool neon green clashed with the copal orange. Ivory keeps the
whole palette warm and still separates from amber and vermilion by hue and
value. Cream is a **surface only**: it is never used as text on the orange
brand fill (that pairing is 2.5:1). Every fill keeps dark text via `--on-fill`
(contrast checked, not assumed).

**Why semantic roles instead of ink/white pairs.** `--ink` is the *text*
colour and flips between themes. Using it as a background produced near-white
panels with dark text in dark mode. Every surface therefore maps to exactly one
of four text roles:

1. `--on-fill` — light sticker fills (`--safe`, `--yellow`, `--coral`), light in
   both themes, so the text stays dark.
2. `--on-panel-dark` — deliberately dark panels (`.card--ink`, `.marquee`,
   `.tile--ink`, active nav).
3. `--on-brand` — brand-filled controls (primary button, active tab, logo).
4. `--on-ink` — the few light `--ink` fills (`.tick`, `.q-num`).

`tools/check-contrast.mjs` reads the token file, resolves `var()` aliases, and
checks **both themes**: 82 pairs, including every semantic role. A token that is
only redefined under `html[data-theme='light']` is read from that block.

### Type

| Role | Family | Notes |
|---|---|---|
| Display, headings, buttons, labels | **Bricolage Grotesque Variable** | weight 800, tracking −0.025em to −0.03em |
| Body, UI | **Atkinson Hyperlegible** | 400/700, chosen for low-vision legibility |

Two families, both self-hosted via `@fontsource` so the CSP stays `font-src 'self'`.

Every size clamp keeps a `rem` term in its preferred value
(`clamp(2.25rem, 1.8rem + 2.2vw, 4.1rem)`). A pure `vw` middle value is pinned
by the viewport, so the text-size toggle and Esensial mode would change the root
font and nothing else at desktop widths. Root scale is
`--text-scale × --mode-scale`.

**No monospace anywhere.** Technical labels use the display face at small size.

### Edges and shadows

- Border `1px solid var(--border)` on every card, button, pill, tab and input.
- Radii: cards 26px, buttons 16–18px, pills 99px, chat bubble `22px 22px 22px 6px`.
- Shadows are soft and diffuse (`0 8px 28px #00000020`), never hard offset
  shadows. Hero and CTA panels use the same shadow at a larger scale.
- The only blurred surface is the ambient hero glow, which is decorative and
  marked `aria-hidden`.

---

## 3. Interaction language

One treatment per role, reused everywhere:

| Role | Rest | Hover | Active |
|---|---|---|---|
| Card (`.lift`) | soft shadow | `translateY(-2px)` | no transform |
| Button (`.btn`) | brand fill | lighter brand + lift | deeper brand, no lift |
| Nav link (`.nav-link`) | transparent | tinted pill | brand pill |
| Tab | transparent | tinted | brand fill, `--on-brand` text |
| Input | solid border | — | solid border + focus ring |

- Focus: `4px solid var(--ink)` with 3px offset, on every interactive element.
- Press feedback is never removed, in any mode.
- **Idle motion:** the two hero stickers bob on a 3.6s loop and the marquee
  runs continuously. Both stop under reduced motion and in Esensial mode.

---

## 4. Motion budget

| Token | Value |
|---|---|
| `--dur-fast` | 120ms |
| `--dur-base` | 200ms |
| `--dur-slow` | 420ms |
| `--ease-out` | `cubic-bezier(.22,1,.36,1)` |

Only `transform`, `opacity` and `background-color` animate. No layout property
is animated anywhere.

**Calm is the default.** `<html data-motion="calm">` ships in `app.html`, so the
first paint is already still; `mode.svelte.ts` only upgrades it to `full` when
the visitor is in Jelajah *and* has not asked for reduced motion. A visitor with
JS blocked, or a crawler, gets the calm version: all content visible, zero
animation.

---

## 5. Layout

- Max width 1920px, gutter `clamp(20px, 3.4vw, 72px)`.
- Fluid `flex-wrap` columns with `flex: 1 1 <basis>` and `min-width: 0`; no
  breakpoint-heavy grids.
- Every section uses a different composition: split hero, marquee band, 3-card
  row, 4-card bento, copy-left/tiles-right, full-bleed CTA panel.
- Phone header is exactly three rows (brand / nav / tools) and the nav stays on
  **one** line down to 360px.

---

## 6. Accessibility contract

- Every route: one `<h1>`, ordered headings, landmarks, `lang="id"`.
- Skip link is the first tab stop on every page.
- Tap targets: measured minimum 44×44px; the audit reports zero below that.
- Status is never colour alone: every verdict pairs an icon, a text label and a
  written explanation.
- Tabs use `role="tablist"` / `role="tab"` / `aria-selected` with roving
  tabindex and arrow keys.
- The first-visit mode picker is **inline, not modal**: it sits after the hero,
  does not trap Tab and does not steal focus on load.
- Esensial mode: single column, +12.5% type, all motion off, identical content.

---

## 7. Honest limits

- The hero scanner is a **worked example**, labelled "Contoh", not a live scan.
  It makes no network call.
- Demo dossiers carry a visible "CONTOH / SIMULASI" banner.
- The score is a risk score, never a confidence score, and never rendered with
  a percent sign.
- "Mode tenang" and "Bacakan hasil" are real (motion off, `speechSynthesis`).
  "Kumpulkan lencananya" is illustrative only; there is no badge persistence,
  by design (no accounts, no storage).
