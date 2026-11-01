<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { i18n, type MessageKey } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import type { Job } from '$lib/types';
	import { formatBytes, fullDate } from '$lib/format';
	import { toasts } from '$lib/toast.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import MediaIcon from '$lib/components/MediaIcon.svelte';
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

	// Two-step delete: the first click arms it, the second deletes.
	let confirmDelete = $state(false);
	let deleting = $state(false);
	let deleteError = $state('');

	async function remove() {
		if (!job) return;
		if (!confirmDelete) {
			confirmDelete = true;
			return;
		}
		deleting = true;
		deleteError = '';
		try {
			await api.deleteJob(job.id);
			toasts.show(i18n.t('job.deleted'));
			goto('/app');
		} catch (err) {
			confirmDelete = false;
			if (err instanceof ApiError && err.status === 409) {
				deleteError = err.message.includes('paid') ? i18n.t('job.deletePaid') : i18n.t('job.deleteBusy');
			} else {
				deleteError = i18n.t('job.deleteError');
			}
		} finally {
			deleting = false;
		}
	}

	onMount(() => {
		load();
		pollHandle = setInterval(() => {
			if (document.hidden) return;
			if (job && (job.status === 'pending' || job.status === 'processing')) load();
		}, 4000);
	});

	onDestroy(() => {
		if (pollHandle) clearInterval(pollHandle);
	});

	// Stepper: pending/processing sit on their step; done completes all three.
	// A failed job shows an explanation banner instead.
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

	// Preview card: play either the watermarked preview or the original,
	// straight from the session-authenticated /api/jobs/.../content route
	// (see src/lib/api.ts) - no signed /p token needed, this is the operator
	// viewing their own upload.
	let previewAsset = $derived(job?.assets?.find((a) => a.kind === 'preview'));
	let originalAsset = $derived(job?.assets?.find((a) => a.kind === 'original'));
	let selectedKind = $state<'preview' | 'original'>('preview');
	let activeAsset = $derived(selectedKind === 'preview' ? previewAsset : originalAsset);

	// Show the original while there is no preview yet, and switch to the
	// preview once it appears (the page polls while processing).
	let lastPreviewId = '';
	$effect(() => {
		const id = previewAsset?.id ?? '';
		if (id && id !== lastPreviewId) {
			lastPreviewId = id;
			selectedKind = 'preview';
		} else if (!id && originalAsset) {
			selectedKind = 'original';
		}
	});
</script>

<svelte:head>
	<title>{job ? job.recipientEmail : i18n.t('job.title')} · SADE</title>
</svelte:head>

<div class="container page">
	<a class="back" href="/app">
		<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
			><path d="M15 6 L9 12 L15 18" /></svg
		>
		{i18n.t('job.back')}
	</a>

	{#if notFound}
		<div class="error-banner">{i18n.t('job.notFound')}</div>
	{:else if !job}
		<p class="muted">{i18n.t('common.loading')}</p>
	{:else}
		<div class="head">
			<span class="head-icon"><MediaIcon type={job.mediaType} size={20} /></span>
			<div class="head-main">
				<span class="recipient mono" title={job.recipientEmail}>{job.recipientEmail}</span>
				<span class="faint meta">
					{i18n.t(`media.${job.mediaType}`)} · {i18n.t(`dashboard.watermarkKind.${job.watermarkKind}`)} ·
					{fullDate(job.createdAt, i18n.locale)}
				</span>
			</div>
			<StatusBadge status={job.status} />
		</div>

		{#if job.status === 'failed'}
			<div class="error-banner failed" role="alert">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" style="flex-shrink:0;margin-top:2px;"
					><path d="M12 8 v5 M12 16 h.01 M10.3 4 L3 17 a2 2 0 0 0 1.7 3 h14.6 a2 2 0 0 0 1.7 -3 L13.7 4 a2 2 0 0 0 -3.4 0 Z" /></svg
				>
				<div>
					<strong>{i18n.t('job.failedTitle')}</strong>
					<p>{i18n.t('job.failedBody')}</p>
				</div>
			</div>
		{:else}
			<div class="stepper card">
				<div class="steps">
					{#each STEPS as step, i}
						<div class="step">
							<span class="step-dot" class:done={i < stepIndex} class:current={i === stepIndex}>
								{#if i < stepIndex}
									<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"
										><path d="M5 12.5 L10 17.5 L19 7" /></svg
									>
								{/if}
							</span>
							<span class:step-label-done={i <= stepIndex}>{i18n.t(STEP_KEYS[step])}</span>
						</div>
						{#if i < STEPS.length - 1}
							<div class="step-line" class:done={i < stepIndex}></div>
						{/if}
					{/each}
				</div>
				<p class="faint step-hint">
					{job.status === 'done'
						? i18n.t('job.deliveredHint', { email: job.recipientEmail })
						: i18n.t('job.workingHint')}
				</p>
			</div>
		{/if}

		<div class="detail-grid">
			<section class="card">
				<div class="preview-head">
					<span class="section-label">
						{previewAsset ? i18n.t('job.sectionPreview') : i18n.t('job.tabOriginal')}
					</span>
					{#if previewAsset && originalAsset}
						<div class="segmented small">
							<button type="button" aria-pressed={selectedKind === 'preview'} onclick={() => (selectedKind = 'preview')}>
								{i18n.t('job.tabPreview')}
							</button>
							<button type="button" aria-pressed={selectedKind === 'original'} onclick={() => (selectedKind = 'original')}>
								{i18n.t('job.tabOriginal')}
							</button>
						</div>
					{/if}
				</div>

				{#if activeAsset}
					{#key activeAsset.id}
						<div class="player" class:audio={job.mediaType === 'audio'}>
							{#if job.mediaType === 'video'}
								<!-- svelte-ignore a11y_media_has_caption -->
								<video controls preload="metadata" src={api.assetContentUrl(job.id, activeAsset.id)}></video>
							{:else if job.mediaType === 'audio'}
								<audio controls preload="metadata" src={api.assetContentUrl(job.id, activeAsset.id)}></audio>
							{:else}
								<img src={api.assetContentUrl(job.id, activeAsset.id)} alt="" />
							{/if}
						</div>
					{/key}
					<div class="player-actions">
						<span class="faint mono file-line" title={activeAsset.filename}>
							{activeAsset.filename} · {formatBytes(activeAsset.sizeBytes)}
						</span>
						<a class="btn btn-ghost" href={api.assetContentUrl(job.id, activeAsset.id, { download: true })}>
							<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
								><path d="M12 4 V15 M8 11 L12 15 L16 11" /><path d="M5 19 h14" /></svg
							>
							{i18n.t('job.download')}
						</a>
					</div>
				{:else}
					<p class="muted">{i18n.t('job.previewNotReady')}</p>
				{/if}
			</section>

			<section class="card">
				<span class="section-label">{i18n.t('job.title')}</span>
				<dl>
					<dt>{i18n.t('job.recipient')}</dt>
					<dd class="mono wrap">{job.recipientEmail}</dd>

					<dt>{i18n.t('job.watermarkKind')}</dt>
					<dd>{i18n.t(`dashboard.watermarkKind.${job.watermarkKind}`)}</dd>

					{#if job.watermarkText}
						<dt>{i18n.t('job.watermarkText')}</dt>
						<dd class="wrap">{job.watermarkText}</dd>
					{/if}

					{#each job.assets ?? [] as asset (asset.id)}
						<dt>{i18n.t(`asset.${asset.kind}`)}</dt>
						<dd class="mono">{formatBytes(asset.sizeBytes)}</dd>
					{/each}

					{#if job.attempts > 1}
						<dt>{i18n.t('job.attempts')}</dt>
						<dd>{job.attempts}</dd>
					{/if}

					<dt>{i18n.t('job.updatedAt')}</dt>
					<dd>{fullDate(job.updatedAt, i18n.locale)}</dd>
				</dl>
				{#if job.status !== 'failed'}
					<p class="faint note">{i18n.t('job.assetsNote')}</p>
				{/if}
			</section>
		</div>

		<section class="danger">
			<div>
				<span class="section-label">{i18n.t('job.deleteTitle')}</span>
				<p class="faint small">{i18n.t('job.deleteHint')}</p>
				{#if deleteError}
					<p class="error-text">{deleteError}</p>
				{/if}
			</div>
			<div class="danger-actions">
				{#if confirmDelete}
					<button type="button" class="btn btn-ghost" onclick={() => (confirmDelete = false)}>
						{i18n.t('job.deleteCancel')}
					</button>
				{/if}
				<button
					type="button"
					class="btn danger-btn"
					onclick={remove}
					disabled={deleting || job.status === 'processing'}
				>
					{confirmDelete ? i18n.t('job.deleteConfirm') : i18n.t('job.delete')}
				</button>
			</div>
		</section>
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
		align-self: flex-start;
	}

	.back:hover {
		color: var(--text);
	}

	.head {
		display: flex;
		align-items: center;
		gap: 0.9rem;
	}

	.head-icon {
		width: 44px;
		height: 44px;
		flex-shrink: 0;
		border-radius: 11px;
		background: var(--accent-tint);
		color: var(--accent-tint-text);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.head-main {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.recipient {
		font-size: 1.3rem;
		font-weight: 700;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.meta {
		font-size: 0.85rem;
	}

	@media (max-width: 520px) {
		.page {
			padding: 1.25rem 1rem 3rem;
		}
		.recipient {
			font-size: 1.05rem;
		}
	}

	.failed {
		gap: 0.75rem;
	}

	.failed p {
		margin: 0.25rem 0 0;
	}

	.stepper {
		padding: 1rem 1.25rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.steps {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.step {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		font-size: 0.85rem;
		color: var(--text-muted);
	}

	@media (max-width: 520px) {
		.steps {
			gap: 0.4rem;
		}
		.step {
			font-size: 0.75rem;
			gap: 0.35rem;
		}
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
		animation: ring 1.6s ease-out infinite;
	}

	@keyframes ring {
		0% {
			box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent) 45%, transparent);
		}
		100% {
			box-shadow: 0 0 0 8px transparent;
		}
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

	.step-hint {
		margin: 0;
		font-size: 0.8rem;
	}

	.detail-grid {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 300px;
		gap: 1.25rem;
		align-items: start;
	}

	@media (max-width: 760px) {
		.detail-grid {
			grid-template-columns: minmax(0, 1fr);
		}
	}

	.detail-grid .card {
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
	}

	.preview-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.segmented.small {
		width: auto;
		border-radius: 999px;
		overflow: hidden;
	}

	.segmented.small button {
		padding: 0.32rem 0.7rem;
		white-space: nowrap;
		font-size: 0.78rem;
	}

	.player {
		border-radius: var(--radius-sm);
		overflow: hidden;
		background: #000;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.player.audio {
		background: var(--bg);
		border: 1px solid var(--border);
	}

	.player video,
	.player img {
		width: 100%;
		max-height: 480px;
		object-fit: contain;
		display: block;
	}

	.player audio {
		width: 100%;
		margin: 1.5rem;
	}

	.player-actions {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.file-line {
		font-size: 0.78rem;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1;
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
		gap: 0.55rem 1.25rem;
		margin: 0;
	}

	dt {
		color: var(--text-faint);
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		padding-top: 0.1rem;
	}

	dd {
		margin: 0;
		font-size: 0.88rem;
		min-width: 0;
	}

	.wrap {
		overflow-wrap: anywhere;
	}

	.note {
		font-size: 0.78rem;
		line-height: 1.5;
		margin: 0;
	}

	.danger {
		margin-top: 1rem;
		padding-top: 1.25rem;
		border-top: 1px solid var(--border);
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
		align-items: center;
		justify-content: space-between;
	}

	.danger p {
		margin: 0.35rem 0 0;
	}

	.small {
		font-size: 0.82rem;
	}

	.danger-actions {
		display: flex;
		gap: 0.5rem;
	}

	.danger-btn {
		background: var(--danger);
	}

	.danger-btn:hover:not(:disabled) {
		background: var(--danger);
		filter: brightness(0.92);
	}
</style>
