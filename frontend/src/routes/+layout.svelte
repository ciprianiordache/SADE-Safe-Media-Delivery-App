<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { theme } from '$lib/theme.svelte';
	import { i18n } from '$lib/i18n.svelte';
	import { onMount } from 'svelte';

	let { children } = $props();

	onMount(() => {
		theme.init();
		i18n.init();
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>{i18n.t('app.name')}</title>
</svelte:head>

<div class="top-bar">
	<div class="container top-bar-inner">
		<a class="brand" href="/">{i18n.t('app.name')}</a>
		<div class="top-bar-controls">
			<select
				aria-label="Language"
				value={i18n.locale}
				onchange={(e) => i18n.set(e.currentTarget.value as 'ro' | 'en')}
			>
				<option value="ro">RO</option>
				<option value="en">EN</option>
			</select>
			<select
				aria-label="Theme"
				value={theme.value}
				onchange={(e) => theme.set(e.currentTarget.value as 'light' | 'dark' | 'system')}
			>
				<option value="system">Auto</option>
				<option value="light">☀️</option>
				<option value="dark">🌙</option>
			</select>
		</div>
	</div>
</div>

{@render children()}

<style>
	.top-bar {
		border-bottom: 1px solid var(--border);
		background: var(--surface);
	}

	.top-bar-inner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.9rem 1.5rem;
	}

	.brand {
		font-weight: 700;
		font-size: 1.1rem;
		color: var(--text);
		text-decoration: none;
	}

	.top-bar-controls {
		display: flex;
		gap: 0.5rem;
	}

	.top-bar-controls select {
		width: auto;
		padding: 0.3rem 0.5rem;
		font-size: 0.85rem;
	}
</style>
