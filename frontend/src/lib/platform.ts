// Where the frontend runs: the web build served by the Go server, or the
// installed app (Tauri - Windows, macOS, Linux, iOS, Android), built with
// VITE_SADE_APP=1. The app lives on its own origin, so it talks to a SADE
// server by URL, keeps its session as a bearer token, and binds sign-in
// emails to a value it generated itself instead of a cookie.

export const IS_APP = import.meta.env.VITE_SADE_APP === '1';

const SERVER_KEY = 'sade:server';
const TOKEN_KEY = 'sade:token';
const NONCE_KEY = 'sade:loginNonce';

// The server the app talks to: a saved choice, else the one baked into the
// build (VITE_SADE_SERVER), else a local server.
export function appServer(): string {
	try {
		const saved = localStorage.getItem(SERVER_KEY);
		if (saved) return saved.replace(/\/+$/, '');
	} catch {
		// no storage
	}
	return (import.meta.env.VITE_SADE_SERVER || 'http://localhost:8080').replace(/\/+$/, '');
}

export function setAppServer(url: string) {
	try {
		localStorage.setItem(SERVER_KEY, url.trim().replace(/\/+$/, ''));
	} catch {
		// best-effort
	}
}

export function appToken(): string {
	try {
		return localStorage.getItem(TOKEN_KEY) ?? '';
	} catch {
		return '';
	}
}

export function setAppToken(token: string) {
	try {
		if (token) localStorage.setItem(TOKEN_KEY, token);
		else localStorage.removeItem(TOKEN_KEY);
	} catch {
		// best-effort
	}
}

// The app's own sign-in binding: 32 random bytes, base64url, kept for good.
// A forwarded email is useless without it, as with the browser's cookie.
export function appLoginNonce(): string {
	try {
		const saved = localStorage.getItem(NONCE_KEY);
		if (saved) return saved;
	} catch {
		// fall through to a fresh one
	}
	const bytes = crypto.getRandomValues(new Uint8Array(32));
	const value = btoa(String.fromCharCode(...bytes)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
	try {
		localStorage.setItem(NONCE_KEY, value);
	} catch {
		// a nonce for this run only
	}
	return value;
}

// Headers every app request carries.
export function appHeaders(): Record<string, string> {
	const h: Record<string, string> = { 'X-SADE-Client': 'app', 'X-SADE-Login': appLoginNonce() };
	const t = appToken();
	if (t) h['Authorization'] = `Bearer ${t}`;
	return h;
}

// Which desktop the installed app runs on, for the window's own controls:
// macOS puts them left as three lights, Windows right as caption buttons,
// Linux (GNOME) right as round buttons. Read from the webview's user agent.
export type DesktopOS = 'mac' | 'windows' | 'linux';

export function desktopOS(): DesktopOS {
	if (typeof navigator === 'undefined') return 'windows';
	const ua = navigator.userAgent;
	if (/Mac OS X|Macintosh/.test(ua)) return 'mac';
	if (/Windows/.test(ua)) return 'windows';
	return 'linux';
}
