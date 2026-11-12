<script lang="ts">
	// Settings in the installed app is a preferences window: a fixed list of
	// sections on the left, the chosen one scrolling on the right. On the web
	// the pages keep their own head and tabs (SettingsTabs).
	import { page } from '$app/state';
	import { i18n } from '$lib/i18n.svelte';
	import { IS_APP } from '$lib/platform';

	let { children } = $props();

	const SECTIONS = [
		{ href: '/app/settings', key: 'settings.tabBrand', icon: 'brand' },
		{ href: '/app/settings/security', key: 'settings.tabSecurity', icon: 'lock' }
	] as const;
</script>

{#if IS_APP}
	<div class="prefs">
		<nav class="prefs-side" aria-label={i18n.t('settings.title')}>
			{#each SECTIONS as s (s.href)}
				<a class="side-item" href={s.href} aria-current={page.url.pathname === s.href ? 'page' : undefined}>
					{#if s.icon === 'brand'}
						<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3 L20 6 V12 C20 16.5 16.5 20 12 21 C7.5 20 4 16.5 4 12 V6 Z" /></svg>
					{:else}
						<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="11" width="14" height="9" rx="1.5" /><path d="M8 11 V8 a4 4 0 0 1 8 0 v3" /></svg>
					{/if}
					<span>{i18n.t(s.key)}</span>
				</a>
			{/each}
		</nav>
		<div class="prefs-main">
			{@render children()}
		</div>
	</div>
{:else}
	{@render children()}
{/if}

<style>
	.prefs {
		height: 100%;
		display: grid;
		grid-template-columns: 220px minmax(0, 1fr);
	}

	.prefs-side {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 14px 10px;
		border-right: 1px solid var(--line);
		background: var(--fill);
		overflow-y: auto;
	}

	.side-item[aria-current='page'] {
		background: var(--fill-2);
		color: var(--text);
		font-weight: 500;
	}

	.prefs-main {
		min-width: 0;
		overflow-y: auto;
		overscroll-behavior: none;
	}

	/* The pages' web padding and sticky offsets, sized for a window. */
	.prefs-main :global(.page) {
		max-width: 1180px;
		margin: 0;
		padding: 22px 28px 40px;
	}

		.prefs-main :global(.layout > .preview) {
		top: 0;
	}

	/* A preferences window, not a web page: groups sit flat on the window,
	   a hairline instead of a floating card. */
	.prefs-main :global(.win) {
		box-shadow: none;
		border: 1px solid var(--line);
		border-radius: 10px;
	}

	@media (max-width: 719px) {
		.prefs {
			grid-template-columns: minmax(0, 1fr);
			grid-template-rows: auto minmax(0, 1fr);
		}
		.prefs-side {
			flex-direction: row;
			border-right: none;
			border-bottom: 1px solid var(--line);
			padding: 8px 10px;
		}
		.prefs-side .side-item {
			width: auto;
		}
		.prefs-main :global(.page) {
			padding: 16px 14px 32px;
		}
	}
</style>
