---
target: aplicatia instalata
total_score: 26
max_score: 40
na_heuristics: 
p0_count: 1
p1_count: 2
target_identity: "file:C:\\Users\\ciipr\\Desktop\\sade\\frontend\\src-tauri"
timestamp: 2026-11-12T16-49-08Z
slug: frontend-src-tauri
---
Method: dual-agent, sequential (A design review, then B detector over CDP 9222 on the running WebView2 app; one window, restored after each). Built from today's code, local server on 8080, demo operator.

Scores: 1:2 2:3 3:3 4:2 5:3 6:3 7:3 8:3 9:2 10:2 = 26/40 (Acceptable, 65%)

Specificity: ~70% native - frameless bar with Win11 caption buttons, list/pane split, context menu, Ctrl+N/,/F, "⋯" menus. Web leftovers: bordered pill status tags (DESIGN.md says dot + word), card-on-cream Settings, orange web focus rings in menus.
Detector: CDP runs on list, pane, file page, Brand, Security, dark: only cream-palette (brand paper) and nested-cards on PositionPicker frame - false positives. Source scan clean. Contrast 0 failures (lowest 4.88:1), no overflow, names complete, caption buttons tabindex -1.

Priority issues:
- P0 File drag-and-drop likely dead in the app: tauri.conf.json has no dragDropEnabled (default true intercepts OS drops on Windows), no onDragDropEvent anywhere; upload dropzone and LogoPicker both say "Trage". Fix: dragDropEnabled false (or onDragDropEvent), test from Explorer.
- P1 Server-down states lie: loadJobs (routes/app/+page.svelte:65-73) try/finally without catch -> failed load shows the empty account + upload form; layout auth.ensureLoaded() (+layout.svelte:17) has no catch -> hangs blank on network/5xx; polls fail silently. Fix: error state with Reîncearcă + "actualizat HH:MM", offline line in the title bar.
- P1 System contradicts itself: .st pills look pressable; Settings = white cards with shadow on cream inside a preferences window. Fix: dot + word in the app; one flat surface with hairline groups.
- P2 Keyboard/SR on the list: every row a Tab stop after the sidebar; rows are links with aria-current. Fix: roving tabindex, listbox/option + aria-selected, Home/End, accelerators in the context menu.
- P2 Web-style focus in menus (2px orange ring, persists after Escape); account menu repeats "Livrări"; no "System" theme. Fix: filled highlight row in menus.
- P3 Login shows "Server: http://..." to studios; email link opens the browser. Fix: behind "Opțiuni avansate"; sade:// deep link.
Also: /app?job= client nav doesn't open the pane (selectedId read once, :83); Security title "Setări · SADE" vs H1 "Securitate"; "Plătit" stated 3x on the file page; disabled upload button gives no reason.

Personas: Alex - no multi-select/batch, no header sort, no "new since last visit", file page duplicates the pane; Sam - Tab trap through rows, status pill looks interactive, disabled submit unexplained; 1366x768 operator - empty band right of "Când", 385px preview with the pane, Brand position grid below the fold, "curl · 3 sesiuni" alarming.
Minor: shield icon for Brand (reads as security), activity not grouped by day, placeholder looks filled, file-page head cream vs white body, "Deconectare de pe toate dispozitivele" wraps.
