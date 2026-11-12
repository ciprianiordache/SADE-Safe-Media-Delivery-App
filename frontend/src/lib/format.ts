import type { Locale } from './i18n.svelte';

// Compact relative time for rows and timelines: "2 min", "3 h", "5 zile"
// ("5 d" in English). Spelled out enough that "1m" can't read as a month.
// The one format for "how long ago"; fullDate is the one for "when".
export function relativeTime(iso: string, locale: Locale): string {
	const diffMs = Date.now() - new Date(iso).getTime();
	const min = Math.floor(diffMs / 60000);
	if (min < 1) return locale === 'ro' ? 'acum' : 'now';
	if (min < 60) return `${min} min`;
	const hr = Math.floor(min / 60);
	if (hr < 24) return `${hr} h`;
	const day = Math.floor(hr / 24);
	if (locale === 'ro') return day === 1 ? '1 zi' : `${day} zile`;
	return `${day} d`;
}

// The date as the server burns it into the watermark ("2026-11-12"), for
// previews of the file itself; everywhere else dates use fullDate.
export function burnedDate(d = new Date()): string {
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

// Full date for tooltips and detail pages: "12 nov. 2026, 11:41", the same
// format the emails use.
export function fullDate(iso: string, locale: Locale): string {
	return new Date(iso).toLocaleString(locale === 'ro' ? 'ro-RO' : 'en-GB', {
		dateStyle: 'medium',
		timeStyle: 'short'
	});
}

export function formatBytes(bytes: number): string {
	if (bytes < 1024) return `${bytes} B`;
	const kb = bytes / 1024;
	if (kb < 1024) return `${Math.round(kb)} KB`;
	const mb = kb / 1024;
	if (mb < 1024) return `${mb.toFixed(mb >= 100 ? 0 : 1)} MB`;
	return `${(mb / 1024).toFixed(2)} GB`;
}

// The server describes devices in English ("Chrome on Windows"); localise
// the connective and the unknown case.
// "49 €" / "€49": whole prices without decimals, the narrow currency sign.
export function formatPrice(cents: number, currency: string, locale: Locale): string {
	try {
		const whole = cents % 100 === 0;
		return new Intl.NumberFormat(locale === 'ro' ? 'ro-RO' : 'en-US', {
			style: 'currency',
			currency: currency.toUpperCase(),
			currencyDisplay: 'narrowSymbol',
			minimumFractionDigits: whole ? 0 : 2,
			maximumFractionDigits: whole ? 0 : 2
		}).format(cents / 100);
	} catch {
		return `${(cents / 100).toFixed(2)} ${currency.toUpperCase()}`;
	}
}

export function deviceLabel(device: string, locale: Locale): string {
	if (!device || device === 'Unknown device') return locale === 'ro' ? 'Dispozitiv necunoscut' : 'Unknown device';
	if (locale === 'ro') {
		// "SADE app on Windows" -> "Aplicația SADE pe Windows"
		return device.replace(/^SADE app /, 'Aplicația SADE ').replace(' on ', ' pe ');
	}
	return device;
}

// "acum 2h" / "2h ago" - relativeTime phrased as a past moment.
export function ago(iso: string, locale: Locale): string {
	const r = relativeTime(iso, locale);
	if (r === 'acum' || r === 'now') return r;
	return locale === 'ro' ? `acum ${r}` : `${r} ago`;
}

// Mirrors internals/wmopts.RenderText for the brand preview: {company},
// {recipient}, {date}; an empty company takes its separator with it.
export function renderWatermarkText(template: string, company: string, recipient: string): string {
	let t = template.trim() || '{recipient} · {date}';
	if (!company.trim()) {
		for (const sep of ['{company} · ', ' · {company}', '{company} - ', ' - {company}', '{company}']) {
			t = t.split(sep).join('');
		}
	}
	t = withRecipientPlaceholder(t);
	const d = new Date();
	const date = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	return t.split('{company}').join(company.trim()).split('{recipient}').join(recipient).split('{date}').join(date).trim();
}

// --- watermark text fit ----------------------------------------------------
// Mirrors internals/ffmpeg/textfit.go (shared examples:
// internals/ffmpeg/testdata/textfit.json). The preview has no real pixel
// width, so it works in cqw on a 1920px reference frame - above the width
// where the server's 18px font floor takes over, which is the only range
// where the two promise to match. Change both sides together.
const FIT = {
	charWidth: 0.62, // estimated glyph advance, x font size
	corner: 0.9, // share of the width one line may use
	tiled: 0.3, // ... per copy when tiled (copies sit a third apart)
	minScale: 0.6, // shrink this far before shortening the text
	baseCqw: 2.2, // normal font size: 2.2% of the width
	borderCqw: 8 / 19.2, // drawtext box border (8px at 1920px), each side
	absMinCqw: 6 / 19.2 // never smaller (6px at 1920px)
};

export interface TextFit {
	budget: number; // characters that fit at the smallest normal size
	fontCqw: (chars: number) => number;
}

export function fitWatermarkText(position: string): TextFit {
	const avail = (position === 'tiled' ? FIT.tiled : FIT.corner) * 100 - 2 * FIT.borderCqw;
	const minCqw = FIT.baseCqw * FIT.minScale;
	return {
		budget: Math.max(1, Math.floor(avail / (FIT.charWidth * minCqw))),
		fontCqw: (n) => (n <= 0 ? FIT.baseCqw : Math.min(FIT.baseCqw, Math.max(FIT.absMinCqw, avail / (FIT.charWidth * n))))
	};
}

// Characters as Go counts them (code points, not UTF-16 units).
export const charCount = (s: string) => [...s].length;

// renderWatermarkTextFit mirrors wmopts.RenderTextFit: a too-long text
// gives up the end of the company name, then the company, then the date -
// never the recipient.
export function renderWatermarkTextFit(template: string, company: string, recipient: string, budget: number): string {
	const date = today();
	const co = company.trim();
	const full = renderParts(template, co, recipient, date);
	if (budget <= 0 || charCount(full) <= budget) return full;
	let current = full;
	let keptCompany = co;
	if (co) {
		const without = renderParts(template, '', recipient, date);
		if (charCount(without) < charCount(full)) {
			const chars = [...co];
			for (let n = chars.length - 1; n >= 3; n--) {
				const t = renderParts(template, chars.slice(0, n).join('').trim() + '...', recipient, date);
				if (charCount(t) <= budget) return t;
			}
			if (charCount(without) <= budget) return without;
			current = without;
			keptCompany = '';
		}
	}
	const noDate = renderParts(template, keptCompany, recipient, '');
	return charCount(noDate) < charCount(current) ? noDate : current;
}

function today(): string {
	const d = new Date();
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

// Every copy names its recipient: a template without {recipient} gets it
// appended once company/date are settled (mirrors wmopts.render).
function withRecipientPlaceholder(template: string): string {
	const t = template.trim();
	if (t.includes('{recipient}')) return t;
	return t ? `${t} · {recipient}` : '{recipient}';
}

// withRecipient mirrors wmopts.WithRecipient: the operator's own text keeps
// the recipient's address, appended when the text does not carry it.
export function withRecipient(text: string, recipient: string, max = 200): string {
	const own = text.trim();
	const to = recipient.trim();
	if (!to) return own;
	if (!own) return to;
	if (own.toLowerCase().includes(to.toLowerCase())) return own;
	const room = max - charCount(to) - 3;
	if (room < 4) return to;
	const cut = charCount(own) > room ? [...own].slice(0, room - 3).join('').trim() + '...' : own;
	return `${cut} · ${to}`;
}

function renderParts(template: string, company: string, recipient: string, date: string): string {
	let t = template.trim() || '{recipient} · {date}';
	for (const [ph, value] of [
		['{company}', company],
		['{date}', date]
	] as const) {
		if (!value) {
			for (const sep of [`${ph} · `, ` · ${ph}`, `${ph} - `, ` - ${ph}`, ph]) t = t.split(sep).join('');
		}
	}
	t = withRecipientPlaceholder(t);
	return t.split('{company}').join(company).split('{recipient}').join(recipient).split('{date}').join(date).trim();
}
