---
target: aplicația desktop
total_score: 26
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
target_identity: "file:C:\\Users\\ciipr\\Desktop\\sade\\frontend\\src-tauri"
timestamp: 2026-11-11T17-56-25Z
slug: frontend-src-tauri
---
Method: dual-agent (A design review, then B detector; sequential, one app window)

Scores: 1:3 2:3 3:2 4:2 5:3 6:3 7:2 8:3 9:3 10:2 = 26/40 (Good, low)

Specificity: product-specific pieces strong (burned-in watermark, "Mereu" lock, folded history, Cu filigran/Original); frame generic SaaS; payment nearly absent operator-side.
Detector: source 0; live: file page flat-type-hierarchy (body 14.4, h3 15.2, h2 18); dark: account avatar initials 2.6:1 (app.css:656-687 .avatar tones). False positives: cream-palette, nested-cards (PositionPicker/BrandPreview frame).

Priority issues:
- P1 Payment invisible to operator (no price / unpaid / paid-on in DeliveryCard or Details). Fix: "Plată" row; delivery API to return price+currency.
- P1 Closing pane leaves ?job= (syncUrl compares page.url.search, not updated by replaceState); F5 reopens closed pane. Fix: compare location.search.
- P2 JobPanel .panel aria-live=polite: polling re-announces whole pane. Fix: drop, announce status only.
- P2 Flat action hierarchy, destructive loud: Copy/Resend/Stop links equal pills; red Șterge loudest in file head; status chip looks like a button; heading steps; dark avatar contrast. Fix: Copy as btn-ink, overflow "⋯" for stop/delete, dot+text status.
- P3 Settings still web cards on cream; Security split layout. Fix: flat groups, single column (Windows 11 Settings).

Personas: Alex - arrows only >=1024, no Enter/Delete/Ctrl+C, ctx menu lacks resend/links, no multi-select, no shortcut sheet, no sortable headers. Sam - aria-live pane, 800px overlay not inert, list unnamed.

Minor: exemplu.com vs exemplu.ro; "Fără logo" as label; orange slider vs black toggles; settings scrollbar; empty dashboard space; mixed facts in link note; "Deconectează" on this device without confirm.
