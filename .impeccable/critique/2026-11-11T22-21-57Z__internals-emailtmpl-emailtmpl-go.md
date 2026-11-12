---
target: emailurile trimise
total_score: 18
max_score: 36
na_heuristics: 7
p0_count: 1
p1_count: 2
target_identity: "file:C:\\Users\\ciipr\\Desktop\\sade\\internals\\emailtmpl\\emailtmpl.go"
target_fingerprint: "sha256:9addfff6d1ff918a5095f34a1d4613f588485a8a1b0efd43a1de776d4c278aae"
target_path: "C:\\Users\\ciipr\\Desktop\\sade\\internals\\emailtmpl\\emailtmpl.go"
timestamp: 2026-11-11T22-21-57Z
slug: internals-emailtmpl-emailtmpl-go
---
Method: dual-agent (A design review, B detector + browser measurements; parallel, static rendered emails)

Scores: 1:3 2:1 3:3 4:1 5:2 6:2 7:n/a (one-shot transactional, no expert path) 8:3 9:2 10:1 = 18/36 (Acceptable, 50%)

Specificity: generic pre-redesign SaaS template; palette right, everything else contradicts DESIGN.md (IBM Plex not Reddit Sans, 700 headings, 9px buttons with white on orange, uppercase eyebrow, bordered card not the window).
Detector: 15 warnings (5 per email): contrast eyebrow #9c9287/#fbf8f4 2.9:1, notes #9c9287/#fff 3.05:1, footer #b4a997/#fbf8f4 2.2:1, white on #e8631c 3.4:1; cream-palette false positive. Measured: horizontal overflow at 390px in all three (592px; fixed 560px table). Web font only in browsers.

Priority issues:
- P0 Recipient email gives no reason to trust it ("Someone shared...", English, unknown brand, big Download). Fix: sender's verified account email, file name/type/size, what SADE is, unlock mention, Download secondary or gone.
- P1 English only. Fix: locale in Data, ro default for recipients, lang attr, Romanian durations.
- P1 Visual language behind the redesign. Fix: Reddit Sans/JetBrains Mono, 500 heading, pill buttons #111 on orange, line secondary, ink destructive, no uppercase, window bar motif.
- P2 Phone overflow + small-print contrast + note hairline glued to buttons (margin on td ignored).
- P2 Plain text drift: notify.go:58 prints 720h0m0s; alert facts as a sentence in HTML vs rows in text.

Personas: first-time recipient on phone - anonymous, overflowing, Download skips the page; screen reader - lang=en, brand read 3x, fallback URL as raw span, ambiguous "Download".
Minor: UTC-only time; footer "ignore" wrong for security alert; loud orange fallback URL; mono 600 synthesized; dash styles mixed.
