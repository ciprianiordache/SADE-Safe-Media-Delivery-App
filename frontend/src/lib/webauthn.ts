// Bridges go-webauthn's JSON (byte fields as base64url strings) and the
// browser's navigator.credentials API (byte fields as ArrayBuffers), in both
// directions. No library: the shapes are small and fixed by the spec.

export function passkeysSupported(): boolean {
	return typeof window !== 'undefined' && !!window.PublicKeyCredential && !!navigator.credentials;
}

function b64urlToBuf(s: string): ArrayBuffer {
	const pad = '='.repeat((4 - (s.length % 4)) % 4);
	const bin = atob((s + pad).replace(/-/g, '+').replace(/_/g, '/'));
	const out = new Uint8Array(bin.length);
	for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
	return out.buffer;
}

function bufToB64url(buf: ArrayBuffer | null | undefined): string {
	if (!buf) return '';
	const bytes = new Uint8Array(buf);
	let bin = '';
	for (const b of bytes) bin += String.fromCharCode(b);
	return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

type Json = Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any

function descriptors(list: Json[] | undefined) {
	return (list ?? []).map((d) => ({ ...d, id: b64urlToBuf(d.id) }));
}

// Registration: options from POST /api/account/passkeys/begin -> the
// credential JSON POST /api/account/passkeys/finish expects.
export async function createPasskey(options: Json): Promise<Json> {
	const pk = options.publicKey;
	const publicKey: PublicKeyCredentialCreationOptions = {
		...pk,
		challenge: b64urlToBuf(pk.challenge),
		user: { ...pk.user, id: b64urlToBuf(pk.user.id) },
		excludeCredentials: descriptors(pk.excludeCredentials)
	};
	const cred = (await navigator.credentials.create({ publicKey })) as PublicKeyCredential | null;
	if (!cred) throw new Error('cancelled');
	const r = cred.response as AuthenticatorAttestationResponse;
	return {
		id: cred.id,
		rawId: bufToB64url(cred.rawId),
		type: cred.type,
		authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
		clientExtensionResults: cred.getClientExtensionResults(),
		response: {
			clientDataJSON: bufToB64url(r.clientDataJSON),
			attestationObject: bufToB64url(r.attestationObject),
			transports: typeof r.getTransports === 'function' ? r.getTransports() : []
		}
	};
}

// Sign-in: options from POST /api/auth/passkey/begin (or the
// passkey_required step) -> the credential JSON /api/auth/passkey/finish
// expects.
export async function getPasskey(options: Json): Promise<Json> {
	const pk = options.publicKey;
	const publicKey: PublicKeyCredentialRequestOptions = {
		...pk,
		challenge: b64urlToBuf(pk.challenge),
		allowCredentials: descriptors(pk.allowCredentials)
	};
	const cred = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null;
	if (!cred) throw new Error('cancelled');
	const r = cred.response as AuthenticatorAssertionResponse;
	return {
		id: cred.id,
		rawId: bufToB64url(cred.rawId),
		type: cred.type,
		authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
		clientExtensionResults: cred.getClientExtensionResults(),
		response: {
			clientDataJSON: bufToB64url(r.clientDataJSON),
			authenticatorData: bufToB64url(r.authenticatorData),
			signature: bufToB64url(r.signature),
			userHandle: r.userHandle ? bufToB64url(r.userHandle) : undefined
		}
	};
}

// The browser rejects with NotAllowedError when the user closes the prompt
// or it times out - not an error worth shouting about.
export function isCancel(err: unknown): boolean {
	return err instanceof DOMException && (err.name === 'NotAllowedError' || err.name === 'AbortError');
}

// A best-guess name for a new passkey, so the list reads "Chrome on
// Windows" instead of "Passkey".
export function deviceName(): string {
	const ua = navigator.userAgent;
	const browser = /Edg\//.test(ua)
		? 'Edge'
		: /Firefox\//.test(ua)
			? 'Firefox'
			: /Chrome\//.test(ua)
				? 'Chrome'
				: /Safari\//.test(ua)
					? 'Safari'
					: 'Browser';
	const os = /iPhone/.test(ua)
		? 'iPhone'
		: /iPad/.test(ua)
			? 'iPad'
			: /Android/.test(ua)
				? 'Android'
				: /Windows/.test(ua)
					? 'Windows'
					: /Mac OS X/.test(ua)
						? 'macOS'
						: /Linux/.test(ua)
							? 'Linux'
							: '';
	return os ? `${browser} · ${os}` : browser;
}
