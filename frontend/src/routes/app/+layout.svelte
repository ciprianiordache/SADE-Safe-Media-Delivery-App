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
	let chipEl = $state<HTMLDivElement | undefined>();

	function onWindowClick(e: MouseEvent) {
		if (menuOpen && chipEl && !chipEl.contains(e.target as Node)) menuOpen = false;
	}

	function onWindowKey(e: KeyboardEvent) {
		if (e.key === 'Escape') menuOpen = false;
	}

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

<svelte:window onclick={onWindowClick} onkeydown={onWindowKey} />

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
				<div class="user-chip" bind:this={chipEl}>
					<button
						type="button"
						class="chip-btn"
						onclick={() => (menuOpen = !menuOpen)}
						aria-haspopup="menu"
						aria-expanded={menuOpen}
					>
						<span class="avatar" aria-hidden="true">{auth.user.email[0]?.toUpperCase()}</span>
						<span class="mono email">{auth.user.email}</span>
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
							<span class="menu-email mono">{auth.user.email}</span>
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
	}

	.chip-btn {
		appearance: none;
		background: none;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.25rem 0.55rem 0.25rem 0.25rem;
		border: 1px solid var(--border);
		border-radius: 999px;
		font-size: 0.8rem;
		color: var(--text-muted);
		cursor: pointer;
	}

	.chip-btn:hover {
		color: var(--text);
		border-color: var(--border-strong);
	}

	.avatar {
		width: 24px;
		height: 24px;
		border-radius: 999px;
		background: var(--accent-tint);
		color: var(--accent-tint-text);
		font-weight: 700;
		font-size: 0.75rem;
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	@media (max-width: 600px) {
		.email {
			display: none;
		}
		.top-bar-right {
			gap: 0.5rem;
		}
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

	.menu-email {
		padding: 0.45rem 0.65rem 0.55rem;
		margin-bottom: 0.25rem;
		border-bottom: 1px solid var(--border);
		font-size: 0.75rem;
		color: var(--text-faint);
		overflow-wrap: anywhere;
	}

	.menu button {
		appearance: none;
		background: none;
		border: none;
		cursor: pointer;
		padding: 0.5rem 0.65rem;
		border-radius: 6px;
		text-align: left;
		font: inherit;
		font-size: 0.82rem;
		color: var(--text);
	}

	.menu button:hover {
		background: var(--bg);
	}
</style>
