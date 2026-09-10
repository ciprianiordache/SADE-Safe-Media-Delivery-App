<script lang="ts">
	import { i18n } from '$lib/i18n.svelte';
	import { auth } from '$lib/stores.svelte';
	import MarkBar from '$lib/components/MarkBar.svelte';
	import { onMount } from 'svelte';

	onMount(() => {
		auth.ensureLoaded();
	});
</script>

<div class="container top">
	<MarkBar />
</div>

<div class="container hero">
	<h1>{i18n.t('landing.title')}</h1>
	<p class="muted">{i18n.t('landing.subtitle')}</p>
	{#if auth.user}
		<a class="btn" href="/app">{i18n.t('landing.ctaApp')}</a>
	{:else}
		<a class="btn" href="/login">{i18n.t('landing.cta')}</a>
	{/if}
</div>

<style>
	.top {
		padding: 1.25rem 1.75rem 0;
	}

	.hero {
		padding: 4.5rem 1.75rem;
		max-width: 640px;
	}

	h1 {
		font-size: 2.3rem;
		letter-spacing: -0.01em;
		margin-bottom: 0.85rem;
	}

	p {
		font-size: 1.05rem;
		line-height: 1.55;
		margin-bottom: 1.85rem;
	}
</style>
