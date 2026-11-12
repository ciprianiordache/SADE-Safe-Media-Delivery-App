<script lang="ts">
	import { authedSrc, authedHref, webSrc } from '$lib/media';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { i18n, type MessageKey } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import type { Job } from '$lib/types';
	import { formatBytes, fullDate } from '$lib/format';
	import { toasts } from '$lib/toast.svelte';
		import { deliveryState, stateClass, stateKey, type DeliveryState } from '$lib/delivery';
		import DeliveryCard from '$lib/components/DeliveryCard.svelte';
	
	import logo from '$lib/assets/logo.png';
		import { onDestroy, onMount } from 'svelte';
	import { IS_APP } from '$lib/platform';
	import { appTitle } from '$lib/apptitle.svelte';

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
		appTitle.value = '';
	});

	// Stepper, while the file is still on its way: queued, watermarking, sent.
	const STEPS = ['queued', 'watermarking', 'delivered'] as const;
	const STEP_KEYS: Record<(typeof STEPS)[number], MessageKey> = {
		queued: 'job.stepQueued',
		watermarking: 'job.stepWatermarking',
		delivered: 'job.stepDelivered'
	};
	let stepIndex = $derived(job?.status === 'pending' ? 0 : job?.status === 'processing' ? 1 : 3);

	// Play either the watermarked preview or the original, straight from the
	// session-authenticated content route - the operator viewing their own upload.
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

		let title = $derived(job ? (originalAsset?.filename ?? job.fileName ?? job.recipientEmail) : '');

	// The app's title bar reads "‹ Livrări  nume-fișier": the page needs no
	// back link of its own there.
	$effect(() => {
		if (IS_APP) appTitle.value = title;
	});
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

<svelte:head>
	<title>{job ? title : i18n.t('job.title')} · SADE</title>
</svelte:head>

<div class="container page" class:app-file={IS_APP}>
	{#if !IS_APP}
	<a class="back" href="/app">
		<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 6 L9 12 L15 18" /></svg>
				{i18n.t('job.back')}
	</a>
	{/if}

	{#if notFound}
		<div class="error-banner">{i18n.t('job.notFound')}</div>
	{:else if !job}
		<div class="loading" aria-busy="true" aria-label={i18n.t('common.loading')}>
			<span class="sk" style="height:3.2rem;width:55%"></span>
			<span class="sk" style="height:1.2rem;width:35%"></span>
			<span class="sk" style="height:420px;margin-top:24px"></span>
		</div>
	{:else}
		<div class="page-head">
			<div>
				<!-- The operator knows the file by its name; the recipient comes second. -->
								<h1 class:sr-only={IS_APP} title={title}>{title}</h1>
								<p class="lead">
					{i18n.t('job.toRecipient')} <span class="ink mono">{job.recipientEmail}</span> ·
					{i18n.t('job.createdOn', { date: fullDate(job.createdAt, i18n.locale) })}
				</p>
				{#if confirmDelete}
					<p class="head-note armed" role="alert">{i18n.t('job.deleteArmed')}</p>
				{/if}
				{#if deleteError}
					<p class="head-note error-text" role="alert">{deleteError}</p>
				{/if}
			</div>
			<!-- The status and the delete beside the title, as the open delivery's head. -->
						<div class="head-acts">
				{#if IS_APP && previewAsset && originalAsset}
					<div class="segmented compact" role="group" aria-label={i18n.t('job.sectionPreview')}>
						<button type="button" aria-pressed={selectedKind === 'preview'} onclick={() => (selectedKind = 'preview')}>
							{i18n.t('job.tabPreview')}
						</button>
						<button type="button" aria-pressed={selectedKind === 'original'} onclick={() => (selectedKind = 'original')}>
							{i18n.t('job.tabOriginal')}
						</button>
					</div>
				{/if}
								{#if st}
										<span class={stateClass(st)}>{i18n.t(stateKey(st))}</span>
				{/if}
				{#if confirmDelete}
					<button type="button" class="btn btn-ghost btn-sm" onclick={() => (confirmDelete = false)}>
						{i18n.t('job.deleteCancel')}
					</button>
					<button type="button" class="btn btn-sm danger-btn armed" onclick={remove} disabled={deleting}>
						{i18n.t('job.deleteConfirm')}
					</button>
								{/if}
			</div>
		</div>

		{#if job.status === 'failed'}
			<div class="error-banner failed" role="alert">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" style="flex-shrink:0;margin-top:2px;"><path d="M12 8 v5 M12 16 h.01 M10.3 4 L3 17 a2 2 0 0 0 1.7 3 h14.6 a2 2 0 0 0 1.7 -3 L13.7 4 a2 2 0 0 0 -3.4 0 Z" /></svg>
				<div>
					<strong>{i18n.t('job.failedTitle')}</strong>
					<p>{i18n.t('job.failedBody')}</p>
				</div>
			</div>
		{:else if job.status !== 'done'}
			<div class="stepper">
				<ol class="steps">
					{#each STEPS as step, i (step)}
						<li class="step" class:done={i < stepIndex} class:current={i === stepIndex}>
							<span class="step-dot" aria-hidden="true">
								{#if i < stepIndex}
									<svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12.5 L10 17.5 L19 7" /></svg>
								{/if}
							</span>
							<span>{i18n.t(STEP_KEYS[step])}</span>
						</li>
					{/each}
				</ol>
				<p class="muted step-hint">{i18n.t('job.workingHint')}</p>
			</div>
		{/if}

		<!-- The open delivery, as the marketing page shows it beside its list. -->
				<section class="win job-win">
			{#if !IS_APP}
			<div class="win-bar">
				<span class="win-dots" aria-hidden="true"><i></i><i></i><i></i></span>
								{#if !IS_APP}
				<span class="crumb">
					<img src={logo} alt="" width="16" height="16" />
					{i18n.t('dashboard.title')} <i aria-hidden="true">/</i>
										<b>{i18n.t('job.barTitle')}</b>
				</span>
				{/if}
				<span class="sp"></span>
				{#if previewAsset && originalAsset}
					<div class="segmented compact" role="group" aria-label={i18n.t('job.sectionPreview')}>
						<button type="button" aria-pressed={selectedKind === 'preview'} onclick={() => (selectedKind = 'preview')}>
							{i18n.t('job.tabPreview')}
						</button>
												<button type="button" aria-pressed={selectedKind === 'original'} onclick={() => (selectedKind = 'original')}>
							{i18n.t('job.tabOriginal')}
						</button>
					</div>
				{/if}
			</div>
			{/if}

			<div class="job-grid">
				<div class="media-col">
					{#if activeAsset}
						{#key activeAsset.id}
							<div class="player" class:audio={job.mediaType === 'audio'}>
								{#if job.mediaType === 'video'}
									<!-- svelte-ignore a11y_media_has_caption -->
									<video controls preload="metadata" use:authedSrc={api.assetContentUrl(job.id, activeAsset.id)}></video>
								{:else if job.mediaType === 'audio'}
									<audio controls preload="metadata" use:authedSrc={api.assetContentUrl(job.id, activeAsset.id)}></audio>
								{:else}
									<img src={webSrc(api.assetContentUrl(job.id, activeAsset.id))} use:authedSrc={api.assetContentUrl(job.id, activeAsset.id)} alt={activeAsset.filename} />
								{/if}
							</div>
						{/key}
						<div class="player-actions">
							<span class="muted mono file-line" title={activeAsset.filename}>
								{activeAsset.filename} · {formatBytes(activeAsset.sizeBytes)}
							</span>
							<a class="btn btn-line btn-sm" href={api.assetContentUrl(job.id, activeAsset.id, { download: true })} use:authedHref={api.assetContentUrl(job.id, activeAsset.id, { download: true })}>
								<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4 V15 M8 11 L12 15 L16 11" /><path d="M5 19 h14" /></svg>
								{i18n.t('job.download')}
							</a>
						</div>
					{:else}
						<div class="player empty-player">
							<p class="muted">{i18n.t('job.previewNotReady')}</p>
						</div>
					{/if}
				</div>

				<aside class="side-col">
					{#if job.status !== 'failed'}
						<DeliveryCard
							jobId={job.id}
							status={job.status}
							recipient={job.recipientEmail}
							ondelete={remove}
							deleteDisabled={job.status === 'processing'}
							deleteTitle={job.status === 'processing' ? i18n.t('job.deleteBusy') : i18n.t('job.deleteHint')}
						/>
					{/if}
					<section class="details" aria-labelledby="details-title">
						<h2 class="section-label" id="details-title">{i18n.t('job.details')}</h2>
						<dl>
							
														
							<div>
								<dt>{i18n.t('job.watermarkKind')}</dt>
								<dd>{i18n.t(`dashboard.watermarkKind.${job.watermarkKind}`)}</dd>
							</div>
							{#if job.watermarkText}
								<div>
									<dt>{i18n.t('job.watermarkText')}</dt>
									<dd class="wrap">{job.watermarkText}</dd>
								</div>
							{/if}
							{#each job.assets ?? [] as asset (asset.id)}
								<div>
									<dt>{i18n.t(`asset.${asset.kind}`)}</dt>
									<dd class="mono">{formatBytes(asset.sizeBytes)}</dd>
								</div>
							{/each}
							{#if job.attempts > 1}
								<div>
									<dt>{i18n.t('job.attempts')}</dt>
									<dd>{job.attempts}</dd>
								</div>
							{/if}
							<div>
								<dt>{i18n.t('job.updatedAt')}</dt>
								<dd>{fullDate(job.updatedAt, i18n.locale)}</dd>
							</div>
						</dl>
					</section>
				</aside>
			</div>
		</section>

		
	{/if}
</div>

<style>
	.loading {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding-top: 8px;
	}

	.page-head h1 {
		font-size: clamp(1.9rem, 1.2rem + 2.2vw, 3rem);
	}

	.ink {
		color: var(--text);
	}

	.failed {
		gap: 0.75rem;
		margin-bottom: 24px;
	}

	.failed p {
		margin: 0.25rem 0 0;
	}

	/* ---- stepper: three dots on a line, only while the file is on its way */
	.stepper {
		margin-bottom: 24px;
	}

	.steps {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.step {
		display: flex;
		align-items: center;
		gap: 8px;
		flex: 1;
		font-size: 0.92rem;
		color: var(--text-muted);
	}

	.step:not(:last-child)::after {
		content: '';
		flex: 1;
		height: 2px;
		margin-left: 4px;
		border-radius: 2px;
		background: var(--line-2);
	}

	.step:last-child {
		flex: none;
	}

	.step.done,
	.step.current {
		color: var(--text);
	}

	.step.done::after {
		background: var(--ink-btn);
	}

	.step-dot {
		width: 22px;
		height: 22px;
		flex: none;
		border-radius: 50%;
		display: inline-grid;
		place-items: center;
		background: var(--fill-2);
		color: var(--ink-btn-text);
	}

	.step.done .step-dot {
		background: var(--ink-btn);
	}

	.step.current .step-dot {
		background: var(--accent-bright);
		animation: ring 1.6s ease-out infinite;
	}

	@keyframes ring {
		0% {
			box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent-bright) 45%, transparent);
		}
		100% {
			box-shadow: 0 0 0 8px transparent;
		}
	}

	.step-hint {
		margin: 10px 0 0;
		font-size: 0.88rem;
	}

	@media (max-width: 520px) {
		.steps {
			gap: 6px;
		}
		.step {
			font-size: 0.8rem;
			gap: 6px;
		}
	}

	/* ---- the open delivery window ------------------------------------- */
	.crumb {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
	}

	.crumb img {
		flex: none;
	}

	.crumb i {
		font-style: normal;
	}

	.crumb b {
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.segmented.compact {
		flex: none;
		padding: 2px;
	}

	.segmented.compact button {
		min-height: 30px;
		padding: 0 0.7rem;
		font-size: 0.82rem;
	}

	.segmented.compact button::after {
		inset: -7px 0;
	}

	.job-grid {
		display: grid;
		grid-template-columns: minmax(0, 1.5fr) minmax(320px, 1fr);
	}

	@media (max-width: 1023px) {
		.job-grid {
			grid-template-columns: minmax(0, 1fr);
		}
	}

	.media-col {
		padding: 20px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.side-col {
		padding: 20px;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 28px;
		border-left: 1px solid var(--line);
	}

	@media (max-width: 1023px) {
		.side-col {
			border-left: none;
			border-top: 1px solid var(--line);
		}
	}

	.player {
		border-radius: 10px;
		overflow: hidden;
		background: #17130f;
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 220px;
	}

	.player.audio {
		background: var(--fill);
		min-height: 120px;
	}

	.player video,
	.player img {
		width: 100%;
		max-height: 70svh;
		object-fit: contain;
		display: block;
	}

	.player audio {
		width: 100%;
		margin: 1.5rem;
	}

	.empty-player {
		background: var(--fill);
		padding: 2rem;
		text-align: center;
	}

	.player-actions {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.file-line {
		font-size: 0.85rem;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1;
	}

	.details {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.details h2 {
		margin: 0;
		font-size: 1.375rem;
	}

	

	/* ---- the installed app: the delivery is the window, not a card on it.
	   The head stays put; the picture and the delivery scroll on their own. */
	.app-file {
		height: 100%;
		display: flex;
		flex-direction: column;
		padding-bottom: 0 !important;
		/* One surface: the head and the delivery on the same white. */
		background: var(--surface);
	}

	.app-file .page-head {
		flex: none;
	}

	.app-file .job-win {
		flex: 1;
		min-height: 0;
		margin: 0 -22px;
		border: none;
		border-top: 1px solid var(--line);
		border-radius: 0;
		box-shadow: none;
		background: var(--surface);
	}

	.app-file .job-grid {
		height: 100%;
	}

	.app-file .media-col,
	.app-file .side-col {
		overflow-y: auto;
		padding: 20px 22px 28px;
	}

	.app-file .player video,
	.app-file .player img {
		max-height: calc(100svh - 230px);
	}

	@media (max-width: 1023px) {
		.app-file .job-grid {
			height: auto;
		}
		.app-file .job-win {
			overflow-y: auto;
		}
		.app-file .media-col,
		.app-file .side-col {
			overflow: visible;
		}
	}

	/* Hairline rows, like the marketing page's template table. */
	dl {
		margin: 0;
	}

	dl > div {
		display: grid;
		grid-template-columns: 120px minmax(0, 1fr);
		gap: 12px;
		padding: 9px 0;
		border-top: 1px solid var(--line);
		font-size: 0.9rem;
	}

	dl > div:last-child {
		border-bottom: 1px solid var(--line);
	}

	dt {
		color: var(--text-muted);
	}

	dd {
		margin: 0;
		min-width: 0;
	}

	.wrap {
		overflow-wrap: anywhere;
	}

		/* ---- head: status + delete --------------------------------------- */
	.head-acts {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 8px;
		justify-self: end;
		align-self: end;
		margin-bottom: 4px;
	}

	@media (max-width: 639px) {
		.head-acts {
			justify-self: start;
		}
	}

	.head-note {
		margin: 10px 0 0;
		font-size: 0.92rem;
	}

	.head-note.armed {
		color: var(--danger);
	}

	.danger-btn {
		color: var(--danger);
		border-color: color-mix(in oklab, var(--danger) 40%, transparent);
	}

	.danger-btn:hover:not(:disabled),
	.danger-btn.armed {
		background: var(--danger);
		color: #fff;
		border-color: transparent;
	}
</style>
