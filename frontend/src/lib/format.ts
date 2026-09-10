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
