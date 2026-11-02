import type { Locale } from './i18n.svelte';

// Compact relative time ("2m", "1h", "3d") matching the design's row/card
// timestamps. Not a full i18n formatter - just now/minutes/hours/days.
export function relativeTime(iso: string, locale: Locale): string {
	const diffMs = Date.now() - new Date(iso).getTime();
	const min = Math.floor(diffMs / 60000);
	if (min < 1) return locale === 'ro' ? 'acum' : 'now';
	if (min < 60) return `${min}m`;
	const hr = Math.floor(min / 60);
	if (hr < 24) return `${hr}h`;
	const day = Math.floor(hr / 24);
	return `${day}d`;
}

// Full date for tooltips and detail pages.
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
export function deviceLabel(device: string, locale: Locale): string {
	if (!device || device === 'Unknown device') return locale === 'ro' ? 'Dispozitiv necunoscut' : 'Unknown device';
	return locale === 'ro' ? device.replace(' on ', ' pe ') : device;
}
