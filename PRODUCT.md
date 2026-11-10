# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Indonesian families, above all parents and older relatives, who receive videos, images and links through WhatsApp groups and social media and want to know whether to believe or forward them. They use AMBA mostly on a phone, often right after the message arrives. Competition judges (MIT-Week 2026) also evaluate it, mostly on laptops. (Confirmed by Sir, 10 Nov 2026.)

## Product Purpose

AMBA (Aplikasi Mengenal & Bertindak Aman) checks one link (TikTok, YouTube, Instagram, Facebook, X, or a news page) or one image. It looks for AI generation, editing, a claim that fact-checkers have already debunked, and dangerous links. It returns one clear conclusion (Aman, Perlu diwaspadai, or Terindikasi hoaks/manipulasi) with plain-language reasons. The Belajar page teaches people to spot fakes themselves, ending in an 8-question quiz. Success means a family member reaches a confident decision in under a minute and understands why.

## Positioning

One input for both links and images. AMBA reads the actual video (frames and audio) or image, and searches Indonesian fact-checkers (TurnBackHoax and others). Every check ends in a conclusion; no "cannot be checked" dead ends. Explanations are in plain Indonesian, with read-aloud. No account is needed.

## Operating Context

The link is pasted from WhatsApp or a social app. The result is often read aloud or shown to someone else in the room. The interface is in Indonesian only (Sir, 10 Nov 2026). Results are indicative, not legal proof; the footer and the about page say so.

## Capabilities and Constraints

- SvelteKit single-page app, built statically and served by nginx behind a Cloudflare tunnel.
- Strict Content Security Policy: fonts and scripts are self-hosted, and no third-party stylesheets are allowed.
- A check takes roughly 10 to 60 seconds. Progress streams over SSE.
- Uploads are limited to 10 MB and 16 MP.
- Deadline: 10 Nov 2026, 23:59 WITA.

## Brand Commitments

- The name AMBA and the line "Cek dulu sebelum percaya." are kept.
- Visual world, pinned by Sir on 10 Nov 2026 (16:31 WITA): the theme and style of the Gemini web app (see DESIGN.md), without its logo or name.
  - Dark is the default theme; light is one toggle away.
  - Every page works on phones, tablets and laptops.
- The look must be professional and free of AI-template patterns.

## Evidence on Hand

- Bundled demo cases in `frontend/static/demo`.
- Quiz images in `frontend/static/edukasi`.
- The live site at https://amba.madewadigital.com.
- There are no testimonials, user counts, partner logos or press, and none may be invented.

## Product Principles

- Always conclude. The result names a verdict and its reasons, never a failure.
- Plain words for families: no jargon, large readable type, and read-aloud.
- Show the user's own content. The thing being checked appears beside its verdict.
- Be honest about limits. Scores are risk, not certainty, and results are indicative.

## Accessibility & Inclusion

- WCAG 2.1 AA contrast in both themes: body text at least 4.5:1.
- Full keyboard use, visible focus, and respect for reduced motion.
- Google Sans Flex at 17px body size is the reading face, roomy for older readers.
