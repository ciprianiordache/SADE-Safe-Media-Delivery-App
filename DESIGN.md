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

## Origin (2026-11-11)

The app follows the marketing page at `frontend/static/index.html` (served at `/`):
Reddit Sans + JetBrains Mono, warm paper, ink #211c18, one orange action with **dark text on
it**, pill buttons, 72px header (logo + "SADE" set at 22px/500), 1280px content width with 24px
gutters, headings at weight 500 with -0.025em tracking, sentence-case labels (no uppercase).
Numbers, sizes, file names and addresses use the text face with tabular figures (`.mono`);
real monospace (`.code-mono`) only for code-like strings (sign-in code, templates).

### The product window (2026-11-11, second pass)

Every app page now uses the marketing page's own mockups as its pattern, not just its fonts:

- **Page head** (`.page-head`): a large h1 (clamp 36-54px, -0.035em) with a muted lead, a
  status chip or tabs on the right. Pages pad 48px top / 96px bottom (`.page`).
- **Window** (`.win` + `.win-bar` + `.win-dots`): a 12px white frame on `--shadow-card`, a 44px
  bar on `--fill` with a hairline, a breadcrumb ("Livrări / Toate"). The dashboard list, the
  upload panel, the job page, the brand form and preview, and the recipient's viewer are windows.
- **Rows, not boxes**: lists and key/value details are hairline rows (`--line`), never cards
  inside cards. History is the mockup's timeline: dot, text, time on the right; paid and
  original-downloaded get the orange dot.
- **Status tag** (`.st`, `.badge`): 24px, 6px corners, a hairline; one tag per delivery from
  `lib/delivery.ts` (`În coadă`, `Filigran` with a spinner - the one accent, `Trimis`,
  `Deschis` ink, `Plătit` green, `Eșuat`/`Oprit` red). The round `.chip` is for the page head and
  panel heads ("Linkuri active").
- **Initials** (`.avatar` + `data-tone` 1-5, `lib/delivery.ts` `initials`/`tone`) for recipients
  and the account.
- **Buttons**: `.btn` (orange, dark text), `.btn-ink` (dark pill: "Descarcă originalul", passkey
  sign-in, banners), `.btn-line` (outlined pill: "Copiază linkul", "Retrimite"), `.btn-ghost`
  (tonal), `.btn-sm` (36px drawn, 44px tapped).
- **Controls**: segmented controls and RO|EN / theme pills are a tonal track with the chosen
  option lifted on white (`.segmented`, `.pill-group`); booleans are the ink `.switch`.
- **Header** (`/app`): logo left, "Livrări" / "Setări" centred with an orange underline on the
  current one, "Fișier nou" + the account on the right; language and theme live in the account
  menu. Public pages use `MarkBar` (same bar, "Despre SADE" link + the toggles).
- **Split view** (dashboard, >= 1024px; secondary nudges always come after it): the list window and the open delivery in one frame,
  each pane scrolling; the right pane is either the selected delivery (`JobPanel`) or the upload
  form. The selected row is a darker fill, never a coloured side stripe.
- **Toggle rows** (brand settings): title over its current value, an ink `.switch` on the right,
  "N din 5 pornite" in the window bar. A row that must stay on (the client's address) shows a
  lock and "Mereu", not a disabled switch. A row's own input sits right under it.
- **Sign-in pages** (`AuthShell`): the marketing page's split - the statement and two hairline
  points on the left, the form as a white card on the warm panel on the right (card first on a
  phone).

### The installed app (2026-11-11)

Inside the Tauri window (`html.is-app`) the app behaves as a desktop program: title bar with the
system's own window controls, no inner window dots, no UI text selection or browser context menu,
thin scrollbars, bundled fonts (works offline), a page's name lives only in the title bar, panes
close with x, Ctrl/Cmd+N, Ctrl/Cmd+, and Ctrl/Cmd+F as shortcuts, rows have a right-click menu,
caption buttons are not Tab stops, preferences save on their own (a quiet "Salvat"), and a page
reads as one window surface (no cards floating on a tinted ground; columns scroll on their own).
One filled action per group (the ink button); rare or risky actions (links off, delete) live in a
"⋯" menu (`OverflowMenu`) and confirm in place; a status is a dot and a word, never a pill that
looks pressable.

## Color tokens

| Token | Light | Dark | Use |
|---|---|---|---|
| `--bg` | #fbf8f4 | #1b1714 | page background |
| `--surface` / `--surface-2` | #ffffff / #f3ede4 | #26211c / #2e2924 | cards, inputs, menus / tonal fills |
| `--text` | #211c18 | #fbf8f4 | body text, headings |
| `--text-muted` | #544b42 | #bcb3a8 | secondary text **and every note that informs** (>= 7:1 on `--bg`) |
| `--text-faint` | #8a8074 | #8f857a | decoration only, never information |
| `--text-hint` | #b4a997 | #6f665c | icons and strokes only, never text |
| `--border` / `--border-strong` | #e7e0d6 / #d6cdc0 | #39322b / #4a4239 | dividers / control outlines |
| `--accent` (+ `-hover`) | #e8631c (#f07a3a) | same | primary buttons and selected controls; text on it is `--accent-text` #111 (5.6:1, hover 6.8:1) |
| `--accent-ink` (+ `-hover`) | #b8481a | #f0834a | links and accent-coloured text on `--bg`/`--surface` (>= 4.5:1) |
| `--accent-bright` | #e8631c | #e8631c | fills with no text, and the focus ring (3.2:1 on `--bg`) |
| `--accent-tint` / `--accent-tint-text` | #fbe3d3 / #8a3410 | #3d2a1a / #f2965e | selected chips, inline links on tinted ground |
| `--danger`, `--success`, `--warning`, `--pending` (+ `-tint`) | | | status only (job states, errors, destructive actions) |
| `--shadow-card` / `--shadow-sm` | | | windows and cards / lifted controls, as on the marketing page's UI mocks |
| `--line` / `--line-2` | ink 9% / 15% | same mix | hairlines between rows / field and tag outlines |
| `--fill` / `--fill-2` | ink 3.5% / 7% on surface | same mix | window bars, footers / tonal tracks, hover |
| `--ink-btn` / `--ink-btn-text` | #211c18 / #fbf8f4 | inverted | the dark pill and the on-state of `.switch` |

Rules:
- One accent color. Status colors are for status, never decoration.
- Contrast: body and informative text >= 4.5:1. That is why notes use `--text-muted`, not
  `--text-faint` (design review 2026-11-07, decision 15A). The `.faint` class is no longer used
  in markup; `--text-faint` remains for disabled controls and the Stripe placeholder only.
- Text on an orange button is dark (`--accent-text` #111), as on the marketing page; white never sits on orange.
- No coloured `border-left`/`border-right` stripes on cards, rows or banners; status lives in
  `StatusBadge`.

## Type

- `--font-sans`: Reddit Sans (UI and body). `--font-mono`: JetBrains Mono (templates, sign-in
  codes). Loaded from Google Fonts in `app.html`.
- Page title `h1` clamp(2.25rem .. 3.4rem), 500, -0.035em; panel headings (`.section-label`)
  0.95rem, 500, ink; body 0.9-0.95rem; hints 0.82-0.88rem.
- More space above a heading than below it.

## Shape and spacing

- `--radius` 12px (windows, cards), `--radius-sm` 8px (inputs, small controls); buttons and
  segmented controls are pills.
- Page gutter 24px, 16px under 640px. Window bodies pad 20px.
- Touch targets >= 44px for anything tappable on a phone (text buttons included). `.btn` has
  `min-height: 44px`; compact controls (RO|EN and theme pills, inline links, template chips) keep
  their drawn size and get the rest from an `::after` hit area.

## Components (frontend/src)

| Component / class | What it is |
|---|---|
| `.btn`, `.btn-ghost` | primary (accent) and secondary buttons |
| `.section-label` | panel heading; defined once in `app.css` (ink, 500) |
| `.card` | one white surface per job of the page; not nested |
| `.link-btn` / `.text-btn` / `.inline-link` | text-only actions, still >= 44px tall |
| `lib/toast.svelte.ts` + `Toasts.svelte` | confirmations and recoverable errors |
| `.st` via `lib/delivery.ts` | one status tag per delivery, the only place status colors appear in lists |
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
