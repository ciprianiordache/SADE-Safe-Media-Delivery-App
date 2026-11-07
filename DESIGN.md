# SADE design system

What the app already uses, written down so pages and reviews follow one set of rules.
Source of truth for values: `frontend/src/app.css` (CSS custom properties on `:root`,
redefined for dark under `:root[data-theme='dark']` and `prefers-color-scheme: dark`).
If a value here and `app.css` disagree, `app.css` wins and this file gets fixed.

## Character

Warm, calm, one accent. Operators use SADE to send work to their own clients, so the
interface stays quiet and the client's file is the loudest thing on screen. No gradients
on UI surfaces, no decorative illustration, no card mosaics. Light and dark themes are
both first-class; the toggle (RO|EN + light|dark) sits next to the mark on every page.

## Color tokens

| Token | Light | Dark | Use |
|---|---|---|---|
| `--bg` | #fbf8f4 | #17130f | page background |
| `--surface` | #ffffff | #221d18 | cards, inputs, menus |
| `--text` | #211c18 | #f3ede3 | body text, headings |
| `--text-muted` | #6e655b | #b8ac9c | secondary text **and every note that informs** (hints, limits, warnings, "step n of 3") |
| `--text-faint` | #9c9287 | #83786a | decoration only: dates in lists, separators, placeholders. Never for information (2.9:1 on white) |
| `--text-hint` | #b4a997 | #6b6255 | icons and strokes only, never text |
| `--border` / `--border-strong` | #ebe3d7 / #ddd3c4 | #322c25 / #3a342c | dividers / control outlines |
| `--accent` (+ `-hover`) | #e8631c | #e8631c | the one accent: primary buttons, selected state, progress |
| `--accent-tint` / `--accent-tint-text` | #fbeadb / #a8480f | #3d2a1a / #f2965e | selected chips, inline links on tinted ground |
| `--danger`, `--success`, `--warning`, `--pending` (+ `-tint`) | | | status only (job states, errors, destructive actions) |

Rules:
- One accent color. Status colors are for status, never decoration.
- Contrast: body and informative text >= 4.5:1. That is why notes use `--text-muted`, not
  `--text-faint` (design review 2026-11-07, decision 15A).

## Type

- `--font-sans`: IBM Plex Sans (UI and body). `--font-mono`: IBM Plex Mono (emails,
  percentages, templates, ids). Loaded from Google Fonts in `app.html`.
- Page title `h1` 1.45-1.6rem bold; card section labels 0.72rem, 600, uppercase, letter-spaced,
  `--text-muted`; body 0.88-0.9rem; hints 0.75-0.8rem.
- More space above a heading than below it.

## Shape and spacing

- `--radius` 11px (cards, buttons), `--radius-sm` 8px (inputs, chips, small controls).
- Page gutter 1.75rem desktop, 1rem at <= 520px. Cards pad ~1.25rem.
- Touch targets >= 44px for anything tappable on a phone (text buttons included).

## Components (frontend/src)

| Component / class | What it is |
|---|---|
| `.btn`, `.btn-ghost` | primary (accent) and secondary buttons |
| `.card` | one white surface per job of the page; not nested |
| `.link-btn` / `.text-btn` / `.inline-link` | text-only actions, still >= 44px tall |
| `lib/toast.svelte.ts` + `Toasts.svelte` | confirmations and recoverable errors |
| `StatusBadge`, `StatusDot` | job status, the only place status colors appear in lists |
| `PasskeyBanner`, `BrandBanner` | dismissable nudges on the dashboard (localStorage flag) |
| `BrandPreview` | the watermark on a sample picture; mirrors the server's text fit (`lib/format.ts` <-> `internals/ffmpeg/textfit.go`) |
| `LogoPicker` | upload/replace/remove the account logo; remove asks twice; saves immediately |
| `PositionPicker` | spatial frame of real radio buttons + a separate "whole picture" choice |
| skeleton blocks (`.sk`) | loading placeholders shaped like the content |

## States every data view needs

loading (skeleton, `aria-busy`), empty (what to do next), error (plain message + "Reîncearcă";
never an endless spinner), success (toast or visible result), partial. A form never shows a
message that may be false while data is still loading (decision 4A).

## Accessibility

- Real form controls (radio inputs, buttons, labels); custom looks go on top of them.
- After a step or view change, focus moves to its heading (`tabindex="-1"`).
- Errors use `role="alert"`; busy areas use `aria-busy`.
- Pictures that carry meaning get a text equivalent (e.g. the preview's visually hidden summary).

## Responsive

- <= 860px: two-column layouts stack; anything you edit "live" keeps its preview in view
  (sticky, compact), and a Save that matters becomes a sticky bar while there are changes.
- <= 520px: tighter gutters; grids drop to one column.
