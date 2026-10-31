<script lang="ts">
	import { goto } from '$app/navigation';
	import { i18n } from '$lib/i18n.svelte';
	import { auth } from '$lib/stores.svelte';
	import logo from '$lib/assets/logo.png';
	import LangThemeToggle from '$lib/components/LangThemeToggle.svelte';
	import { onMount } from 'svelte';
	import { UNAUTHORIZED_EVENT } from '$lib/api';

	let { children } = $props();

	onMount(() => {
		auth.ensureLoaded().then(() => {
			if (!auth.user) goto('/login');
		});
		const onUnauthorized = () => {
			auth.user = null;
			goto('/login');
		};
		window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
		return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
	});

	let menuOpen = $state(false);

	async function signOut() {
		menuOpen = false;
		await auth.logout();
		goto('/login');
	}

	async function signOutEverywhere() {
		menuOpen = false;
		await auth.logoutEverywhere();
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
					<button
						type="button"
						onclick={() => (menuOpen = !menuOpen)}
						aria-label={i18n.t('nav.signOut')}
						aria-expanded={menuOpen}
					>
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
					{#if menuOpen}
						<div class="menu" role="menu">
							<button type="button" role="menuitem" onclick={signOut}>{i18n.t('nav.signOut')}</button>
							<button type="button" role="menuitem" onclick={signOutEverywhere}>
								{i18n.t('nav.signOutEverywhere')}
							</button>
						</div>
					{/if}
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
		position: relative;
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

	.menu {
		position: absolute;
		top: calc(100% + 6px);
		right: 0;
		z-index: 20;
		min-width: 230px;
		display: flex;
		flex-direction: column;
		padding: 0.3rem;
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: 10px;
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.12);
	}

	.user-chip .menu button {
		padding: 0.5rem 0.65rem;
		border-radius: 6px;
		text-align: left;
		font: inherit;
		font-size: 0.82rem;
		color: var(--text);
	}

	.user-chip .menu button:hover {
		background: var(--bg);
	}
</style>
