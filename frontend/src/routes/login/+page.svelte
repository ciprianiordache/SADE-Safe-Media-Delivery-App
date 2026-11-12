<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { auth } from '$lib/stores.svelte';
	import { toasts } from '$lib/toast.svelte';
	import { finishSignIn, signInWithPasskey } from '$lib/signin';
	import { isCancel, passkeysSupported } from '$lib/webauthn';
	import type { SignInResult } from '$lib/types';
		import AuthShell from '$lib/components/AuthShell.svelte';
	import { IS_APP, appServer, setAppServer } from '$lib/platform';
	import { onDestroy, onMount } from 'svelte';

	const RESEND_SECONDS = 30;

	let email = $state('');
	let submitting = $state(false);
	let sent = $state(false);
	let error = $state('');
	let cooldown = $state(0);
	let timer: ReturnType<typeof setInterval> | undefined;

	let code = $state('');
	let codeBusy = $state(false);
	let codeError = $state('');

	// Passkeys: offered on the login form when the server and browser both
	// support them; also the second step for accounts that require one.
	let passkeysOn = $state(false);
	let passkeyBusy = $state(false);
	let passkeyError = $state('');
	let pendingStep = $state<SignInResult | null>(null);

		const linkError = page.url.searchParams.get('error') === 'invalid_link';

	// The installed app: which SADE server it signs in to.
	let server = $state(IS_APP ? appServer() : '');
	let editingServer = $state(false);
	function saveServer(e: SubmitEvent) {
		e.preventDefault();
		setAppServer(server);
		// API_BASE is read once at start: reload onto the new server.
		location.reload();
	}

	onMount(async () => {
		try {
			passkeysOn = passkeysSupported() && (await api.authConfig()).passkeys;
		} catch {
			passkeysOn = false;
		}
	});

	function startCooldown() {
		cooldown = RESEND_SECONDS;
		clearInterval(timer);
		timer = setInterval(() => {
			cooldown--;
			if (cooldown <= 0) clearInterval(timer);
		}, 1000);
	}

	onDestroy(() => clearInterval(timer));

	async function send(): Promise<boolean> {
		error = '';
		submitting = true;
		try {
			await api.requestLink(email.trim());
			startCooldown();
			return true;
		} catch (err) {
			error =
				err instanceof ApiError && err.status === 429
					? i18n.t('login.errorRateLimit')
					: i18n.t('login.errorGeneric');
			return false;
		} finally {
			submitting = false;
		}
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (await send()) sent = true;
	}

	async function resend() {
		if (await send()) toasts.show(i18n.t('login.resent'));
	}

	function useOther() {
		sent = false;
		error = '';
		code = '';
		codeError = '';
	}

	async function signedIn() {
		await auth.refresh();
		goto('/app');
	}

	// Runs the passkey prompt when the account requires one after the email
	// step; kept so the user can retry after cancelling the prompt.
	async function runPasskeyStep(step: SignInResult) {
		pendingStep = step;
		passkeyError = '';
		passkeyBusy = true;
		try {
			await finishSignIn(step);
			await signedIn();
		} catch (err) {
			if (!isCancel(err)) passkeyError = i18n.t('login.passkeyFailed');
		} finally {
			passkeyBusy = false;
		}
	}

	async function submitCode(e: SubmitEvent) {
		e.preventDefault();
		codeError = '';
		codeBusy = true;
		try {
			const res = await api.completeCode(email.trim(), code.trim());
			if (res.status === 'passkey_required') {
				await runPasskeyStep(res);
			} else {
				await signedIn();
			}
		} catch (err) {
			codeError =
				err instanceof ApiError && err.status === 400
					? i18n.t('login.codeInvalid')
					: err instanceof ApiError && err.status === 429
						? i18n.t('login.errorRateLimit')
						: i18n.t('common.error');
		} finally {
			codeBusy = false;
		}
	}

	function onCodeInput(e: Event) {
		code = (e.currentTarget as HTMLInputElement).value.replace(/\D/g, '').slice(0, 6);
	}

	async function passkeyOnly() {
		passkeyError = '';
		passkeyBusy = true;
		try {
			await signInWithPasskey();
			await signedIn();
		} catch (err) {
			if (!isCancel(err)) passkeyError = i18n.t('login.passkeyFailed');
		} finally {
			passkeyBusy = false;
		}
	}
</script>

<svelte:head>
	<title>{i18n.t('login.title')} · SADE</title>
</svelte:head>

<AuthShell>
		{#if pendingStep}
			<span class="icon-tile" aria-hidden="true">
				<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="var(--accent-tint-text)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"
					><circle cx="8" cy="15" r="4" /><path d="M10.8 12.2 L19 4 M16 7 l3 3 M14 9 l2 2" /></svg
				>
			</span>
			<div class="card-head">
				<h1>{i18n.t('login.passkeyStepTitle')}</h1>
				<p class="muted">{i18n.t('login.passkeyStepBody')}</p>
			</div>
			{#if passkeyError}
				<p class="error-text">{passkeyError}</p>
			{/if}
			<button class="btn" type="button" disabled={passkeyBusy} onclick={() => pendingStep && runPasskeyStep(pendingStep)}>
				{passkeyBusy ? i18n.t('common.loading') : i18n.t('login.passkeyRetry')}
			</button>
		{:else if sent}
			<span class="icon-tile" aria-hidden="true">
				<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="var(--accent-tint-text)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"
					><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M3.5 6.5 L12 13 L20.5 6.5" /></svg
				>
			</span>
			<div class="card-head">
				<h1>{i18n.t('login.sentTitle')}</h1>
				<p class="muted">{i18n.t('login.sentTo')}</p>
			</div>
			<p class="account mono">{email}</p>
						<p class="muted hint">{IS_APP ? i18n.t('login.appSentHint') : i18n.t('login.sentHint')}</p>

			<details class="code-box" open={IS_APP}>
				<summary>{i18n.t('login.codeToggle')}</summary>
			<form class="code-form" onsubmit={submitCode}>
				<label for="code">{i18n.t('login.codeLabel')}</label>
				<span class="muted small">{i18n.t('login.codeHint')}</span>
				<div class="code-row">
					<input
						id="code"
						class="code-input code-mono"
						type="text"
						inputmode="numeric"
						autocomplete="one-time-code"
						maxlength="6"
						placeholder="000000"
						value={code}
						oninput={onCodeInput}
					/>
					<button class="btn" type="submit" disabled={codeBusy || code.length !== 6}>
						{codeBusy ? i18n.t('common.loading') : i18n.t('login.codeSubmit')}
					</button>
				</div>
				{#if codeError}
					<p class="error-text">{codeError}</p>
				{/if}
			</form>
			</details>

			{#if error}
				<p class="error-text">{error}</p>
			{/if}
			<div class="actions">
				<button class="btn btn-ghost" type="button" onclick={resend} disabled={submitting || cooldown > 0}>
					{cooldown > 0 ? i18n.t('login.resendIn', { s: cooldown }) : i18n.t('login.resend')}
				</button>
				<button class="link-btn" type="button" onclick={useOther}>{i18n.t('login.useOther')}</button>
			</div>
		{:else}
			<div class="card-head">
				<h1>{i18n.t('login.title')}</h1>
				<p class="muted">{IS_APP ? i18n.t('login.appSubtitle') : i18n.t('login.subtitle')}</p>
			</div>

			{#if linkError}
				<div class="error-banner">
					<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" style="flex-shrink:0;margin-top:2px;"
						><path
							d="M12 8 v5 M12 16 h.01 M10.3 4 L3 17 a2 2 0 0 0 1.7 3 h14.6 a2 2 0 0 0 1.7 -3 L13.7 4 a2 2 0 0 0 -3.4 0 Z"
						/></svg
					>
					<span>{i18n.t('login.errorInvalidLink')}</span>
				</div>
			{/if}

			<form onsubmit={submit}>
				<div class="field">
					<label for="email">{i18n.t('login.emailLabel')}</label>
					<!-- svelte-ignore a11y_autofocus -->
					<input
						id="email"
						type="email"
						required
						autofocus
						autocomplete="email webauthn"
						inputmode="email"
						placeholder={i18n.t('login.emailPlaceholder')}
						bind:value={email}
					/>
				</div>
				{#if error}
					<p class="error-text">{error}</p>
				{/if}
				<button class="btn" type="submit" disabled={submitting} style="width:100%">
					{submitting ? i18n.t('login.submitting') : IS_APP ? i18n.t('login.appSubmit') : i18n.t('login.submit')}
				</button>
			</form>

			{#if passkeysOn}
				<div class="divider"><span>{i18n.t('login.or')}</span></div>
				<button class="btn btn-ink passkey-btn" type="button" onclick={passkeyOnly} disabled={passkeyBusy}>
					<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
						><circle cx="8" cy="15" r="4" /><path d="M10.8 12.2 L19 4 M16 7 l3 3 M14 9 l2 2" /></svg
					>
					{i18n.t('login.passkey')}
				</button>
				<p class="muted small center">{i18n.t('login.passkeyHint')}</p>
				{#if passkeyError}
					<p class="error-text center">{passkeyError}</p>
				{/if}
			{/if}
		{/if}
	{#if IS_APP}
		<details class="server" bind:open={editingServer}>
			<summary>{i18n.t('login.advanced')}</summary>
			<form class="server-form" onsubmit={saveServer}>
				<label for="server">{i18n.t('login.serverLabel')}</label>
				<div class="server-row">
					<input id="server" type="url" required placeholder="https://" bind:value={server} />
					<button class="btn btn-ink btn-sm" type="submit">{i18n.t('login.serverSave')}</button>
				</div>
				<span class="muted small">{i18n.t('login.serverHint')}</span>
			</form>
		</details>
	{/if}
</AuthShell>

<style>



	.card-head {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	h1 {
		margin: 0;
		font-size: 1.6rem;
		font-weight: 500;
		letter-spacing: -0.03em;
	}

	.card-head p,
	.hint {
		margin: 0;
		font-size: 0.92rem;
		line-height: 1.5;
	}

	.icon-tile {
		width: 48px;
		height: 48px;
		border-radius: 50%;
		background: var(--accent-tint);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.account {
		margin: 0;
		padding: 0.7rem 0.85rem;
		border-radius: var(--radius-sm);
		background: var(--fill);
		font-size: 0.9rem;
		overflow-wrap: anywhere;
	}

	.code-box {
		border-top: 1px solid var(--line);
	}

	.code-box summary {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		min-height: 44px;
		cursor: pointer;
		font-size: 0.88rem;
		font-weight: 500;
		color: var(--text);
		list-style: none;
	}

	.code-box summary::-webkit-details-marker {
		display: none;
	}

	.code-box summary::before {
		content: '';
		width: 0.45rem;
		height: 0.45rem;
		border-right: 2px solid var(--text-muted);
		border-bottom: 2px solid var(--text-muted);
		transform: rotate(-45deg);
		transition: transform 0.15s;
	}

	.code-box[open] summary::before {
		transform: rotate(45deg);
	}

	.code-form {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		padding-bottom: 0.25rem;
	}

	.code-form label {
		margin: 0;
	}

	.code-row {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.35rem;
	}

	.code-input {
		flex: 1;
		min-width: 0;
		font-size: 1.15rem;
		letter-spacing: 0.3em;
		text-align: center;
	}

	.code-row .btn {
		white-space: nowrap;
	}

	.small {
		font-size: 0.78rem;
		line-height: 1.45;
	}

	.center {
		text-align: center;
		margin: 0;
	}

	.actions {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		align-items: center;
	}

	.actions .btn {
		width: 100%;
	}

	.divider {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		color: var(--text-muted);
		font-size: 0.78rem;
	}

	.divider::before,
	.divider::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--line);
	}

	.passkey-btn {
		width: 100%;
	}

	.link-btn {
		appearance: none;
		background: none;
		border: none;
		color: var(--text-muted);
		font: inherit;
		font-size: 0.85rem;
		cursor: pointer;
		min-height: 44px;
		padding: 0 0.5rem;
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.link-btn:hover {
		color: var(--text);
	}

	.server {
		padding-top: 10px;
		border-top: 1px solid var(--line);
	}

	.server summary {
		display: inline-flex;
		align-items: center;
		min-height: 36px;
		font-size: 0.85rem;
		color: var(--text-muted);
		cursor: pointer;
	}

	.server summary:hover {
		color: var(--text);
	}

	.server-form {
		width: 100%;
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding-top: 6px;
	}

	.server-row {
		display: flex;
		gap: 8px;
	}

	.server-row input {
		flex: 1;
		min-width: 0;
		width: 100%;
		background: var(--surface);
		color: var(--text);
		border: 1px solid var(--line-2);
		border-radius: var(--radius-sm);
		min-height: 44px;
		padding: 0.6rem 0.8rem;
		font: inherit;
	}
</style>
