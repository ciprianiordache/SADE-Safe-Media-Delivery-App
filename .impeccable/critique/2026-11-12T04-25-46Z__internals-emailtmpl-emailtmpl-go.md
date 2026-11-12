---
target: emailurile trimise (re-critique)
total_score: 24
max_score: 36
na_heuristics: 7
p0_count: 0
p1_count: 2
target_identity: "file:C:\\Users\\ciipr\\Desktop\\sade\\internals\\emailtmpl\\emailtmpl.go"
target_fingerprint: "sha256:583a1a3a7472c6569fd6977e4cb4ccd39253f4f5cba19ea613777ab199671816"
target_path: "C:\\Users\\ciipr\\Desktop\\sade\\internals\\emailtmpl\\emailtmpl.go"
timestamp: 2026-11-12T04-25-46Z
slug: internals-emailtmpl-emailtmpl-go
---
Method: dual-agent (A design review, B detector + browser measurements; parallel, static rendered emails at 390/680/1280, browser render only - not real Gmail/Outlook)

Scores: 1:3 2:3 3:2 4:3 5:2 6:3 7:n/a (one-shot transactional) 8:3 9:2 10:3 = 24/36 (67%, Acceptable, up from 18/36)

Specificity: now authored for SADE (Reddit Sans, ink-on-orange pills, hairline rows, window bar). The window bar with three dots is borrowed app chrome in an inbox: Outlook drops the dot spans, screen readers hear "SADE / Livrare" first.
Detector: 3 findings, all cream-palette (brand paper #fbf8f4, false positive); overlay same. Measured: no overflow at 390/1280, lang=ro, buttons 46px, every informative text >= 4.5:1 (only the decorative "/" in the bar 2.46:1), preheaders hidden, images-off and forced-dark fine. new-device has no fallback URL under its button.

Priority issues:
- P1 Buttons are padded inline-block <a> with border-radius (emailtmpl.go:255): Outlook desktop (Word engine) ignores padding/radius -> thin strip, not a pill. Fix: bulletproof td-padded button + VML v:roundrect in MSO conditional.
- P1 No Reply-To (mailer has none) and From name is English "Safe Media Delivery" (config/struct.go:181). A recipient cannot answer the sender; English sender on a Romanian body looks like phishing. Fix: Message.ReplyTo = operator's verified account email on the delivery email; From name "SADE".
- P2 Alert: "Închide toate sesiunile" comes before the condition; raw IP; no fallback link. Fix: "Nu ai fost tu?" line above the button, say it signs out this device too, drop/qualify IP, add fallback URL.
- P2 Delivery copy: sender named 3x (heading, De la row, footer), intro packs 3 ideas, no-password reassurance last, long heading lacks word-break, no-details fallback "Ai primit un fișier prin SADE" is the most phishing-like variant. Fix: drop De la row, split intro, move reassurance near the button, word-break on heading.
- P3 Plain-text drift: no footer in any text body; alert text drops passkey advice; sign-in text orders code after link.

Personas: phone recipient - subject is just an address from unknown English sender, heavy mono URL under the button, reassurance at the end; screen reader - window bar read before heading, heading is a td not h1, code read as one number (group "482 913"), URL read char by char; operator getting alert - raw IP, button warns of nothing, "link din email" doesn't hint mailbox risk.
Minor: Google Fonts link makes Apple Mail fetch remotely; code in lock-screen preheader (deliberate?); "curl" device unlocalized; "SADE (Safe Media Delivery)" English in 30-day note.
