---
target: aplicația desktop
total_score: 26
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
target_identity: "file:C:\\Users\\ciipr\\Desktop\\sade\\frontend\\src-tauri"
timestamp: 2026-11-11T17-36-04Z
slug: frontend-src-tauri
---
Method: dual-agent (A design review, then B detector; sequential, one app window)

Scores: 1:3 2:3 3:3 4:2 5:3 6:3 7:2 8:3 9:2 10:2 = 26/40 (Good, low)

Specificity: dashboard is authored (burned-in watermark preview, delivery signals, OS-adaptive chrome); file page and Security still read as a web page in a window.
Detector: source 0; live: job page no h1 + flat h2/h3 (15.2 vs 14.4 body); Brand h1->h3 skip; list column headers 3.9:1 (+page.svelte:794); "IP." straggler in Security. False positives: cream-palette (brand paper), nested-cards on PositionPicker .frame.

Priority issues:
- P1 Security labels this app "Edge pe Windows" (WebView2 UA). Fix: record X-SADE-Client on the session, label "Aplicația SADE pe Windows".
- P1 File page is a bordered/shadowed web card, whole document scrolls, no h1. Fix: flush surface in app, two independently scrolling columns, visually-hidden h1, heading scale.
- P2 Delivery history repeats identical "opened" events; status chips duplicated (row, pane head, delivery). Fix: collapse runs ("Deschis de 5 ori"), drop pane-head chip.
- P2 Brand settings mix instant logo save with an off-screen Save button; disabled logo-size hint in accent colour; h1->h3. Fix: autosave with "Salvat" tick, muted hint, h2.
- P3 Desktop conventions half-done: no Ctrl+F, context menu suppressed with no row menu, caption buttons in tab order before "Fișier nou", format hint missing webp.

Personas: Alex - no Ctrl+F/Delete/Ctrl+C/row menu, sort only via select, no bulk, last selection reopens. Sam - 6 chrome tab stops before primary action, no h1, Escape gated on wide, ~11px chips.

Minor: 0 MB shown for 282 KB; localhost link in app; logo between back and file name in title bar; 800px overlay crumb still "Livrări / Toate"; dark selected row faint; passkeys section is a paragraph with no action.

Questions: does the app need a separate file page; should settings have Save at all; should the pane show "Neplătit · 49 €" upfront.
