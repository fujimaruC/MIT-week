---
version: 1
slug: "frontend-src-routes"
primary_target: "frontend/src/routes"
related_targets: []
---

Scope: the whole AMBA web app (home /, /deteksi, /hasil, /edukasi, /tentang, the error page). Mode: Operate for /deteksi and /hasil, Read for /edukasi and /tentang, Persuade for /. Audience: Indonesian families on phones; judges on laptops. Constraints: SvelteKit SPA, strict CSP (self-hosted fonts), WCAG AA in both themes, default theme follows the device. Direction pinned by Sir (10 Nov 2026): glassmorphism with a simple Claude-like palette. No concept-seed roll was run, because the direction is pinned.

## Direction contract

THESIS: Glass is the inspection instrument. What a family wants to verify sits behind frosted glass, and AMBA's verdict is what clears it. The page refuses the category default: a dark neon hero, three numbered step cards, and a closing CTA band.

OWN-WORLD:
- Claude-like neutrals: ivory #F5F4EF over charcoal ink #1F1E1C, inverted in dark mode.
- One terracotta accent: #B4532F light, #D97757 dark. It appears only on the primary action, focus, and the risk meter.
- Frosted panels: 20px blur, a 1px inner highlight, and a soft shadow lit from above.
- Ambient fields of terracotta and slate exist only where glass sits over them.
- Type: Bricolage Grotesque at 600 for display, Atkinson Hyperlegible for text.

STORY: A parent pastes a forwarded link, watches AMBA check the real video, reads one clear conclusion and its reasons, then learns how to spot the next one.

FIRST VIEWPORT: Desktop, 7/12 left: the headline "Cek dulu sebelum percaya.", one sentence on what is checked, and the glass checker (link field, Periksa, an image-upload link). 5/12 right: a real forwarded hoax image from the demo set, seen through a glass lens that carries its verdict chip, labelled Contoh. Mobile stacks the headline, the checker, then the example.

FORM: pinned glassmorphism. Signature move: the frost clears on the result page. The verdict pane is glass over the user's own image or video frame, heavily frosted while AMBA checks and clearing when the verdict lands. Seed key: none (pinned brief).

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
