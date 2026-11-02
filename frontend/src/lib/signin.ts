// Finishes a sign-in after the email step (link or code): when the account
// also requires a passkey, runs the browser's passkey prompt and sends the
// result. Resolves once the session cookie is set; rejects if the prompt is
// cancelled or the passkey is refused.
import { api } from './api';
import type { SignInResult } from './types';
import { getPasskey } from './webauthn';

export async function finishSignIn(result: SignInResult): Promise<void> {
	if (result.status === 'signed_in') return;
	const credential = await getPasskey(result.passkey.options);
	await api.passkeyLoginFinish(result.passkey.ceremonyId, credential);
}

// Passkey-only sign-in (no email).
export async function signInWithPasskey(): Promise<void> {
	const ch = await api.passkeyLoginBegin();
	const credential = await getPasskey(ch.options);
	await api.passkeyLoginFinish(ch.ceremonyId, credential);
}
