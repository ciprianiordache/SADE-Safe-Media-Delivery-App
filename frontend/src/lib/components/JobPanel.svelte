<script lang="ts">
	import { authedSrc, webSrc } from '$lib/media';
	// The open delivery beside the list, as the marketing page's "Lista de
	// livrări" shows it: the file and who it went to, the picture, the pills
	// and the history. The full job page (/app/jobs/[id]) keeps the details
	// and delete; this panel links to it.
	import { i18n } from '$lib/i18n.svelte';
	import { api } from '$lib/api';
	import type { Job, JobStatus } from '$lib/types';
	import { fullDate } from '$lib/format';
	import { deliveryState, stateKey, type DeliveryState } from '$lib/delivery';
	import DeliveryCard from './DeliveryCard.svelte';

	let { jobId, status, onclose }: { jobId: string; status: JobStatus; onclose?: () => void } = $props();

	let job = $state<Job | null>(null);
	let failed = $state(false);

	async function load(id: string) {
		failed = false;
		try {
			const j = await api.getJob(id);
			if (id === jobId) job = j;
		} catch {
			if (id === jobId) failed = true;
		}
	}

	// A new selection loads at once; the list's own poll tells us when this
	// job's status moves on (the preview appears when it is done).
	let loadedFor = '';
	$effect(() => {
		const key = `${jobId}:${status}`;
		if (key === loadedFor) return;
		if (!loadedFor.startsWith(jobId + ':')) job = null;
		loadedFor = key;
		load(jobId);
	});

	let preview = $derived(job?.assets?.find((a) => a.kind === 'preview'));
	let original = $derived(job?.assets?.find((a) => a.kind === 'original'));
	let shown = $derived(preview ?? original);
	let title = $derived(job ? (original?.filename ?? job.fileName ?? job.recipientEmail) : '');
	let st = $derived<DeliveryState | null>(job ? deliveryState(job) : null);
	const CHIP: Record<DeliveryState, string> = {
		wait: 'chip',
		work: 'chip chip-accent',
		failed: 'chip chip-bad',
		off: 'chip chip-bad',
		paid: 'chip chip-ok',
		opened: 'chip',
		sent: 'chip'
	};
</script>

<div class="panel">
	<!-- A screen reader hears the state of the open delivery, not the whole
	     pane on every refresh. -->
	<p class="sr-only" aria-live="polite">{st ? i18n.t(stateKey(st)) : ''}</p>
	{#if failed}
		<p class="error-banner" role="alert">
			{i18n.t('job.notFound')}
		</p>
	{:else if !job}
		<div class="loading" aria-busy="true" aria-label={i18n.t('common.loading')}>
			<span class="sk" style="height:1.4rem;width:60%"></span>
			<span class="sk" style="height:1rem;width:40%"></span>
			<span class="sk" style="height:260px;margin-top:12px"></span>
		</div>
	{:else}
		<div class="head">
			<div class="head-text">
				<h2 title={title}>{title}</h2>
				<p class="muted">
					{i18n.t('job.toRecipient')} <span class="mono">{job.recipientEmail}</span> ·
					{i18n.t('job.createdOn', { date: fullDate(job.createdAt, i18n.locale) })}
				</p>
			</div>
						<div class="head-acts">
				
				<a class="btn btn-line btn-sm" href={`/app/jobs/${job.id}`}>
					{i18n.t('dashboard.openJobPage')}
					<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M7 17 L17 7 M9 7 H17 V15" /></svg>
				</a>
				{#if onclose}
					<button type="button" class="close-pane" aria-label={i18n.t('common.closePane')} title={i18n.t('common.closePane')} onclick={onclose}>
						<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" aria-hidden="true"><path d="M6 6 L18 18 M18 6 L6 18" /></svg>
					</button>
				{/if}
			</div>
		</div>

		{#if job.status === 'failed'}
			<p class="error-banner" role="alert">{i18n.t('job.failedBody')}</p>
		{/if}

		<div class="grid">
			<div class="media" class:audio={job.mediaType === 'audio'}>
				{#if shown}
					{#key shown.id}
						{#if job.mediaType === 'video'}
							<!-- svelte-ignore a11y_media_has_caption -->
							<video controls preload="metadata" use:authedSrc={api.assetContentUrl(job.id, shown.id)}></video>
						{:else if job.mediaType === 'audio'}
							<audio controls preload="metadata" use:authedSrc={api.assetContentUrl(job.id, shown.id)}></audio>
						{:else}
							<img src={webSrc(api.assetContentUrl(job.id, shown.id))} use:authedSrc={api.assetContentUrl(job.id, shown.id)} alt={shown.filename} />
						{/if}
					{/key}
				{:else}
					<p class="muted">{i18n.t('job.previewNotReady')}</p>
				{/if}
			</div>
			{#if job.status !== 'failed'}
				<DeliveryCard jobId={job.id} status={job.status} recipient={job.recipientEmail} />
			{/if}
		</div>

		
	{/if}
</div>

<style>
	.panel {
		container: panel / inline-size;
		padding: 20px 22px 24px;
		display: flex;
		flex-direction: column;
		gap: 18px;
		min-width: 0;
	}

	.loading {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.head {
		display: flex;
		align-items: flex-start;
		gap: 12px;
	}

	.head-text {
		flex: 1;
		min-width: 0;
	}

	h2 {
		margin: 0;
		font-size: 1.25rem;
		font-weight: 500;
		letter-spacing: -0.02em;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.head p {
		margin: 4px 0 0;
		font-size: 0.88rem;
		overflow-wrap: anywhere;
	}

	.error-banner {
		margin: 0;
	}

	/* The picture beside its history once the panel is wide enough. */
	.grid {
		display: grid;
		gap: 20px;
		align-items: start;
	}

	@container panel (min-width: 640px) {
		.grid {
			grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr);
		}
	}

	.media {
		border-radius: var(--radius);
		overflow: hidden;
		background: #17130f;
		display: grid;
		place-items: center;
		min-height: 200px;
	}

	.media.audio {
		background: var(--fill);
		min-height: 120px;
		padding: 16px;
	}

	.media img,
	.media video {
		display: block;
		width: 100%;
		max-height: 52svh;
		object-fit: contain;
	}

	/* Stacked (narrow pane, e.g. 1024px): a shorter picture, so the delivery
	   block and its actions start inside the first screen. */
	@container panel (max-width: 639px) {
		.media img,
		.media video {
			max-height: 34svh;
		}
	}

	.media audio {
		width: 100%;
	}

	.media p {
		padding: 24px;
		text-align: center;
		color: #d8d0c6;
	}

	.media.audio p {
		color: var(--text-muted);
	}

		.close-pane {
		appearance: none;
		width: 36px;
		height: 36px;
		display: grid;
		place-items: center;
		border: none;
		border-radius: 50%;
		background: transparent;
		color: var(--text-muted);
		cursor: pointer;
		position: relative;
	}

	.close-pane::after {
		content: '';
		position: absolute;
		inset: -4px;
	}

	.close-pane:hover {
		background: var(--fill-2);
		color: var(--text);
	}

	.head-acts {
		display: flex;
		align-items: center;
		gap: 10px;
		flex: none;
	}

	/* The picture stays in view while the history beside it scrolls. */
	@container panel (min-width: 640px) {
		.media {
			position: sticky;
			top: 0;
		}
	}
</style>
