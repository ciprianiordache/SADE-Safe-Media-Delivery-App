---
target: aplicatia desktop, a doua rulare
total_score: 29
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 0
target_identity: "file:C:\\Users\\ciipr\\Desktop\\sade\\frontend\\src-tauri"
timestamp: 2026-11-11T17-20-13Z
slug: frontend-src-tauri
---
⚠️ DEGRADED: single-context (session policy: sub-agents only on explicit user request)

Target: the installed desktop app, after the file-page, narrow-window, inner-dots, browser-behaviour and polish fixes.

| # | Heuristic | Score | Key Issue |
|---|---|---|---|
| 1 | Visibility of System Status | 3 | |
| 2 | Match System / Real World | 3 | browser scrollbars |
| 3 | User Control and Freedom | 3 | F5 now keeps the page |
| 4 | Consistency and Standards | 3 | upload pane closes with a pill, delivery pane with x |
| 5 | Error Prevention | 3 | |
| 6 | Recognition Rather Than Recall | 3 | |
| 7 | Flexibility and Efficiency | 3 | no Ctrl/Cmd+N, Ctrl/Cmd+, |
| 8 | Aesthetic and Minimalist Design | 3 | file name twice on the file page; empty bar band |
| 9 | Error Recovery | 3 | |
| 10 | Help and Documentation | 2 | |
| Total | | 29/40 | Good |

Priority issues:
- [P2] File page repeats itself: the file name in the title bar and again as the page heading; the inner panel's bar is an empty band with only the Cu filigran/Original switch. Fix: in the app, the head keeps the recipient line, status and Șterge; the switch moves into that head; the inner bar goes. /impeccable distill
- [P2] Browser scrollbars: thick grey bars with arrows on the file page and settings. Fix: thin, rounded, track-less scrollbars in the app (wider on hover). /impeccable polish
- [P2] Fonts load from Google Fonts at run time: offline (a studio on location) the app falls back to the system face. Fix: bundle Reddit Sans + JetBrains Mono with the app build. /impeccable harden
- [P2] Upload pane: the form stops at 520px leaving a third empty, and closes with an "Anulează" pill while the delivery pane closes with x. Fix: the form uses the pane (fields in a comfortable column, the file drop wider), close with the same x. /impeccable layout
- [P3] Desktop shortcuts: Ctrl/Cmd+N for a new file, Ctrl/Cmd+, for settings; the device list in Security grows long with seven identical "Chrome pe Windows" rows. /impeccable polish

Detector: source 0 findings; live CDP scan 1 (cream-palette, the brand's paper - false positive, as in run 4).
