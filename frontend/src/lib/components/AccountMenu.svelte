<script lang="ts">
	// The operator's chip and its menu: where to go (deliveries, settings),
	// language and theme, signing out. In the web header it sits right of
	// "Fișier nou"; in the installed app it sits in the window's title bar.
	import { goto } from '$app/navigation';
	import { i18n } from '$lib/i18n.svelte';
	import { auth } from '$lib/stores.svelte';
	import { theme } from '$lib/theme.svelte';
	import { initials, tone } from '$lib/delivery';

	let { compact = false, alwaysNav = false }: { compact?: boolean; alwaysNav?: boolean } = $props();

	let menuOpen = $state(false);
	let chipEl = $state<HTMLDivElement | undefined>();
	let chipBtn = $state<HTMLButtonElement | undefined>();
	let menuEl = $state<HTMLDivElement | undefined>();

	let isDark = $derived(
		theme.value === 'dark' ||
			(theme.value === 'system' && typeof matchMedia !== 'undefined' && matchMedia('(prefers-color-scheme: dark)').matches)
	);

	function onWindowClick(e: MouseEvent) {
		if (menuOpen && chipEl && !chipEl.contains(e.target as Node)) menuOpen = false;
	}

	function onWindowKey(e: KeyboardEvent) {
		if (e.key === 'Escape' && menuOpen) {
			menuOpen = false;
			chipBtn?.focus();
		}
	}

	function menuItems(): HTMLElement[] {
		return Array.from(menuEl?.querySelectorAll<HTMLElement>('a, button') ?? []).filter((el) => el.offsetParent !== null);
	}

	// The first item gets focus on open; arrows move, Home/End jump.
	$effect(() => {
		if (menuOpen && menuEl) menuItems()[0]?.focus();
	});

	function onMenuKey(e: KeyboardEvent) {
		const items = menuItems();
		const i = items.indexOf(document.activeElement as HTMLElement);
		let next = -1;
		if (e.key === 'ArrowDown') next = (i + 1) % items.length;
		else if (e.key === 'ArrowUp') next = (i - 1 + items.length) % items.length;
		else if (e.key === 'Home') next = 0;
		else if (e.key === 'End') next = items.length - 1;
		if (next >= 0) {
			e.preventDefault();
			items[next]?.focus();
		}
	}

	// Tabbing out of the menu closes it.
	function onMenuFocusOut(e: FocusEvent) {
		if (chipEl && !chipEl.contains(e.relatedTarget as Node | null)) menuOpen = false;
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

{#if auth.user}
	<div class="user-chip" class:compact bind:this={chipEl} onfocusout={onMenuFocusOut}>
		<button
			type="button"
			class="chip-btn"
			bind:this={chipBtn}
			onclick={() => (menuOpen = !menuOpen)}
			aria-haspopup="true"
			aria-expanded={menuOpen}
			aria-label={`${i18n.t('nav.account')}: ${auth.user.email}`}
		>
			<span class="avatar" data-tone={tone(auth.user.email)} aria-hidden="true">{initials(auth.user.email)}</span>
			<span class="mono email" aria-hidden="true">{auth.user.email}</span>
			<svg class="chev" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9 L12 15 L18 9" /></svg>
		</button>
		{#if menuOpen}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div class="menu" bind:this={menuEl} onkeydown={onMenuKey}>
				<div class="menu-who">
					<span class="avatar" data-tone={tone(auth.user.email)} aria-hidden="true">{initials(auth.user.email)}</span>
					<span class="mono">{auth.user.email}</span>
				</div>
				{#if !alwaysNav}
					<a class="menu-item narrow-only" href="/app" onclick={() => (menuOpen = false)}>{i18n.t('nav.deliveries')}</a>
				{/if}
				<a class="menu-item" href="/app/settings" onclick={() => (menuOpen = false)}>{i18n.t('nav.settings')}</a>
				<div class="menu-sep"></div>
				<div class="menu-pref">
					<span>{i18n.t('lang.label')}</span>
					<div class="pill-group" role="group" aria-label={i18n.t('lang.label')}>
						<button type="button" aria-pressed={i18n.locale === 'ro'} onclick={() => i18n.set('ro')}>RO</button>
						<button type="button" aria-pressed={i18n.locale === 'en'} onclick={() => i18n.set('en')}>EN</button>
					</div>
				</div>
				<div class="menu-pref">
					<span>{i18n.t('theme.label')}</span>
					<div class="pill-group" role="group" aria-label={i18n.t('theme.label')}>
						<button type="button" aria-label={i18n.t('theme.light')} aria-pressed={!isDark} onclick={() => theme.set('light')}>
							<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="4.2" /><path d="M12 3 v2 M12 19 v2 M4.2 4.2 l1.4 1.4 M18.4 18.4 l1.4 1.4 M3 12 h2 M19 12 h2 M4.2 19.8 l1.4-1.4 M18.4 5.6 l1.4-1.4" /></svg>
						</button>
						<button type="button" aria-label={i18n.t('theme.dark')} aria-pressed={isDark} onclick={() => theme.set('dark')}>
							<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 14.5 A8 8 0 1 1 9.5 4 A6.2 6.2 0 0 0 20 14.5 Z" /></svg>
						</button>
					</div>
				</div>
				<div class="menu-sep"></div>
				<button type="button" class="menu-item" onclick={signOut}>{i18n.t('nav.signOut')}</button>
				<button type="button" class="menu-item" onclick={signOutEverywhere}>{i18n.t('nav.signOutEverywhere')}</button>
			</div>
		{/if}
	</div>
{/if}

<style>
	.user-chip {
		position: relative;
		flex: none;
	}

	.chip-btn {
		appearance: none;
		background: none;
		display: flex;
		align-items: center;
		gap: 0.55rem;
		min-height: 44px;
		padding: 0 0.5rem 0 0.3rem;
		border: none;
		border-radius: 999px;
		font: inherit;
		font-size: 0.9rem;
		color: var(--text);
		cursor: pointer;
		transition: background 0.15s var(--ease);
	}

	.compact .chip-btn {
		min-height: 32px;
		gap: 0.45rem;
		padding: 0 0.45rem 0 0.2rem;
		font-size: 0.85rem;
	}

	.compact .chip-btn .avatar {
		width: 24px;
		height: 24px;
		font-size: 0.68rem;
	}

	.chip-btn:hover,
	.chip-btn[aria-expanded='true'] {
		background: var(--fill-2);
	}

	.chip-btn .chev {
		color: var(--text-muted);
	}

	.email {
		max-width: 220px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	@media (max-width: 1100px) {
		.email {
			display: none;
		}
	}

	.narrow-only {
		display: none;
	}

	@media (max-width: 799px) {
		.narrow-only {
			display: flex;
		}
	}

	.menu {
		position: absolute;
		top: calc(100% + 8px);
		right: 0;
		z-index: 40;
		width: 270px;
		display: flex;
		flex-direction: column;
		padding: 6px;
		background: var(--surface);
		border-radius: var(--radius);
		box-shadow: var(--shadow-card);
		animation: menu-in 0.16s var(--ease);
	}

	@keyframes menu-in {
		from {
			opacity: 0;
			transform: translateY(-4px);
		}
	}

	.menu-who {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 10px 10px;
		font-size: 0.85rem;
		color: var(--text-muted);
		overflow-wrap: anywhere;
	}

	.menu-sep {
		height: 1px;
		margin: 4px 6px;
		background: var(--line);
	}

	.menu-item {
		appearance: none;
		background: none;
		border: none;
		display: flex;
		align-items: center;
		min-height: 40px;
		padding: 0 10px;
		border-radius: var(--radius-sm);
		font: inherit;
		font-size: 0.9rem;
		color: var(--text);
		text-align: left;
		cursor: pointer;
	}

	.menu-item:hover,
	.menu-item:focus-visible {
		background: var(--fill);
		color: var(--text);
		outline: none;
	}

	.menu-pref {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		min-height: 44px;
		padding: 0 4px 0 10px;
		font-size: 0.9rem;
		color: var(--text-muted);
	}

	.menu-pref .pill-group button svg {
		display: block;
	}
</style>
