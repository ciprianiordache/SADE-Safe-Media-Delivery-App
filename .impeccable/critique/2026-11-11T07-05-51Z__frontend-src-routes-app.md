---
target: aplicatia vs pozele din pagina de prezentare
total_score: 28
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
target_identity: "file:C:\\Users\\ciipr\\Desktop\\sade\\frontend\\src\\routes\\app"
timestamp: 2026-11-11T07-05-51Z
slug: frontend-src-routes-app
---
⚠️ DEGRADED: single-context (session policy: sub-agents only on explicit user request)

Target: the operator app + login + recipient page, compared with the marketing page mockups.

| # | Heuristic | Score | Key Issue |
|---|---|---|---|
| 1 | Visibility of System Status | 3 | status tags + toasts clear; Stripe form loads with no placeholder |
| 2 | Match System / Real World | 3 | rows show emails where the mockup shows client names |
| 3 | User Control and Freedom | 3 | row click leaves the list; no in-place detail |
| 4 | Consistency and Standards | 3 | fake window dots on real panels; LogoPicker keeps old dashed/text-link style |
| 5 | Error Prevention | 3 | typo guard, two-step delete and links-off |
| 6 | Recognition Rather Than Recall | 3 | filters with counts visible |
| 7 | Flexibility and Efficiency | 2 | mockup shows "/" to search; no shortcuts |
| 8 | Aesthetic and Minimalist Design | 3 | sparse list window with one row; sort select stranded |
| 9 | Error Recovery | 3 | retry states exist |
| 10 | Help and Documentation | 2 | no contextual help beyond hints |
| Total | | 28/40 | Good |

Priority issues:
- [P1] Dashboard is not the mockup's composition: the mockup is one window, list on the left and the open delivery (photo, pills, timeline) on the right. The app shows list + upload form; opening a delivery leaves the page. Fix: master-detail at >= 1280px (selected row opens the delivery panel in place, ?job=id in the URL), upload behind "Fișier nou" as a panel/drawer; keep the job page for phones and deep links. Command: /impeccable layout.
- [P1] Recipient pay column does not match the hero mockup: Stripe accordion with four methods (Card, Bancontact, MB WAY, EPS) and "Plătește — 49,00 EUR", no file size. Mockup: "Originalul fără filigran, 24 MB", one card field, "Plătește 49 €", then "Plată primită" + ink "Descarcă originalul". Fix: Payment Element layout "tabs"/card-first, payment methods restricted to card in test, title with the original's size, compact price. Command: /impeccable distill.
- [P2] Brand settings is a long form; the mockup promises a short toggle list ("Filigranul contului, 3 din 5 pornite": logo, company, recipient address, date, whole picture). Fix: lead with the toggle rows, expand position/opacity/size under the logo row. Command: /impeccable layout.
- [P2] Sparse list window: min-height 420px with one row, the sort select pinned to the sidebar bottom. Fix: drop the min-height, move sort into the window bar next to search, show a filled empty state. Command: /impeccable layout.
- [P2] Window dots (three grey circles) on real panels: in the mockup they say "this is a screenshot of an app"; inside the app they are fake chrome that looks clickable. Fix: remove them from app windows, keep the bar + breadcrumb. Command: /impeccable quieter.

Personas:
- Alex (power user): no "/" for search though the mockup shows it; each delivery needs a page change and back.
- Sam (a11y): fine overall; the decorative heading on login is aria-hidden; the window dots add nothing for SR but are harmless.
- Recipient on a phone: the Stripe accordion pushes the pay button far down; the clean original's size is unknown.

Minor: no activity feed like the mockup's "Andreea M. a plătit... Vezi" (only toasts); login code is one input behind a disclosure, mockup shows 6 boxes; rows show emails, mockup shows names (no name data in the API); LogoPicker still has the old dashed zone and text-link actions.

Detector: 0 findings on frontend/src/routes + lib/components. Browser overlay blocked by the app's CSP (script-src 'self'), no overlay shown.
