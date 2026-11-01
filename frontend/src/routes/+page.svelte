<script lang="ts">
	import { i18n, type MessageKey } from '$lib/i18n.svelte';
	import { auth } from '$lib/stores.svelte';
	import MarkBar from '$lib/components/MarkBar.svelte';
	import { onMount } from 'svelte';

	onMount(() => {
		auth.ensureLoaded();
	});

	const STEPS: { title: MessageKey; body: MessageKey }[] = [
		{ title: 'landing.step1Title', body: 'landing.step1Body' },
		{ title: 'landing.step2Title', body: 'landing.step2Body' },
		{ title: 'landing.step3Title', body: 'landing.step3Body' }
	];
</script>

<svelte:head>
	<title>SADE · {i18n.t('app.tagline')}</title>
</svelte:head>

<div class="container top">
	<MarkBar />
</div>

<div class="container hero">
	<h1>{i18n.t('landing.title')}</h1>
	<p class="muted lead">{i18n.t('landing.subtitle')}</p>
	{#if auth.user}
		<a class="btn" href="/app">{i18n.t('landing.ctaApp')}</a>
	{:else}
		<a class="btn" href="/login">{i18n.t('landing.cta')}</a>
	{/if}
</div>

<div class="container how">
	<span class="section-label">{i18n.t('landing.how')}</span>
	<ol class="steps">
		{#each STEPS as step, i (step.title)}
			<li class="card step">
				<span class="num mono">{i + 1}</span>
				<strong>{i18n.t(step.title)}</strong>
				<span class="muted">{i18n.t(step.body)}</span>
			</li>
		{/each}
	</ol>
</div>

<style>
	.top {
		padding: 1.25rem 1.75rem 0;
	}

	.hero {
		padding: 4.5rem 1.75rem 3rem;
		max-width: 680px;
		margin-left: max(0px, calc((100% - 1160px) / 2));
	}

	h1 {
		font-size: 2.3rem;
		letter-spacing: -0.01em;
		margin: 0 0 0.85rem;
		line-height: 1.15;
	}

	.lead {
		font-size: 1.05rem;
		line-height: 1.55;
		margin: 0 0 1.85rem;
	}

	@media (max-width: 520px) {
		.top {
			padding: 1rem 1rem 0;
		}
		.hero {
			padding: 2.75rem 1rem 2rem;
		}
		h1 {
			font-size: 1.8rem;
		}
	}

	.how {
		padding-bottom: 4rem;
	}

	.section-label {
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		color: var(--text-faint);
		text-transform: uppercase;
	}

	.steps {
		list-style: none;
		padding: 0;
		margin: 0.9rem 0 0;
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 1rem;
	}

	.step {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		font-size: 0.88rem;
		line-height: 1.5;
	}

	.num {
		width: 28px;
		height: 28px;
		border-radius: 8px;
		background: var(--accent-tint);
		color: var(--accent-tint-text);
		font-weight: 600;
		font-size: 0.85rem;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		margin-bottom: 0.3rem;
	}

	strong {
		font-size: 0.95rem;
	}
</style>
