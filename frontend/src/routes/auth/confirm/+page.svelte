<script lang="ts">
	// The emailed sign-in link lands here with the token in the URL fragment
	// (#token=...), which browsers never send to a server. Nothing signs in
	// until the user clicks: a mail scanner prefetching the link, or someone
	// planting it in another page, cannot start a session on their own.
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { auth } from '$lib/stores.svelte';
	import MarkBar from '$lib/components/MarkBar.svelte';

	let token = '';
	let email = $state('');
	let phase = $state<'checking' | 'ready' | 'invalid' | 'signing'>('checking');
	let error = $state('');

	onMount(async () => {
		token = new URLSearchParams(location.hash.slice(1)).get('token') ?? '';
		// Drop the token from the address bar and history right away.
		history.replaceState(history.state, '', location.pathname);
		if (!token) {
			phase = 'invalid';
			return;
		}
		try {
			email = (await api.verifyLink(token)).email;
			phase = 'ready';
		} catch {
			phase = 'invalid';
		}
	});

	async function confirm() {
		error = '';
		phase = 'signing';
		try {
			await api.completeLink(token);
			await auth.refresh();
			goto('/app');
		} catch (err) {
			if (err instanceof ApiError && err.status === 400) {
				phase = 'invalid';
			} else {
				error = i18n.t('confirm.errorGeneric');
				phase = 'ready';
			}
		}
	}
</script>

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
</style>
