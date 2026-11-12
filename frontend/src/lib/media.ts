// use:authedSrc / use:authedHref put an operator's own media on an element.
// In the browser the session cookie rides along, so the URL goes on as is.
// In the installed app an <img>, <video> or <a> cannot send the bearer
// token: a job's file becomes a short-lived signed link, and the brand logo
// is fetched with the token and shown from a blob: URL.
import { api } from './api';
import { IS_APP } from './platform';

const CONTENT = /\/api\/jobs\/([^/]+)\/assets\/([^/]+)\/content(\?dl=1)?$/;

async function resolve(url: string): Promise<{ url: string; blob: boolean }> {
	if (!IS_APP || !url) return { url, blob: false };
	const m = url.match(CONTENT);
	if (m) {
		const signed = await api.assetLink(decodeURIComponent(m[1]), decodeURIComponent(m[2]));
		return { url: m[3] ? `${signed}&dl=1` : signed, blob: false };
	}
	return { url: await api.blobUrl(url), blob: true };
}

function bind(node: HTMLElement, url: string, attr: 'src' | 'href') {
	let current = '';
	let blob = '';
	let seq = 0;

	async function apply(next: string) {
		if (next === current) return;
		current = next;
		const mine = ++seq;
		try {
			const r = await resolve(next);
			if (mine !== seq) {
				if (r.blob) URL.revokeObjectURL(r.url);
				return;
			}
			if (blob) URL.revokeObjectURL(blob);
			blob = r.blob ? r.url : '';
			node.setAttribute(attr, r.url);
		} catch {
			// leave the element empty; the page shows its own empty state
		}
	}

	apply(url);
	return {
		update(next: string) {
			apply(next);
		},
		destroy() {
			seq++;
			if (blob) URL.revokeObjectURL(blob);
		}
	};
}

export function authedSrc(node: HTMLImageElement | HTMLMediaElement, url: string) {
	return bind(node, url, 'src');
}

export function authedHref(node: HTMLAnchorElement, url: string) {
	return bind(node, url, 'href');
}

// The plain URL for the browser's first paint; undefined in the app, where
// the action above fills the attribute once it has a usable URL.
export function webSrc(url: string): string | undefined {
	return IS_APP ? undefined : url;
}
