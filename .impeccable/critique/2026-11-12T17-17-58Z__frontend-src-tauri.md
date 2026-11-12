---
target: aplicatia instalata dupa reparatii
total_score: 25
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
target_identity: "file:C:\\Users\\ciipr\\Desktop\\sade\\frontend\\src-tauri"
timestamp: 2026-11-12T17-17-58Z
slug: frontend-src-tauri
---
Method: dual-agent, sequential (A design review, then B detector over CDP 9222; one window). App rebuilt with today's fixes, local server on 8080, demo operator.

Scores: 1:3 2:3 3:2 4:2 5:3 6:3 7:2 8:3 9:3 10:1 = 25/40 (Acceptable, 63%)
vs previous run (26): status (2->3) and error recovery (2->3) improved with the offline/boot states; control (3->2), efficiency (3->2) and help (2->1) went down on newly examined points (no "Sistem" theme, no bulk/row shortcuts, no help at all).

Specificity: mostly a desktop program now (caption buttons, context menu, roving tab stop, Ctrl+N/,/F, Escape). Web leftovers: Brand's white cards on cream, file page cream head over white body, Chromium's default video player, "Linkuri active" pill, title bar changes shape per page.
Detector: CDP on list, pane, file page, Brand, Security, dark: only cream-palette and nested-cards (PositionPicker) false positives; source scan 0. Contrast 0 failures (worst 5.52:1 dark). Only small targets: Brand's two range sliders 207x16.

Priority issues:
- P1 Two buttons named "Închide": the pane's close (JobPanel.svelte:86, common.close) and the WINDOW's close (WindowControls.svelte:44, window.close) - the reviewer closed the whole app by name; voice control / screen reader users can too. Also Security: "Arată" x2, "Deconectează toate" x2 without context. Fix: "Închide panoul", "Închide SADE", aria-labels naming the device group.
- P1 No way back to the Windows theme: AccountMenu/LangThemeToggle offer only light/dark; 'system' is only the first-run default. Fix: "Sistem" option (default), follow prefers-color-scheme.
- P2 Title bar differs per page (search/sort only on the dashboard); maximize label never becomes "Restabilește". Fix: one title-bar layout; restore label.
- P2 Brand still cards on cream; "Linkuri active" pill; file page two-tone head. Fix: flat surface with hairline groups like Security; dot + word.
- P3 Chromium's default video controls; recipient link shows localhost (deploy config); disabled upload button without a reason; sliders 16px tall.

Personas: Alex - no multi-select/bulk, no Delete/Ctrl+C on a row, menus without shortcuts, no shortcut sheet; Sam - "Închide" collision, maximize label, rows are links (no "selected / 3 of 5"); 1366x768 - pane fits but history one row above the fold, 1024 chips cut "Plătite", the long email chip eats the title bar.
Minor: dead strip right of the list columns, dropzone focus ring on open reads as error, document.title doesn't follow the open pane, crumb truncates at 720.
