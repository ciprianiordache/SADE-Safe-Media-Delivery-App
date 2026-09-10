<script lang="ts">
	import { page } from '$app/state';
	import { i18n, type MessageKey } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import type { Job } from '$lib/types';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { onDestroy, onMount } from 'svelte';

	let job = $state<Job | null>(null);
	let notFound = $state(false);
	let pollHandle: ReturnType<typeof setInterval> | undefined;

	async function load() {
		try {
			job = await api.getJob(page.params.id as string);
		} catch (err) {
			if (err instanceof ApiError && err.status === 404) notFound = true;
		}
	}

	onMount(() => {
		load();
		pollHandle = setInterval(() => {
			if (job && (job.status === 'pending' || job.status === 'processing')) load();
		}, 4000);
	});

	onDestroy(() => {
		if (pollHandle) clearInterval(pollHandle);
	});

	// Stepper: pending/processing haven't reached a step yet; done completes
	// all three; failed marks the step it died on (always "watermarking" -
	// the worker only fails mid-processing, never at queue or delivery).
	const STEPS = ['queued', 'watermarking', 'delivered'] as const;
	const STEP_KEYS: Record<(typeof STEPS)[number], MessageKey> = {
		queued: 'job.stepQueued',
		watermarking: 'job.stepWatermarking',
		delivered: 'job.stepDelivered'
	};
	let stepIndex = $derived.by(() => {
		if (!job) return -1;
		switch (job.status) {
			case 'pending':
				return 0;
			case 'processing':
				return 1;
			case 'done':
				return 3;
			case 'failed':
				return 1;
		}
	});
</script>

<div class="container page">
	<a class="back" href="/app">
		<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
			><path d="M15 6 L9 12 L15 18" /></svg
		>
		{i18n.t('job.back')}
	</a>

	{#if notFound}
		<p class="error-text">{i18n.t('job.notFound')}</p>
	{:else if !job}
		<p class="muted">{i18n.t('common.loading')}</p>
	{:else}
		<div class="head">
			<div class="head-main">
				<span class="recipient mono">{job.recipientEmail}</span>
				<span class="faint meta">{job.mediaType} · {job.watermarkKind}</span>
			</div>
			<StatusBadge status={job.status} />
		</div>

		{#if job.status !== 'failed'}
			<div class="stepper card">
				{#each STEPS as step, i}
					<div class="step">
						<span class="step-dot" class:done={i < stepIndex} class:current={i === stepIndex}>
							{#if i < stepIndex}
								<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"
									><path d="M5 12.5 L10 17.5 L19 7" /></svg
								>
							{/if}
						</span>
						<span class:step-label-done={i < stepIndex}>{i18n.t(STEP_KEYS[step])}</span>
					</div>
					{#if i < STEPS.length - 1}
						<div class="step-line" class:done={i < stepIndex}></div>
					{/if}
				{/each}
			</div>
		{/if}

		<div class="detail-grid">
			<section class="card">
				<span class="section-label">{i18n.t('job.title')}</span>
				<dl>
					<dt>{i18n.t('job.mediaType')}</dt>
					<dd>{job.mediaType}</dd>

					<dt>{i18n.t('job.watermarkKind')}</dt>
					<dd>{i18n.t(`dashboard.watermarkKind.${job.watermarkKind}`)}</dd>

					{#if job.watermarkText}
						<dt>{i18n.t('job.watermarkText')}</dt>
						<dd>{job.watermarkText}</dd>
					{/if}

					<dt>{i18n.t('job.attempts')}</dt>
					<dd>{job.attempts}</dd>

					{#if job.error}
						<dt>{i18n.t('job.error')}</dt>
						<dd class="error-text">{job.error}</dd>
					{/if}

					<dt>{i18n.t('job.createdAt')}</dt>
					<dd>{new Date(job.createdAt).toLocaleString(i18n.locale)}</dd>

					<dt>{i18n.t('job.updatedAt')}</dt>
					<dd>{new Date(job.updatedAt).toLocaleString(i18n.locale)}</dd>
				</dl>
			</section>

			<section class="card">
				<span class="section-label">{i18n.t('job.assets')}</span>
				{#if !job.assets || job.assets.length === 0}
					<p class="muted">{i18n.t('job.assetsEmpty')}</p>
				{:else}
					<div class="asset-list">
						{#each job.assets as asset (asset.id)}
							<div class="asset-row">
								<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--text-faint)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"
									><path
										d="M14 3 H7 a2 2 0 0 0 -2 2 v14 a2 2 0 0 0 2 2 h10 a2 2 0 0 0 2 -2 V8 Z M14 3 v5 h5"
									/></svg
								>
								<div class="asset-meta">
									<span class="bold">{asset.kind}</span>
									<span class="faint mono">{asset.filename} · {(asset.sizeBytes / (1024 * 1024)).toFixed(2)} MB</span>
								</div>
							</div>
						{/each}
					</div>
				{/if}
				<p class="faint note">{i18n.t('job.assetsNote')}</p>
			</section>
		</div>
	{/if}
</div>

<style>
	.page {
		padding: 1.5rem 1.75rem 3rem;
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
	}

	.back {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		color: var(--text-muted);
		font-size: 0.85rem;
	}

	.head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}

	.head-main {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.recipient {
		font-size: 1.4rem;
		font-weight: 700;
	}

	.meta {
		font-size: 0.85rem;
	}

	.stepper {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 1rem 1.25rem;
	}

	.step {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		font-size: 0.85rem;
		color: var(--text-muted);
	}

	.step-label-done {
		color: var(--text);
	}

	.step-dot {
		width: 22px;
		height: 22px;
		border-radius: 999px;
		background: var(--border-strong);
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
	}

	.step-dot.done {
		background: var(--success);
	}

	.step-dot.current {
		background: var(--accent);
	}

	.step-line {
		flex: 1;
		height: 2px;
		background: var(--border-strong);
		border-radius: 2px;
	}

	.step-line.done {
		background: var(--success);
	}

	.detail-grid {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 300px;
		gap: 1.25rem;
		align-items: start;
	}

	@media (max-width: 760px) {
		.detail-grid {
			grid-template-columns: 1fr;
		}
	}

	.detail-grid .card {
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
	}

	.section-label {
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		color: var(--text-faint);
		text-transform: uppercase;
	}

	dl {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.5rem 1.25rem;
		margin: 0;
	}

	dt {
		color: var(--text-faint);
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	dd {
		margin: 0;
		font-size: 0.9rem;
	}

	.asset-list {
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
	}

	.asset-row {
		display: flex;
		gap: 0.6rem;
		align-items: flex-start;
	}

	.asset-meta {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
		font-size: 0.85rem;
	}

	.note {
		font-size: 0.78rem;
		line-height: 1.5;
		margin: 0;
	}
</style>
