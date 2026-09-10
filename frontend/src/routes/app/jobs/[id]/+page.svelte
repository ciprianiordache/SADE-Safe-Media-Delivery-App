<script lang="ts">
	import { page } from '$app/state';
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import type { Job } from '$lib/types';
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
</script>

<div class="container">
	<a class="back" href="/app">&larr; {i18n.t('job.back')}</a>

	{#if notFound}
		<p class="error-text">{i18n.t('job.notFound')}</p>
	{:else if !job}
		<p class="muted">{i18n.t('common.loading')}</p>
	{:else}
		<h1>{i18n.t('job.title')}</h1>
		<div class="card">
			<dl>
				<dt>{i18n.t('job.status')}</dt>
				<dd><span class={`badge badge-${job.status}`}>{i18n.t(`status.${job.status}`)}</span></dd>

				<dt>{i18n.t('job.mediaType')}</dt>
				<dd>{job.mediaType}</dd>

				<dt>{i18n.t('job.recipient')}</dt>
				<dd>{job.recipientEmail}</dd>

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
		</div>

		<h2>{i18n.t('job.assets')}</h2>
		<div class="card">
			{#if !job.assets || job.assets.length === 0}
				<p class="muted">{i18n.t('job.assetsEmpty')}</p>
			{:else}
				<table>
					<tbody>
						{#each job.assets as asset (asset.id)}
							<tr>
								<td>{asset.kind}</td>
								<td>{asset.filename}</td>
								<td>{(asset.sizeBytes / (1024 * 1024)).toFixed(2)} MiB</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
			<p class="muted note">{i18n.t('job.assetsNote')}</p>
		</div>
	{/if}
</div>

<style>
	.back {
		display: inline-block;
		margin: 1rem 0;
		text-decoration: none;
	}

	dl {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.5rem 1.5rem;
		margin: 0;
	}

	dt {
		color: var(--text-muted);
		font-size: 0.85rem;
		font-weight: 600;
	}

	dd {
		margin: 0;
	}

	.note {
		margin-top: 1rem;
		font-size: 0.85rem;
	}

	h2 {
		margin-top: 2rem;
	}
</style>
