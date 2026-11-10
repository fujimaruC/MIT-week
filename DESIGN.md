---
name: AMBA
description: Cek dulu sebelum percaya. A content checker for Indonesian families, in the style of the Gemini web app with Claude's colours, Indonesian only.
---

# Design System: AMBA

## Overview

On 10 Nov 2026, Sir asked for:
- the theme and style of the Google Gemini web app;
- pages that work on every device;
- an interface in Indonesian only.

Checking a forwarded video, image or link reads like a conversation: the person sends the item from one round prompt bar, AMBA "thinks" while its checks run, and the verdict comes back as an answer with the evidence under it.

The shapes, type and layout were measured on gemini.google.com that day; the captures are in `OpenBot/Shared/amba-v2/gemini/ref/`. Later the same day Sir changed the colours to Claude's and kept the Gemini style ("warna nya diganti ke khas dari claude aja bukan gemini tapi style nya tetap dari gemini"), so the colours below were measured on claude.com. Neither product's logo or name is used: AMBA keeps its own shield mark on the three-colour gradient.

Dark is the default theme. Light is a one-click option (Sir's choice, 10 Nov 2026).

## Colours

| Role | Dark (default) | Light |
|---|---|---|
| Page | #141413 | #faf9f5 |
| Dashboard frame (side bar, top bar) | #1a1918 | #f0eee6 |
| Work sheet | #141413 | #ffffff |
| Prompt pill | #262624 | #ffffff |
| Cards | #1f1e1d | #f5f4ed |
| Raised, hover | #262624 / #30302e | #f0eee6 |
| Text | #faf9f5 | #141413 |
| Secondary text | #c2c0b6 / #9c9a92 | #3d3d3a / #5e5d59 |
| Borders | #3d3d3a / #30302e | #d9d6cc / #e8e6dc |
| Primary pill | #d97757, text #141413 | #b4532f, text #ffffff |
| Links, active | #e8957a | #a8492a |
| Selected item | #3b2a22 | #f3e3da |
| Risk-map bar under its line | #87867f | #87867f |

Claude's own button is #c6613f with white text. White on #c6613f is under 4.5:1, so the light theme uses the deeper #b4532f. The dark theme uses the logo orange #d97757 with dark text.

The gradient runs #c6613f → #d97757 → #e8b088 on dark and #a8492a → #c6613f → #b9772f on light. It is used for the greeting, the AMBA mark, the prompt's focus ring, the "thinking" ring and the landing page's wave.

A soft clay glow sits behind the home greeting in both themes.

The verdict roles are Material tones, never the accent. Each has a pale fill for its chip.

| Verdict | Dark | Light |
|---|---|---|
| Safe | #6dd58c | #146c2e |
| Caution | #fdd663 | #7a5900 |
| Danger | #f28b82 | #b3261e |

`tools/check-contrast.mjs` checks 106 pairs in both themes, and all pass AA.

## Type

Google Sans Flex (OFL), self-hosted:
- the weight axis only, 51 KB Latin plus 26 KB Latin Extended;
- `font-display: optional`, with a preload.

Large text is light. The greeting is set at weight 400–500 and the verdict sentence at weight 400. Body text is 17px, the prompt's size, roomy for older readers.

## Shape and depth

| Element | Radius |
|---|---|
| Buttons and chips | pill (999px) |
| Prompt bar | 32px |
| Cards | 24px |
| Small panes | 12–16px |

Surfaces are tonal and have no borders. The only shadow is the prompt's soft lift.

## Layout

Simplified for parents on 10 Nov 2026 (Sir, seq 926 and 933): few pages, few buttons, as little text as possible, and diagrams and real pictures instead of paragraphs. There are no eyebrow badges, no lead paragraphs, and no section repeats what another says.

- **Shell, every page and every width:**
  - One top bar with the mark, two destinations (Periksa, Belajar), each an icon with a word, and the theme toggle.
  - There is no sidebar and no hidden menu.
  - The footer is one line: results are indicative, with a link to Tentang, which holds the full legal text.
- **Home:**
  - The headline in two lines, then one sentence.
  - The link pill, with "atau Pilih foto" under it as a labelled button, and the platforms' marks (Simple Icons, CC0).
  - The flow diagram: what you send, then the four checks, then the three verdicts.
  - An example result built from the result page's own components with bundled demo data, labelled as a simulation, and the three example chips.
  - A Belajar card with a real image and its ELA map.
- **/deteksi:**
  - The title, then the link pill with "Pilih foto". The whole panel also takes a dropped image.
  - The example chips show while idle. The four-check strip appears only once a check starts.
  - The session list appears only when it has entries.
- **/hasil, top to bottom:**
  - the item beside the verdict (the h1), one sentence, the score and its meter, up to four reasons, then Bacakan hasil and Periksa lagi;
  - the risk map;
  - the video frames or the ELA viewer;
  - the written detail, folded under "Lihat rincian lengkap".
- **/edukasi:** each sign is one line that opens on tap. The photo practice, the quiz and the family password follow.
- **Images from any page:** a picked image is handed to /deteksi in memory and checked at once.

## Motion

- Entrances rise 12px and fade in, staggered.
- The home glow breathes slowly.
- A dot travels along the flow diagram's arrows.
- The score bar and the risk-map bars fill once they are in view.
- Page changes cross-fade through View Transitions, and the prompt bar glides between pages.
- Reduced motion shows every final state at once.

## Language

Indonesian only. `t()` reads one dictionary, `src/lib/i18n/id.ts`. The server's Indonesian sentences are shown as written.
