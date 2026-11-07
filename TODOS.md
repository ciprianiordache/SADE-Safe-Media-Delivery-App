# TODOS

## Operator brand in the preview-ready email and on /preview

Deferred on 2026-11-07 (/plan-ceo-review, decisions D6 and D14). The recipient's
`/preview/[token]` page is the email's landing page, so it goes behind the same gate:
`GET /api/public/brand[/logo]` (`internals/app/share/brand.go`) and the sender block on
the page (company + logo above the Stripe form) are removed from this step too. The
company name still appears in the watermark itself (text template).

The brand work can put the operator's company name in the email to the recipient
(subject "X sent you a preview", heading, intro) and swap SADE's logo for theirs
(`worker.EmailNotifier.Branding`, `emailBranding` in `worker_wiring.go`,
`emailtmpl.Data.BrandName/BrandLogo`). Left out of this step because signup is open:
anyone can name their "company" after a bank or PayPal and have SADE mail any address,
with valid SPF/DKIM, a link to a page with a card form. Platforms that render a
user-controlled name field into mail they send have been abused at scale this way.

Do it together with the SaaS plans step, once there is something to gate it on:
- a verified sender (operator's own domain with SPF/DKIM, or a manual approval of the
  company name), and/or only paid/verified accounts get branded mail;
- the From name stays SADE ("X via SADE"), SADE's logo stays in the chrome;
- an abuse report link in every recipient email.
