---
target: aplicatia desktop instalata
total_score: 28
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
target_identity: "file:C:\\Users\\ciipr\\Desktop\\sade\\frontend\\src-tauri"
timestamp: 2026-11-11T16-59-20Z
slug: frontend-src-tauri
---
⚠️ DEGRADED: single-context (session policy: sub-agents only on explicit user request)

Target: the installed desktop app (Tauri, Windows build; macOS/Linux title bars checked by user-agent override).

| # | Heuristic | Score | Key Issue |
|---|---|---|---|
| 1 | Visibility of System Status | 3 | clear tags, selection, skeletons |
| 2 | Match System / Real World | 3 | system-specific window controls; emails not names |
| 3 | User Control and Freedom | 3 | pane closes with x/Escape; F5 on a deep route lands on /app |
| 4 | Consistency and Standards | 2 | file page and <1024px layout are still the web app |
| 5 | Error Prevention | 3 | |
| 6 | Recognition Rather Than Recall | 3 | |
| 7 | Flexibility and Efficiency | 3 | "/", arrows, Escape; no window state memory |
| 8 | Aesthetic and Minimalist Design | 3 | windows drawn inside the window |
| 9 | Error Recovery | 3 | |
| 10 | Help and Documentation | 2 | |
| Total | | 28/40 | Good |

Priority issues:
- [P1] The file page inside the app is the web page: two back links ("< Livrări" in the bar and "Înapoi la livrări"), a 48px web heading, and a decorative window with dots nested inside the app window. Fix: in the app, the file page drops the back link and page head (the bar carries "< Livrări" + the file name), the content fills the window like the open pane. /impeccable layout
- [P1] Below 1024px the app falls back to the web layout: list and form as floating rounded cards, the title bar inside a card, the window controls not at the window's edge. Fix: keep the app shell at every width - bar flush at the top, list full window; the form and the delivery open as a full-window sheet on narrow windows. /impeccable adapt
- [P2] Windows drawn inside the window: the brand "Previzualizare" panel and the file page carry the decorative three dots. Fix: in the app, inner panels use the plain bar (title only). /impeccable quieter
- [P2] Browser behaviour leaks: the WebView2 right-click menu (Back, Reload, Inspect), selectable UI labels, F5 on a deep route returning to /app. Fix: suppress the default context menu outside text fields, user-select none on chrome, keep the route across reloads. /impeccable harden
- [P3] Native <select> for sort; no remembered window size/position; no macOS app menu. /impeccable polish

Detector: source scan 0 findings; live scan of the running app over CDP (impeccable detect --cdp 9222) 1 finding, cream-palette (#fbf8f4 background) - false positive: the warm paper is the brand's committed palette from the marketing page (DESIGN.md).
