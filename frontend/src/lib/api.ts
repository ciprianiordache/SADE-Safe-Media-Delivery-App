// Typed client for internals/app/router.go's /api/* surface. Session auth is
// a cookie (sade_session, HttpOnly) set by POST /api/auth/complete, so every
// call goes with credentials: 'include' - no token to attach by hand.
import type {
	ActivityEntry,
	Brand,
	BrandUpdate,
	Delivery,
	Job,
	NewJobInput,
	PasskeyChallenge,
	PasskeyInfo,
	PaymentStatus,
	SessionInfo,
	ShareInfo,
	SignInResult,
	Usage,
	User
} from './types';
import { IS_APP, appHeaders, appServer, setAppToken } from './platform';

// In dev the frontend runs on Vite (:5173) and the API on the Go server
// (config default :8080, CORS already allows :5173 - see config.ServerConfig
// and internals/app/middleware.go's CORS). The production build is served by
// the Go binary itself (internals/app/static.go), so same-origin relative
// paths are correct there.
// Exported so the public /preview/[token] page can build direct <video>/<a>
// URLs to GET /p/{token} and GET /d/{token} without a JSON round trip.
// Reuses whatever hostname loaded this page (not a hardcoded "localhost") so
// the same build works opened from another device on the LAN.
// The installed app talks to its configured server by URL (platform.ts).
export const API_BASE = IS_APP
	? appServer()
	: import.meta.env.DEV
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
		// The app's session is a bearer token, never a cookie.
		credentials: IS_APP ? 'omit' : 'include',
		...init,
		headers: {
			...(init?.body && !(init.body instanceof FormData)
				? { 'Content-Type': 'application/json' }
				: {}),
			...(IS_APP ? appHeaders() : {}),
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
	const data = (await res.json()) as T;
	// A finished sign-in hands the app its session token (the server only
	// includes it for X-SADE-Client: app).
	if (IS_APP && path.startsWith('/api/auth/')) {
		const tok = (data as { token?: unknown } | null)?.token;
		if (typeof tok === 'string' && tok) setAppToken(tok);
	}
	return data;
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

		async authConfig(): Promise<{ passkeys: boolean }> {
		const cfg = await request<{ passkeys: boolean }>('/api/auth/config');
		return IS_APP ? { ...cfg, passkeys: false } : cfg;
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

	// --- brand ---------------------------------------------------------------
	brand(): Promise<Brand> {
		return request('/api/brand');
	},

	updateBrand(b: BrandUpdate): Promise<Brand> {
		return request('/api/brand', { method: 'PUT', body: JSON.stringify(b) });
	},

	uploadLogo(file: File): Promise<Brand> {
		const form = new FormData();
		form.set('logo', file);
		return request('/api/brand/logo', { method: 'POST', body: form });
	},

	deleteLogo(): Promise<Brand> {
		return request('/api/brand/logo', { method: 'DELETE' });
	},

	completeOnboarding(): Promise<Brand> {
		return request('/api/brand/onboarded', { method: 'POST' });
	},

	// The account's own logo; version busts the cache when it changes.
	brandLogoUrl(version?: string): string {
		return `${API_BASE}/api/brand/logo?v=${encodeURIComponent(version ?? '')}`;
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

	async logout(): Promise<void> {
		try {
			await request('/api/auth/logout', { method: 'POST' });
		} finally {
			if (IS_APP) setAppToken('');
		}
	},

	// Ends every session of the signed-in account, on every device.
	async logoutAll(): Promise<void> {
		try {
			await request('/api/auth/logout-all', { method: 'POST' });
		} finally {
			if (IS_APP) setAppToken('');
		}
	},

	// The installed app's players cannot send the token: a short-lived signed
	// URL of the same content route (GET .../link).
	async assetLink(jobId: string, assetId: string): Promise<string> {
		const r = await request<{ url: string }>(
			`/api/jobs/${encodeURIComponent(jobId)}/assets/${encodeURIComponent(assetId)}/link`
		);
		return `${API_BASE}${r.url}`;
	},

	// Fetches a session-only resource (the brand logo) with the app's token
	// and hands back a blob: URL for an <img>.
	async blobUrl(url: string): Promise<string> {
		const res = await fetch(url, { credentials: 'omit', headers: appHeaders() });
		if (!res.ok) throw new ApiError(res.status, res.statusText);
		return URL.createObjectURL(await res.blob());
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

	// Delivery: the recipient's link, resend, links on/off, history.
	delivery(id: string): Promise<Delivery> {
		return request(`/api/jobs/${encodeURIComponent(id)}/delivery`);
	},

	resend(id: string): Promise<void> {
		return request(`/api/jobs/${encodeURIComponent(id)}/resend`, { method: 'POST' });
	},

	setLinks(id: string, enabled: boolean): Promise<void> {
		return request(`/api/jobs/${encodeURIComponent(id)}/links`, {
			method: 'PUT',
			body: JSON.stringify({ enabled })
		});
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
			xhr.withCredentials = !IS_APP;
			if (IS_APP) for (const [k, v] of Object.entries(appHeaders())) xhr.setRequestHeader(k, v);
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
	// Which state a recipient's link is in, and what it holds. 404 = not a
	// link SADE issued; 410 "expired" / "disabled".
	shareInfo(previewToken: string): Promise<ShareInfo> {
		return request(`/api/share/info?token=${encodeURIComponent(previewToken)}`);
	},

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
