import type { Job } from './types';
import type { MessageKey } from './i18n.svelte';

// One status tag per delivery, as in the marketing page's list: where the
// file is in the pipeline, then what the recipient did with it.
export type DeliveryState = 'wait' | 'work' | 'failed' | 'off' | 'paid' | 'opened' | 'sent';

export function deliveryState(j: Job): DeliveryState {
	if (j.status === 'pending') return 'wait';
	if (j.status === 'processing') return 'work';
	if (j.status === 'failed') return 'failed';
	if (j.paid) return 'paid';
	if (j.linksDisabled) return 'off';
	if (j.openedAt) return 'opened';
	return 'sent';
}

const CLASS: Record<DeliveryState, string> = {
	wait: 'st st-wait',
	work: 'st st-work',
	failed: 'st st-bad',
	off: 'st st-bad',
	paid: 'st st-paid',
	opened: 'st st-open',
	sent: 'st st-done'
};

export function stateClass(s: DeliveryState): string {
	return CLASS[s];
}

export function stateKey(s: DeliveryState): MessageKey {
	return `st.${s === 'failed' ? 'failed' : s}` as MessageKey;
}

// "andreea.m@example.com" -> "AM"; "client@x.ro" -> "CL".
export function initials(email: string): string {
	const local = email.split('@')[0] ?? '';
	const parts = local.split(/[._\-+]+/).filter(Boolean);
	if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
	return local.slice(0, 2).toUpperCase() || '?';
}

// A stable tone 1-5 per address, so a client keeps their colour.
export function tone(email: string): number {
	let h = 0;
	for (const ch of email) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
	return (h % 5) + 1;
}
