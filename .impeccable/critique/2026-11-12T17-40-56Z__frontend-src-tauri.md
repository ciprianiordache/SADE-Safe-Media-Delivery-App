---
target: aplicatia instalata + design-taste-frontend
total_score: 27
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
target_identity: "file:C:\\Users\\ciipr\\Desktop\\sade\\frontend\\src-tauri"
timestamp: 2026-11-12T17-40-56Z
slug: frontend-src-tauri
---
Method: dual-agent, sequential (A design review + design-taste-frontend lens, then B detector over CDP + measured one-system checks + taste copy-lint). Installed build from commit 451aeac, local server 8080, demo operator.

Scores: 1:3 2:3 3:3 4:2 5:3 6:2 7:3 8:3 9:3 10:2 = 27/40 (Good, 68%)

Specificity: a desktop program now (caption buttons, split view, context menu, shortcuts, dot+word status). Web leftovers: marketing pills that lift on hover (.btn translateY), Settings cards on cream with a visible h1 that repeats the title bar.
Detector: only cream-palette / PositionPicker nested-cards false positives; dark clean; source scan 0. Contrast 0 failures (worst 5.6:1), no real small targets.
Taste lens (one system): code has ~17 button classes (.btn-danger vs .danger-btn duplicates, .link-btn defined 3x, .close-pane 2x, .text-btn), 5 link looks, radii 2/4/5/6/7/8/10/12/13/16/50%/999; live: 4-9 button looks per page, 4 heights (30/32/36/40), radii 999/12/10/8/7/50%, the white weight-400 "⋯" is the odd one. Press (:active) only on .btn and caption buttons; rows, side items, chip, close-pane, more-btn, segmented, dropzone have hover but no press; .switch and summary have neither. Global button:focus-visible sets border-radius (a focused switch loses its pill). .text-btn:hover turns red for every text button.
Copy (taste rules): "Gata!" x2 (exclaim), 6 em dashes (budget 2), virtue lines "în siguranță" x3, "Sigur? Oprește linkurile", objectless buttons ("Vezi", "Confirmă", "Arată"), one fact many words: oprește/dezactivează/reactivează, Deconectare/Deconectează/Ieși din cont, În așteptare/În coadă, previzualizare/cu filigran, "Gata" as button and as title; dead keys brand.save/leaveConfirm/unsaved. copy-lint's word rules are English-only.

Priority issues:
- P1 Not one control system: merge danger buttons, one link class, one .link-btn, radius scale 12/8/999 (+ maybe 4), heights 36/44 (+ 30 compact), "⋯" like the other hairline pills.
- P1 Press/hover feedback: shared :active for rows, side items, chip, close/more, segmented, dropzone, menu items, switch; switch hover; drop the hover lift in the app; .text-btn red only when .danger; remove border-radius from the global focus rule.
- P2 Settings vs the app's window rule: flat hairline groups, no visible h1 repeating the title bar, one content width for Brand and Security.
- P2 One wording per fact (Oprește/Pornește, Deconectează, În coadă, Cu filigran), no "!" and no "în siguranță" lines, em dashes to 2, objects on buttons, remove dead keys.
- P3 Windows list conventions: sortable column headers, "acum 1 h", table to full width; row accessible names (initials read aloud, columns lost), aria-haspopup on a navigating link.

Personas: Alex - no header sort, multi-select, Ctrl+C on a row, shortcut list; Sam - rows read as "IR ioana.radu… Deschis 1 h", column headers aria-hidden, focused switch changes shape; 1366x768 - fits, menu item wraps, Brand needs scroll while the preview is half empty.
Minor: "3 din 5 pornite" counts locked/unswitchable rows; "Mereu" lock looks like a button; placeholder looks filled; breadcrumb repeats the filter; dropzone focus ring on open; "Neplătit" box reads like a warning.
