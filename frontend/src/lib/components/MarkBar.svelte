<script lang="ts">
	// The header of the pages with no operator session (login, confirm,
	// revoke, the recipient's preview): the marketing page's bar, with the
	// language and theme a visitor may need.
	import logo from '$lib/assets/logo.png';
	import { i18n } from '$lib/i18n.svelte';
	import LangThemeToggle from './LangThemeToggle.svelte';
	import { IS_APP } from '$lib/platform';
	import WindowControls from './WindowControls.svelte';

	let { siteLink = true }: { siteLink?: boolean } = $props();
</script>

<header class="mark-bar" class:app={IS_APP} data-tauri-drag-region={IS_APP ? true : undefined}>
	<div class="container mark-bar-inner" data-tauri-drag-region={IS_APP ? true : undefined}>
		{#if IS_APP}<WindowControls />{/if}
		<!-- "/" is the static marketing page, outside the SvelteKit router -->
		<a class="mark" href={IS_APP ? '/app' : '/'} data-sveltekit-reload={IS_APP ? undefined : true}>
			<img src={logo} alt="" width="28" height="28" />
			<span>SADE</span>
		</a>
		<div class="right">
			{#if siteLink && !IS_APP}
				<a class="site-link" href="/" data-sveltekit-reload>{i18n.t('nav.backToSite')}</a>
			{/if}
						<LangThemeToggle />
		</div>
		{#if IS_APP}<WindowControls side="right" />{/if}
	</div>
</header>

<style>
	.mark-bar {
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
		.mark-bar {
			height: 60px;
		}
	}

	/* The installed app's title bar on the signed-out pages: the window has no
	   frame, so this bar carries its controls and drags it. */
	.mark-bar.app {
		position: static;
		height: 44px;
		background: var(--fill);
		backdrop-filter: none;
		user-select: none;
	}

		.mark-bar.app .container {
		max-width: none;
		height: 100%;
		padding: 0 10px 0 14px;
		--bar-pad-right: 10px;
		justify-content: flex-start;
		gap: 14px;
	}

	.mark-bar.app .mark span {
		font-size: 15px;
	}

	.mark-bar.app .mark img {
		width: 18px;
		height: 18px;
	}

	.mark-bar.app .right {
		margin-left: auto;
	}

	.mark-bar-inner {
		width: 100%;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
	}

	.mark {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		min-height: 44px;
		color: var(--text);
	}

	.mark:hover {
		color: var(--text);
	}

	.mark img {
		display: block;
	}

	.mark span {
		font-size: 22px;
		font-weight: 500;
		line-height: 1;
		letter-spacing: -0.02em;
	}

	.right {
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.site-link {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		padding: 0 12px;
		border-radius: 999px;
		font-size: 15px;
		color: var(--text);
		transition: background 0.15s var(--ease);
	}

	.site-link:hover {
		color: var(--text);
		background: var(--fill-2);
	}

	@media (max-width: 519px) {
		.site-link {
			display: none;
		}
	}
</style>
