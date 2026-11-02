// Typed client for internals/app/router.go's /api/* surface. Session auth is
// a cookie (sade_session, HttpOnly) set by POST /api/auth/complete, so every
// call goes with credentials: 'include' - no token to attach by hand.
import type {
	ActivityEntry,
	Job,
	NewJobInput,
	PasskeyChallenge,
	PasskeyInfo,
	PaymentStatus,
	SessionInfo,
	SignInResult,
	Usage,
	User
} from './types';

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

// Fired when an authenticated call comes back 401 - the session expired, or
// was ended from another device ("sign out on all devices"). The /app layout
// listens and sends the user to /login instead of leaving a dashboard that
// silently fails every poll.
export const UNAUTHORIZED_EVENT = 'sade:unauthorized';

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
		if (res.status === 401 && path !== '/api/me' && typeof window !== 'undefined') {
			window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
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

	// The /auth/confirm page: which account a sign-in link belongs to and
	// whether this is the browser that asked for it (does not spend it), then
	// the explicit sign-in on the user's click (409 "other_browser" from
	// anywhere else).
	verifyLink(token: string): Promise<{ email: string; sameBrowser: boolean }> {
		return request('/api/auth/verify', { method: 'POST', body: JSON.stringify({ token }) });
	},

	completeLink(token: string): Promise<SignInResult> {
		return request('/api/auth/complete', { method: 'POST', body: JSON.stringify({ token }) });
	},

	// The 6-digit code from the same email, typed in the requesting browser.
	completeCode(email: string, code: string): Promise<SignInResult> {
		return request('/api/auth/code', { method: 'POST', body: JSON.stringify({ email, code }) });
	},

	authConfig(): Promise<{ passkeys: boolean }> {
		return request('/api/auth/config');
	},

	passkeyLoginBegin(): Promise<PasskeyChallenge> {
		return request('/api/auth/passkey/begin', { method: 'POST' });
	},

	passkeyLoginFinish(ceremonyId: string, credential: unknown): Promise<void> {
		return request('/api/auth/passkey/finish', {
			method: 'POST',
			body: JSON.stringify({ ceremonyId, credential })
		});
	},

	// The new-device alert email's "this wasn't me".
	revokeFromAlert(token: string): Promise<{ email: string }> {
		return request('/api/auth/revoke', { method: 'POST', body: JSON.stringify({ token }) });
	},

	// --- account security ---------------------------------------------------
	sessions(): Promise<SessionInfo[]> {
		return request('/api/account/sessions');
	},

	revokeSession(id: string): Promise<void> {
		return request(`/api/account/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' });
	},

	activity(): Promise<ActivityEntry[]> {
		return request('/api/account/activity');
	},

	passkeys(): Promise<PasskeyInfo[]> {
		return request('/api/account/passkeys');
	},

	passkeyRegisterBegin(): Promise<PasskeyChallenge> {
		return request('/api/account/passkeys/begin', { method: 'POST' });
	},

	passkeyRegisterFinish(ceremonyId: string, credential: unknown, name: string): Promise<PasskeyInfo> {
		return request('/api/account/passkeys/finish', {
			method: 'POST',
			body: JSON.stringify({ ceremonyId, credential, name })
		});
	},

	deletePasskey(id: string): Promise<void> {
		return request(`/api/account/passkeys/${encodeURIComponent(id)}`, { method: 'DELETE' });
	},

	setRequirePasskey(enabled: boolean): Promise<User> {
		return request('/api/account/require-passkey', {
			method: 'PUT',
			body: JSON.stringify({ enabled })
		});
	},

	logout(): Promise<void> {
		return request('/api/auth/logout', { method: 'POST' });
	},

	// Ends every session of the signed-in account, on every device.
	logoutAll(): Promise<void> {
		return request('/api/auth/logout-all', { method: 'POST' });
	},

	usage(): Promise<Usage> {
		return request('/api/usage');
	},

	// --- jobs ----------------------------------------------------------------
	listJobs(offset = 0, limit = 50): Promise<Job[]> {
		return request(`/api/jobs?offset=${offset}&limit=${limit}`);
	},

	getJob(id: string): Promise<Job> {
		return request(`/api/jobs/${encodeURIComponent(id)}`);
	},

	deleteJob(id: string): Promise<void> {
		return request(`/api/jobs/${encodeURIComponent(id)}`, { method: 'DELETE' });
	},

	// Uploads over XHR rather than fetch: fetch has no upload-progress event,
	// and a 2 GiB upload with no progress bar looks frozen. onProgress gets
	// 0..1; the signal aborts the upload (rejects with an AbortError).
	createJob(
		input: NewJobInput,
		opts: { onProgress?: (fraction: number) => void; signal?: AbortSignal } = {}
	): Promise<Job> {
		const form = new FormData();
		form.set('file', input.file);
		form.set('recipientEmail', input.recipientEmail);
		form.set('watermarkKind', input.watermarkKind);
		if (input.watermarkText) form.set('watermarkText', input.watermarkText);

		return new Promise((resolve, reject) => {
			const xhr = new XMLHttpRequest();
			xhr.open('POST', `${API_BASE}/api/jobs`);
			xhr.withCredentials = true;
			xhr.responseType = 'text';
			xhr.upload.onprogress = (e) => {
				if (e.lengthComputable) opts.onProgress?.(e.loaded / e.total);
			};
			xhr.onload = () => {
				let body: unknown = null;
				try {
					body = JSON.parse(xhr.responseText);
				} catch {
					// non-JSON body
				}
				if (xhr.status >= 200 && xhr.status < 300) {
					resolve(body as Job);
					return;
				}
				if (xhr.status === 401) window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
				const msg =
					body && typeof (body as { error?: unknown }).error === 'string'
						? (body as { error: string }).error
						: xhr.statusText;
				reject(new ApiError(xhr.status, msg));
			};
			xhr.onerror = () => reject(new ApiError(0, 'network error'));
			xhr.onabort = () => reject(new DOMException('aborted', 'AbortError'));
			opts.signal?.addEventListener('abort', () => xhr.abort());
			xhr.send(form);
		});
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

	// Starts (or resumes) paying for the job behind previewToken. Despite the
	// route's name, this mints a Stripe PaymentIntent for the Payment
	// Element form (PaymentForm.svelte), not a hosted Checkout redirect.
	paymentIntent(previewToken: string): Promise<{ clientSecret: string }> {
		return request('/api/payments/checkout', {
			method: 'POST',
			body: JSON.stringify({ token: previewToken })
		});
	}
};
