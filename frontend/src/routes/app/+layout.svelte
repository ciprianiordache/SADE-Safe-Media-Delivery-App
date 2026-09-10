<script lang="ts">
	import { goto } from '$app/navigation';
	import { i18n } from '$lib/i18n.svelte';
	import { auth } from '$lib/stores.svelte';
	import { onMount } from 'svelte';

	let { children } = $props();

	onMount(async () => {
		await auth.ensureLoaded();
		if (!auth.user) goto('/login');
	});

	async function signOut() {
		await auth.logout();
		goto('/login');
	}
</script>

{#if auth.loading}
	<div class="container">
		<p class="muted">{i18n.t('common.loading')}</p>
	</div>
{:else if auth.user}
	<nav class="container app-nav">
		<a href="/app">{i18n.t('nav.dashboard')}</a>
		<span class="muted">{auth.user.email}</span>
		<button class="btn btn-ghost" onclick={signOut}>{i18n.t('nav.signOut')}</button>
	</nav>
	{@render children()}
{/if}

<style>
	.app-nav {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 1rem 1.5rem;
	}

	.app-nav a {
		font-weight: 600;
		text-decoration: none;
	}

	.app-nav span {
		flex: 1;
	}
</style>
