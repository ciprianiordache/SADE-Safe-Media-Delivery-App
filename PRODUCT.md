# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

**Operators** (the paying customer, signed in): anyone who delivers media files to their own
clients and wants control over the work until it is paid for. Confirmed audiences, none ranked
above the others:

- photo and video studios, including event work (weddings, baptisms, corporate events);
- media freelancers in general: photographers, videographers, audio producers, designers;
- agencies and production companies sending drafts and finals to business clients.

Their job: upload a finished file, have it watermarked with their own brand and the recipient's
identity, send it, see whether it was opened and paid, and resend or switch the links off when
needed. They work from a desktop most of the time but also check status from a phone.

**Recipients** (no account, no login): the operator's clients. They arrive from an email link on
whatever device they read mail on, often a phone, look at the watermarked preview, and may pay to
download the clean original. They did not choose SADE and owe it no trust; the page has to be
clear and credible on first sight.

## Product Purpose

SADE (Safe Media Delivery) watermarks a video, audio or image file and delivers it to a recipient
through a signed link. The recipient sees the watermarked preview and can pay (Stripe) to unlock
the clean original. Success for the operator: the client sees the work, the operator knows what
happened to it, a leaked copy is traceable, and the operator is paid before the clean file leaves.

Protection, traceability and payment are one package; none of the three is secondary.

## Positioning

The watermark is not generic: every copy carries the operator's brand (logo, company name, chosen
position and opacity) **and the recipient's own address and the date**, frozen at upload. A leaked
preview points to the person it was sent to. The recipient needs no account, and the unlock is a
payment inside the operator's delivery page, not a redirect to a third-party checkout.

## Operating Context

- Operator flow: magic-link sign-in (browser-bound, 6-digit code for other devices, optional
  passkeys) -> first-run brand setup (company, logo, position) -> upload with recipient email ->
  async watermarking -> recipient email -> dashboard signals (opened, paid, links off) -> job page
  with copyable recipient link, resend, links on/off, delivery history.
- Recipient flow: email ("Your preview is ready") -> `/preview/<token>` page with the player and,
  when payments are on, an embedded Stripe Payment Element -> download of the original.
- Media: video, audio (audible watermark), images. Uploads up to 2 GiB; daily job and storage
  quotas per account.
- Self-serve signup, open by default (allowlist/closed modes exist).

## Capabilities and Constraints

- Own SaaS, Romanian market first. Romanian is the default locale, English the second; every UI
  string exists in both.
- The recipient page and the preview email stay unbranded by the operator for now (company name and
  logo appear only inside the watermark). Reason: with open signup, a sender could impersonate a bank
  and send a card form. Gate operator branding on those surfaces on verified or paid senders first.
- Payment is optional per deployment (no Stripe keys = unlock section hidden). One fixed global
  price today; no per-job price, no operator plans or billing yet.
- Card data never touches SADE; Stripe Elements handles it inside SADE's own page.
- Undecided: operator pricing/plans, per-job prices, verified-sender program, S3 storage.

## Brand Commitments

- Name: **SADE**, expanded as "Safe Media Delivery". Tagline in use: "Livrare sigură de fișiere
  media, cu filigran" / "Safe media delivery, watermarked".
- Logo: `docs/icon150.png` (also `frontend/src/lib/assets/logo.png`, embedded in emails).
- Voice: plain, short, direct; Romanian first. The operator's client file is the loudest thing on
  screen; SADE itself stays quiet.

## Evidence on Hand

- Screenshots from the seeded demo: `docs/dashboard.png`, `docs/job.png`, `docs/preview.png`,
  `docs/unlocked.png`. Demo video `docs/test.mp4`.
- No customers, testimonials, case studies, usage numbers or press exist yet. Do not invent any.

## Product Principles

1. The client's work comes first; SADE's interface stays out of its way.
2. Every delivered copy is traceable to its recipient; never trade that away for looks or length.
3. The recipient owes SADE nothing: their page must be immediately clear and trustworthy, with no
   account and no surprise.
4. Never show a state that may be false: loading, error and empty are designed, not skipped.
5. Safety over reach: features that could help impersonation wait for sender verification.

## Accessibility & Inclusion

Text that informs meets 4.5:1 contrast in both light and dark themes; touch targets are at least
44px; real form controls with keyboard support; focus moves to the new heading on step changes;
`prefers-reduced-motion` is respected. Recipients may be on any phone, so the preview page works at
375px.
