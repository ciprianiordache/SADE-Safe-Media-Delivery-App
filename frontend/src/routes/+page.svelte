<script lang="ts">
	import { i18n } from '$lib/i18n.svelte';
	import { auth } from '$lib/stores.svelte';
	import { onMount } from 'svelte';

	onMount(() => {
		auth.ensureLoaded();
	});
</script>

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
	.hero {
		padding: 4rem 1.5rem;
		max-width: 640px;
	}

	h1 {
		font-size: 2.2rem;
		margin-bottom: 0.75rem;
	}

	p {
		font-size: 1.05rem;
		line-height: 1.5;
		margin-bottom: 1.75rem;
	}
</style>
