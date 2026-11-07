<script lang="ts">
	import { page } from '$app/state';
	import { i18n } from '$lib/i18n.svelte';

	const TABS = [
		{ href: '/app/settings', key: 'settings.tabBrand' },
		{ href: '/app/settings/security', key: 'settings.tabSecurity' }
	] as const;
</script>

<div class="head">
	<a class="back" href="/app">
		<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
			><path d="M15 6 L9 12 L15 18" /></svg
		>
		{i18n.t('job.back')}
	</a>
	<h1>{i18n.t('settings.title')}</h1>
	<nav class="tabs">
		{#each TABS as t (t.href)}
			<a href={t.href} class:active={page.url.pathname === t.href} aria-current={page.url.pathname === t.href ? 'page' : undefined}>
				{i18n.t(t.key)}
			</a>
		{/each}
	</nav>
</div>

<style>
	.head {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.back {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		color: var(--text-muted);
		font-size: 0.85rem;
		align-self: flex-start;
	}

	h1 {
		margin: 0;
		font-size: 1.55rem;
		font-weight: 700;
	}

	.tabs {
		display: flex;
		gap: 0.25rem;
		border-bottom: 1px solid var(--border);
	}

	.tabs a {
		padding: 0.55rem 0.85rem;
		margin-bottom: -1px;
		border-bottom: 2px solid transparent;
		color: var(--text-muted);
		font-size: 0.88rem;
		font-weight: 600;
	}

	.tabs a:hover {
		color: var(--text);
	}

	.tabs a.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}
</style>
