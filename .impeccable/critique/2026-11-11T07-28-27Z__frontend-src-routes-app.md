---
target: aplicatia dupa remedieri
total_score: 29
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 0
target_identity: "file:C:\\Users\\ciipr\\Desktop\\sade\\frontend\\src\\routes\\app"
timestamp: 2026-11-11T07-28-27Z
slug: frontend-src-routes-app
---
⚠️ DEGRADED: single-context (session policy: sub-agents only on explicit user request)

Target: the operator app + recipient page vs the marketing page pictures, after the split-view, unlock and toggle-list fixes.

| # | Heuristic | Score | Key Issue |
|---|---|---|---|
| 1 | Visibility of System Status | 3 | Stripe now has a skeleton; status tags clear |
| 2 | Match System / Real World | 3 | rows show emails, the mockup shows names |
| 3 | User Control and Freedom | 3 | selection kept in ?job=, Cancel on the upload pane |
| 4 | Consistency and Standards | 3 | "whole picture" exists twice on the brand page |
| 5 | Error Prevention | 3 | client address cannot be switched off |
| 6 | Recognition Rather Than Recall | 3 | filters with counts, open row marked |
| 7 | Flexibility and Efficiency | 3 | "/" focuses search; no arrow-key row selection |
| 8 | Aesthetic and Minimalist Design | 3 | dead space under the picture in the open delivery |
| 9 | Error Recovery | 3 | "Arată toate livrările" on no matches |
| 10 | Help and Documentation | 2 | hints only |
| Total | | 29/40 | Good |

Priority issues:
- [P2] Double scroll in the split view: the window starts ~330px down (page head + passkey banner) and is ~780px tall, so at 1440x900 the page scrolls and both panes scroll. Fix: on wide screens shrink the page head (title + lead on one row, banner below the window or inside the bar) and size the window to the space left under the header. /impeccable layout
- [P2] The open delivery leaves a hole: the picture column ends early, "Pagina fișierului" floats alone under it after the history grows. Fix: move "Pagina fișierului" to the panel head next to the chip; let the picture keep its column and the history follow it. /impeccable layout
- [P2] Brand page repeats itself: "Pe toată imaginea" is a toggle row and a PositionPicker option; the logo row and the LogoPicker; the company row and the company field sit far apart. Fix: drop the tiled option from the picker on this page, put the company field and the logo picker right under their rows. /impeccable distill
- [P3] The locked client-address switch is grey and reads as broken. Fix: replace it with a lock icon + "Mereu" label. /impeccable clarify
- [P3] The no-logo row says "Fără logo" twice (title and note). Fix: sub "Fișierele primesc doar text". /impeccable clarify

Personas: Alex - no arrow keys to move through rows in the split view. Sam - fine; the locked switch is announced as a disabled checked switch, acceptable. Recipient on a phone - existing demo intent still lists 4 methods until a new intent (data, not design).

Minor: rows show emails (no names in the API); the upload pane form stops at 520px, leaving the right third empty; the job page and the open delivery now differ (page has details + delete).

Detector: 0 findings on frontend/src/routes + lib/components. Browser overlay: not available, the app's CSP (script-src 'self' + Stripe + hashes) blocks the injected detector, confirmed in the previous run and the header is unchanged.
