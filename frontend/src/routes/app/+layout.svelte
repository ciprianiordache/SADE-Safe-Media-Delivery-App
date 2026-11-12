<script lang="ts">
	import { goto } from '$app/navigation';
	import { i18n } from '$lib/i18n.svelte';
	import { auth, brand } from '$lib/stores.svelte';
	import { page } from '$app/state';
	import logo from '$lib/assets/logo.png';
	import { IS_APP } from '$lib/platform';
	import AccountMenu from '$lib/components/AccountMenu.svelte';
	import WindowControls from '$lib/components/WindowControls.svelte';
		import { onMount } from 'svelte';
	import { appTitle } from '$lib/apptitle.svelte';
	import { UNAUTHORIZED_EVENT } from '$lib/api';

	let { children } = $props();

	onMount(() => {
		auth.ensureLoaded().then(async () => {
			if (!auth.user) {
				goto('/login');
				return;
			}
			// A new account sets up its brand first (skippable).
			const b = await brand.load();
			if (b && !b.onboarded && page.url.pathname !== '/app/welcome') goto('/app/welcome');
		});
		const onUnauthorized = () => {
			auth.user = null;
			goto('/login');
		};
		window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
		return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
	});

	// The dashboard waits for the brand (or its failure) before it renders:
	// a brand-new account goes straight to the welcome page instead of
	// glimpsing an empty dashboard first. Other /app pages render at once.
	let holdDashboard = $derived(
		page.url.pathname === '/app' &&
			(brand.status === 'idle' ||
				brand.status === 'loading' ||
				(brand.status === 'ready' && !!brand.value && !brand.value.onboarded))
	);

	let path = $derived(page.url.pathname);
	let onDashboard = $derived(path === '/app');
	let onDeliveries = $derived(path === '/app' || path.startsWith('/app/jobs'));
	let onSettings = $derived(path.startsWith('/app/settings'));
			let onWelcome = $derived(path === '/app/welcome');
	let onFile = $derived(path.startsWith('/app/jobs/'));

	// A program's shortcuts: Ctrl/Cmd+N a new file, Ctrl/Cmd+, the settings.
	function onAppKey(e: KeyboardEvent) {
		if (!IS_APP || !(e.ctrlKey || e.metaKey) || e.altKey || e.shiftKey || !auth.user) return;
		if (e.key === 'n' || e.key === 'N') {
			e.preventDefault();
			goto('/app?upload=1');
		} else if (e.key === ',') {
			e.preventDefault();
			goto('/app/settings');
		}
	}
</script>

<svelte:window onkeydown={onAppKey} />

{#if auth.loading}
	<div class="container">
		<p class="muted loading">{i18n.t('common.loading')}</p>
	</div>
{:else if auth.user && IS_APP}
	<!-- The installed app: a frameless window. The dashboard draws its own
	     title bar (the list window's bar); every other page gets this one. -->
	<div class="app-shell">
		{#if !onDashboard}
			<header class="titlebar" data-tauri-drag-region>
				<WindowControls />
				{#if !onWelcome}
					<a class="tb-back" href="/app">
						<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 6 L9 12 L15 18" /></svg>
						{i18n.t('nav.deliveries')}
					</a>
				{/if}
								<span class="tb-mark" data-tauri-drag-region>
					{#if onWelcome}<img src={logo} alt="" width="16" height="16" />{/if}
										<span class="tb-name">{onSettings ? i18n.t('nav.settings') : appTitle.value || 'SADE'}</span>
				</span>
				<span class="sp" data-tauri-drag-region></span>
								<AccountMenu compact alwaysNav />
				<WindowControls side="right" />
			</header>
		{/if}
		<div class="app-body" class:full={onDashboard || onFile}>
			{#if holdDashboard}
				<div class="hold" aria-busy="true" aria-label={i18n.t('common.loading')}>
					<span class="sk sk-row"></span>
					<span class="sk sk-row"></span>
				</div>
			{:else}
				{@render children()}
			{/if}
		</div>
	</div>
{:else if auth.user}
	<header class="top-bar">
		<div class="container top-bar-inner">
			<a class="mark" href="/app">
				<img src={logo} alt="" width="28" height="28" />
				<span>SADE</span>
			</a>
			<nav class="nav-links" aria-label={i18n.t('nav.main')}>
				<a class="nav-link" href="/app" aria-current={onDeliveries ? 'page' : undefined}>{i18n.t('nav.deliveries')}</a>
				<a class="nav-link" href="/app/settings" aria-current={onSettings ? 'page' : undefined}>{i18n.t('nav.settings')}</a>
			</nav>
			<div class="top-bar-right">
				{#if !onWelcome}
					<a class="btn new-file" href="/app?upload=1" aria-label={i18n.t('dashboard.jumpUpload')}>
						<span class="nf-text">{i18n.t('dashboard.jumpUpload')}</span>
						<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><path d="M12 5 V19 M5 12 H19" /></svg>
					</a>
				{/if}
				<AccountMenu />
			</div>
		</div>
	</header>
	{#if holdDashboard}
		<div class="container hold" aria-busy="true" aria-label={i18n.t('common.loading')}>
			<span class="sk sk-title"></span>
			<span class="sk sk-row"></span>
			<span class="sk sk-row"></span>
		</div>
	{:else}
		{@render children()}
	{/if}
{/if}

<style>
	.hold {
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
		padding-top: 48px;
	}

	.sk-title {
		width: 40%;
		height: 3rem;
	}

	.sk-row {
		width: 100%;
		height: 3.2rem;
	}

	.loading {
		padding: 2rem 0;
	}

	/* ---- the installed app's window ---------------------------------- */
		.app-shell {
		height: 100%;
		display: flex;
		flex-direction: column;
		background: var(--bg);
	}

	.titlebar {
		flex: none;
		display: flex;
		align-items: center;
		gap: 14px;
		height: 44px;
				padding: 0 10px 0 14px;
		--bar-pad-right: 10px;
		background: var(--fill);
		border-bottom: 1px solid var(--line);
		font-size: 0.85rem;
		color: var(--text-muted);
		user-select: none;
	}

	.tb-back {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		height: 30px;
		padding: 0 10px 0 6px;
		border-radius: 7px;
		color: var(--text);
		font-weight: 500;
	}

	.tb-back:hover {
		color: var(--text);
		background: var(--fill-2);
	}

		.tb-mark {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
		color: var(--text-muted);
	}

	.tb-name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--text);
		font-weight: 500;
	}

	.titlebar .sp {
		flex: 1;
		align-self: stretch;
	}

	.app-body {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
	}

	.app-body.full {
		overflow: hidden;
	}

	.app-body .hold {
		padding: 24px;
	}

	/* ---- the web header: logo left, links centred, actions right ------ */
	.top-bar {
		position: sticky;
		top: 0;
		z-index: 30;
		height: 72px;
		background: color-mix(in oklab, var(--bg) 88%, transparent);
		backdrop-filter: saturate(1.4) blur(12px);
		-webkit-backdrop-filter: saturate(1.4) blur(12px);
		border-bottom: 1px solid var(--line);
		display: flex;
		align-items: center;
	}

	@media (max-width: 639px) {
		.top-bar {
			height: 60px;
		}
	}

	.top-bar-inner {
		display: grid;
		grid-template-columns: 1fr auto 1fr;
		align-items: center;
		width: 100%;
		gap: 16px;
	}

	.mark {
		justify-self: start;
		display: flex;
		align-items: center;
		gap: 0.55rem;
		min-height: 44px;
		color: var(--text);
		font-weight: 500;
		font-size: 22px;
		line-height: 1;
		letter-spacing: -0.02em;
	}

	.mark:hover {
		color: var(--text);
	}

	.mark img {
		display: block;
	}

	.nav-links {
		display: flex;
		align-items: center;
		gap: 4px;
	}

	.nav-link {
		position: relative;
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		padding: 0 14px;
		font-size: 15px;
		color: var(--text);
		border-radius: 999px;
		transition: background 0.15s var(--ease);
	}

	.nav-link:hover {
		color: var(--text);
		background: var(--fill-2);
	}

	.nav-link[aria-current='page'] {
		font-weight: 500;
	}

	.nav-link[aria-current='page']::after {
		content: '';
		position: absolute;
		left: 14px;
		right: 14px;
		bottom: 6px;
		height: 2px;
		border-radius: 2px;
		background: var(--accent-bright);
	}

	.top-bar-right {
		justify-self: end;
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.new-file svg {
		margin-right: -2px;
	}

	@media (max-width: 799px) {
		.top-bar-inner {
			grid-template-columns: 1fr auto;
		}
		.nav-links {
			display: none;
		}
	}

	@media (max-width: 559px) {
		.new-file {
			width: 44px;
			padding: 0;
		}
		.nf-text {
			display: none;
		}
		.new-file svg {
			margin: 0;
			width: 16px;
			height: 16px;
		}
	}
</style>
