<script lang="ts">
	// The new-device alert email's "this wasn't me" link lands here with a
	// signed token in the fragment. Nothing happens until the click (a mail
	// scanner opening the link must not sign anyone out).
	import { onMount } from 'svelte';
	import { i18n } from '$lib/i18n.svelte';
	import { api } from '$lib/api';
	import { auth } from '$lib/stores.svelte';
	import AuthShell from '$lib/components/AuthShell.svelte';

	let token = '';
	let phase = $state<'ready' | 'working' | 'done' | 'invalid'>('ready');
	let email = $state('');

	onMount(() => {
		token = new URLSearchParams(location.hash.slice(1)).get('token') ?? '';
		history.replaceState(history.state, '', location.pathname);
		if (!token) phase = 'invalid';
	});

	async function revoke() {
		phase = 'working';
		try {
			email = (await api.revokeFromAlert(token)).email;
			auth.user = null;
			phase = 'done';
		} catch {
			phase = 'invalid';
		}
	}
</script>

<svelte:head>
	<title>{i18n.t('revoke.title')} · SADE</title>
</svelte:head>

<AuthShell>
		{#if phase === 'invalid'}
			<h1>{i18n.t('confirm.invalidTitle')}</h1>
			<p class="muted">{i18n.t('login.errorInvalidLink')}</p>
			<a class="btn" href="/login">{i18n.t('confirm.backToLogin')}</a>
		{:else if phase === 'done'}
			<span class="icon-tile ok" aria-hidden="true">
				<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="var(--success)" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"
					><path d="M5 12.5 L10 17.5 L19 7" /></svg
				>
			</span>
			<p>{i18n.t('revoke.done', { email })}</p>
			<p class="muted small">{i18n.t('revoke.tip')}</p>
			<a class="btn" href="/login">{i18n.t('revoke.signIn')}</a>
		{:else}
			<span class="icon-tile" aria-hidden="true">
				<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="var(--danger)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
					><path d="M12 3 L20 6 V12 C20 16.5 16.5 20 12 21 C7.5 20 4 16.5 4 12 V6 Z" /><path d="M12 8 v5 M12 16 h.01" /></svg
				>
			</span>
			<div class="card-head">
				<h1>{i18n.t('revoke.title')}</h1>
				<p class="muted">{i18n.t('revoke.body')}</p>
			</div>
			<button class="btn danger-btn" type="button" onclick={revoke} disabled={phase === 'working'}>
				{phase === 'working' ? i18n.t('revoke.working') : i18n.t('revoke.submit')}
			</button>
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
		font-size: 1.75rem;
		font-weight: 500;
	}

	p {
		margin: 0;
		line-height: 1.5;
	}

	.card-head p,
	.small {
		font-size: 0.85rem;
	}

	.icon-tile {
		width: 48px;
		height: 48px;
		border-radius: 13px;
		background: var(--danger-tint);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.icon-tile.ok {
		background: var(--success-tint);
	}

	.danger-btn {
		background: var(--danger);
	}

	.danger-btn:hover:not(:disabled) {
		background: var(--danger);
		filter: brightness(0.92);
	}
</style>
