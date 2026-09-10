<script lang="ts">
	import { goto } from '$app/navigation';
	import { i18n } from '$lib/i18n.svelte';
	import { auth } from '$lib/stores.svelte';
	import logo from '$lib/assets/logo.png';
	import LangThemeToggle from '$lib/components/LangThemeToggle.svelte';
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
		<p class="muted loading">{i18n.t('common.loading')}</p>
	</div>
{:else if auth.user}
	<div class="top-bar">
		<div class="container top-bar-inner">
			<a class="mark" href="/app">
				<img src={logo} alt="" width="24" height="24" />
				<span>SADE</span>
			</a>
			<div class="top-bar-right">
				<LangThemeToggle />
				<div class="user-chip">
					<span class="mono">{auth.user.email}</span>
					<button type="button" onclick={signOut} aria-label={i18n.t('nav.signOut')}>
						<svg
							width="13"
							height="13"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"><path d="M6 9 L12 15 L18 9" /></svg
						>
					</button>
				</div>
			</div>
		</div>
	</div>
	{@render children()}
{/if}

<style>
	.loading {
		padding: 2rem 0;
	}

	.top-bar {
		height: 56px;
		background: var(--surface);
		border-bottom: 1px solid var(--border);
		display: flex;
		align-items: center;
	}

	.top-bar-inner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
	}

	.mark {
		display: flex;
		align-items: center;
		gap: 0.65rem;
		color: var(--text);
		font-weight: 600;
		font-size: 0.85rem;
		letter-spacing: 0.14em;
	}

	.mark img {
		border-radius: 999px;
		display: block;
	}

	.top-bar-right {
		display: flex;
		align-items: center;
		gap: 0.9rem;
	}

	.user-chip {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.32rem 0.5rem 0.32rem 0.7rem;
		border: 1px solid var(--border);
		border-radius: 999px;
		font-size: 0.8rem;
		color: var(--text-muted);
	}

	.user-chip button {
		appearance: none;
		background: none;
		border: none;
		padding: 0.15rem;
		color: var(--text-faint);
		cursor: pointer;
		display: inline-flex;
	}

	.user-chip button:hover {
		color: var(--text);
	}
</style>
