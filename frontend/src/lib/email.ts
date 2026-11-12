// Catches the typo that matters most in SADE: the recipient's address is
// burned into the watermark and the email goes out at once, so "gmial.com"
// means a wrongly attributed file. Suggest only; the operator decides.

const COMMON_DOMAINS = [
	'gmail.com',
	'yahoo.com',
	'yahoo.ro',
	'hotmail.com',
	'outlook.com',
	'live.com',
	'icloud.com',
	'me.com',
	'protonmail.com',
	'proton.me',
	'gmx.com',
	'gmx.net',
	'aol.com',
	'ymail.com',
	'mail.com',
	'email.com'
];

function distance(a: string, b: string): number {
	const row = Array.from({ length: b.length + 1 }, (_, i) => i);
	for (let i = 1; i <= a.length; i++) {
		let prev = row[0];
		row[0] = i;
		for (let j = 1; j <= b.length; j++) {
			const cur = row[j];
			row[j] = Math.min(row[j] + 1, row[j - 1] + 1, prev + (a[i - 1] === b[j - 1] ? 0 : 1));
			prev = cur;
		}
	}
	return row[b.length];
}

/** The corrected address when the domain looks like a typo of a common one, else null. */
export function suggestEmail(email: string): string | null {
	const at = email.trim().lastIndexOf('@');
	if (at < 1) return null;
	const local = email.trim().slice(0, at);
	const domain = email.trim().slice(at + 1).toLowerCase();
	if (!domain || COMMON_DOMAINS.includes(domain)) return null;
	let best: string | null = null;
	let bestD = 3;
	for (const d of COMMON_DOMAINS) {
		const dist = distance(domain, d);
		if (dist < bestD) {
			bestD = dist;
			best = d;
		}
	}
	// One or two edits away from a known provider, e.g. gmial.com, gmail.co, yaho.com.
	return best && bestD <= 2 ? `${local}@${best}` : null;
}
