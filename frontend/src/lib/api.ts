// Typed client for internals/app/router.go's /api/* surface. Session auth is
// a cookie (sade_session, HttpOnly) set by GET /api/auth/callback, so every
// call goes with credentials: 'include' - no token to attach by hand.
import type { Job, NewJobInput, PaymentStatus, User } from './types';

// In dev the frontend runs on Vite (:5173) and the API on the Go server
// (config default :8080, CORS already allows :5173 - see config.ServerConfig
// and internals/app/middleware.go's CORS). The production build is served by
// the Go binary itself (internals/app/static.go), so same-origin relative
// paths are correct there.
// Exported so the public /preview/[token] page can build direct <video>/<a>
// URLs to GET /p/{token} and GET /d/{token} without a JSON round trip.
// Reuses whatever hostname loaded this page (not a hardcoded "localhost") so
// the same build works opened from another device on the LAN.
export const API_BASE = import.meta.env.DEV
	? `http://${typeof location === 'undefined' ? 'localhost' : location.hostname}:8080`
	: '';

export class ApiError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.status = status;
	}
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(`${API_BASE}${path}`, {
		credentials: 'include',
		...init,
		headers: {
			...(init?.body && !(init.body instanceof FormData)
				? { 'Content-Type': 'application/json' }
				: {}),
			...init?.headers
		}
	});
	if (!res.ok) {
		let message = res.statusText;
		try {
			const body = await res.json();
			if (typeof body?.error === 'string') message = body.error;
		} catch {
			// non-JSON error body - fall back to statusText
		}
		throw new ApiError(res.status, message);
	}
	if (res.status === 204) return undefined as T;
	return (await res.json()) as T;
}

export const api = {
	// --- auth --------------------------------------------------------------
	requestLink(email: string): Promise<void> {
		return request('/api/auth/request', {
			method: 'POST',
			body: JSON.stringify({ email })
		});
	},

	async me(): Promise<User | null> {
		try {
			return await request<User>('/api/me');
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) return null;
			throw err;
		}
	},

	logout(): Promise<void> {
		return request('/api/auth/logout', { method: 'POST' });
	},

	// --- jobs ----------------------------------------------------------------
	listJobs(offset = 0, limit = 50): Promise<Job[]> {
		return request(`/api/jobs?offset=${offset}&limit=${limit}`);
	},

	getJob(id: string): Promise<Job> {
		return request(`/api/jobs/${encodeURIComponent(id)}`);
	},

	createJob(input: NewJobInput): Promise<Job> {
		const form = new FormData();
		form.set('file', input.file);
		form.set('recipientEmail', input.recipientEmail);
		form.set('watermarkKind', input.watermarkKind);
		if (input.watermarkText) form.set('watermarkText', input.watermarkText);
		return request('/api/jobs', { method: 'POST', body: form });
	},

	// GET /api/jobs/{id}/assets/{assetId}/content: session-authenticated
	// streaming of one of the caller's own job assets (original or preview) -
	// unlike GET /p/{token}, which only ever serves a preview to whoever
	// holds the emailed link. A plain <video>/<audio>/<img src> tag sends the
	// session cookie itself (same-site, just a different port in dev), so
	// this is a URL builder, not a fetch wrapper.
	assetContentUrl(jobId: string, assetId: string, opts?: { download?: boolean }): string {
		const dl = opts?.download ? '?dl=1' : '';
		return `${API_BASE}/api/jobs/${encodeURIComponent(jobId)}/assets/${encodeURIComponent(assetId)}/content${dl}`;
	},

	// --- payments (public, authorised by the preview token itself) -------
	paymentStatus(previewToken: string): Promise<PaymentStatus> {
		return request(`/api/payments/status?token=${encodeURIComponent(previewToken)}`);
	},

	paymentCheckout(previewToken: string): Promise<{ checkoutUrl: string }> {
		return request('/api/payments/checkout', {
			method: 'POST',
			body: JSON.stringify({ token: previewToken })
		});
	}
};
