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
	import MarkBar from '$lib/components/MarkBar.svelte';
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

<div class="container top">
	<MarkBar />
</div>

<div class="container">
	<div class="card form-card">
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
			<p class="muted hint">{i18n.t('login.sentHint')}</p>

			<form class="code-form" onsubmit={submitCode}>
				<label for="code">{i18n.t('login.codeLabel')}</label>
				<span class="faint small">{i18n.t('login.codeHint')}</span>
				<div class="code-row">
					<input
						id="code"
						class="code-input mono"
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
				<p class="muted">{i18n.t('login.subtitle')}</p>
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
					{submitting ? i18n.t('login.submitting') : i18n.t('login.submit')}
				</button>
			</form>

			{#if passkeysOn}
				<div class="divider"><span>{i18n.t('login.or')}</span></div>
				<button class="btn btn-ghost passkey-btn" type="button" onclick={passkeyOnly} disabled={passkeyBusy}>
					<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
						><circle cx="8" cy="15" r="4" /><path d="M10.8 12.2 L19 4 M16 7 l3 3 M14 9 l2 2" /></svg
					>
					{i18n.t('login.passkey')}
				</button>
				<p class="faint small center">{i18n.t('login.passkeyHint')}</p>
				{#if passkeyError}
					<p class="error-text center">{passkeyError}</p>
				{/if}
			{/if}
		{/if}
	</div>
</div>

<style>
	.top {
		padding: 1.25rem 1.75rem 0;
	}

	.form-card {
		max-width: 396px;
		margin: 3.5rem auto;
		display: flex;
		flex-direction: column;
		gap: 1.1rem;
	}

	@media (max-width: 520px) {
		.top {
			padding: 1rem 1rem 0;
		}
		.form-card {
			margin: 2rem auto;
		}
	}

	.card-head {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	h1 {
		margin: 0;
		font-size: 1.45rem;
		font-weight: 700;
		letter-spacing: -0.01em;
	}

	.card-head p,
	.hint {
		margin: 0;
		font-size: 0.85rem;
		line-height: 1.5;
	}

	.icon-tile {
		width: 48px;
		height: 48px;
		border-radius: 13px;
		background: var(--accent-tint);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.account {
		margin: 0;
		padding: 0.7rem 0.85rem;
		border: 1px solid var(--border);
		border-radius: 10px;
		background: var(--bg);
		font-size: 0.9rem;
		overflow-wrap: anywhere;
	}

	.code-form {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		padding-top: 0.9rem;
		border-top: 1px solid var(--border);
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
		color: var(--text-faint);
		font-size: 0.78rem;
	}

	.divider::before,
	.divider::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--border);
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
		padding: 0.25rem;
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.link-btn:hover {
		color: var(--text);
	}
</style>
