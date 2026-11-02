<script lang="ts">
	// The emailed sign-in link lands here with the token in the URL fragment
	// (#token=...), which browsers never send to a server. Nothing signs in
	// until the user clicks, and only in the browser that asked for the link:
	// anywhere else, the page explains how to use the code from the same
	// email instead.
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { auth } from '$lib/stores.svelte';
	import { finishSignIn } from '$lib/signin';
	import { isCancel } from '$lib/webauthn';
	import type { SignInResult } from '$lib/types';
	import MarkBar from '$lib/components/MarkBar.svelte';

	let token = '';
	let email = $state('');
	let phase = $state<'checking' | 'ready' | 'invalid' | 'signing' | 'otherBrowser' | 'passkey'>('checking');
	let error = $state('');
	let pendingStep: SignInResult | null = null;

	onMount(async () => {
		token = new URLSearchParams(location.hash.slice(1)).get('token') ?? '';
		// Drop the token from the address bar and history right away.
		history.replaceState(history.state, '', location.pathname);
		if (!token) {
			phase = 'invalid';
			return;
		}
		try {
			const v = await api.verifyLink(token);
			email = v.email;
			phase = v.sameBrowser ? 'ready' : 'otherBrowser';
		} catch {
			phase = 'invalid';
		}
	});

	async function done() {
		await auth.refresh();
		goto('/app');
	}

	async function passkeyStep() {
		if (!pendingStep) return;
		error = '';
		phase = 'signing';
		try {
			await finishSignIn(pendingStep);
			await done();
		} catch (err) {
			if (!isCancel(err)) error = i18n.t('login.passkeyFailed');
			phase = 'passkey';
		}
	}

	async function confirm() {
		error = '';
		phase = 'signing';
		try {
			const res = await api.completeLink(token);
			if (res.status === 'passkey_required') {
				pendingStep = res;
				await passkeyStep();
				return;
			}
			await done();
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				phase = 'otherBrowser';
			} else if (err instanceof ApiError && err.status === 400) {
				phase = 'invalid';
			} else {
				error = i18n.t('confirm.errorGeneric');
				phase = 'ready';
			}
		}
	}
</script>

<svelte:head>
	<title>{i18n.t('confirm.title')} · SADE</title>
</svelte:head>

<div class="container top">
	<MarkBar />
</div>

<div class="container">
	<div class="card form-card">
		{#if phase === 'checking'}
			<p class="muted">{i18n.t('common.loading')}</p>
		{:else if phase === 'invalid'}
			<h1>{i18n.t('confirm.invalidTitle')}</h1>
			<p class="muted">{i18n.t('login.errorInvalidLink')}</p>
			<a class="btn" href="/login" style="width:100%;text-align:center">{i18n.t('confirm.backToLogin')}</a>
		{:else if phase === 'otherBrowser'}
			<span class="icon-tile" aria-hidden="true">
				<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="var(--accent-tint-text)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"
					><rect x="2" y="4" width="14" height="10" rx="1.5" /><rect x="15" y="9" width="7" height="11" rx="1.5" /><path d="M6 18 h6" /></svg
				>
			</span>
			<div class="card-head">
				<h1>{i18n.t('confirm.otherBrowserTitle')}</h1>
				<p class="muted">{i18n.t('confirm.otherBrowserBody')}</p>
			</div>
			<p class="account mono">{email}</p>
			<a class="link" href="/login">{i18n.t('confirm.otherBrowserNew')}</a>
		{:else if phase === 'passkey'}
			<div class="card-head">
				<h1>{i18n.t('login.passkeyStepTitle')}</h1>
				<p class="muted">{i18n.t('login.passkeyStepBody')}</p>
			</div>
			{#if error}
				<p class="error-text">{error}</p>
			{/if}
			<button class="btn" type="button" onclick={passkeyStep} style="width:100%">
				{i18n.t('login.passkeyRetry')}
			</button>
		{:else}
			<div class="card-head">
				<h1>{i18n.t('confirm.title')}</h1>
				<p class="muted">{i18n.t('confirm.subtitle')}</p>
			</div>
			<p class="account mono">{email}</p>
			{#if error}
				<p class="error-text">{error}</p>
			{/if}
			<button class="btn" type="button" onclick={confirm} disabled={phase === 'signing'} style="width:100%">
				{phase === 'signing' ? i18n.t('confirm.signingIn') : i18n.t('confirm.submit')}
			</button>
			<p class="hint muted">{i18n.t('confirm.notYou')}</p>
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

	.link {
		font-size: 0.85rem;
		font-weight: 600;
	}
</style>
