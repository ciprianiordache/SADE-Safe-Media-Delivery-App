<script lang="ts">
	import { page } from '$app/state';
	import { i18n } from '$lib/i18n.svelte';
	import { IS_APP } from '$lib/platform';

	let { lead = '' }: { lead?: string } = $props();

	const TABS = [
		{ href: '/app/settings', key: 'settings.tabBrand' },
		{ href: '/app/settings/security', key: 'settings.tabSecurity' }
	] as const;
</script>

{#if IS_APP}
	<header class="pref-head">
		<h1>{i18n.t(page.url.pathname === '/app/settings/security' ? 'settings.tabSecurity' : 'settings.tabBrand')}</h1>
		{#if lead}<p class="muted">{lead}</p>{/if}
	</header>
{:else}
<div class="page-head">
	<div>
		<h1>{i18n.t('settings.title')}</h1>
		{#if lead}<p class="lead">{lead}</p>{/if}
	</div>
	<nav class="segmented tabs" aria-label={i18n.t('settings.title')}>
		{#each TABS as t (t.href)}
			<a href={t.href} aria-current={page.url.pathname === t.href ? 'page' : undefined}>
				{i18n.t(t.key)}
			</a>
		{/each}
	</nav>
</div>
{/if}

<style>
	/* The app's preferences pane: a window title, not a web page head. */
	.pref-head {
		margin-bottom: 20px;
	}

	.pref-head h1 {
		margin: 0;
		font-size: 1.45rem;
		letter-spacing: -0.02em;
	}

	.pref-head p {
		margin: 4px 0 0;
		font-size: 0.92rem;
	}

	.tabs {
		align-self: end;
	}

	/* Links styled as the segmented control's buttons. */
	.tabs a {
		position: relative;
		flex: 1;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 38px;
		padding: 0 1.1rem;
		border-radius: 999px;
		font-size: 0.92rem;
		font-weight: 500;
		color: var(--text-muted);
		white-space: nowrap;
		transition:
			background 0.18s var(--ease),
			color 0.18s var(--ease),
			box-shadow 0.18s var(--ease);
	}

	.tabs a::after {
		content: '';
		position: absolute;
		inset: -3px 0;
	}

	.tabs a:hover {
		color: var(--text);
	}

	.tabs a[aria-current='page'] {
		background: var(--surface);
		color: var(--text);
		box-shadow: var(--shadow-sm);
	}

	@media (max-width: 639px) {
		.tabs {
			justify-self: stretch;
		}
	}
</style>
