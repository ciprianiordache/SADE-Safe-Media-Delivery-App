<script lang="ts">
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { isAllowedFile, mediaTypeOf } from '$lib/types';
	import type { Job, JobStatus, Usage, WatermarkKind } from '$lib/types';
	import { ago, formatBytes, fullDate, relativeTime } from '$lib/format';
	import { toasts } from '$lib/toast.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import MediaIcon from '$lib/components/MediaIcon.svelte';
	import UsageMeter from '$lib/components/UsageMeter.svelte';
	import PasskeyBanner from '$lib/components/PasskeyBanner.svelte';
	import { onDestroy, onMount } from 'svelte';

	// --- data -----------------------------------------------------------
	let jobs = $state<Job[]>([]);
	let jobsLoading = $state(true);
	let usage = $state<Usage | null>(null);

	async function loadUsage() {
		try {
			usage = await api.usage();
		} catch {
			// The meter is informational; the dashboard works without it.
		}
	}

	// Tells the operator when a job they are waiting on finishes, so they
	// don't have to keep watching the list.
	function announceFinished(prev: Job[], next: Job[]) {
		const before = new Map(prev.map((j) => [j.id, j.status]));
		for (const j of next) {
			const was = before.get(j.id);
			if (was !== 'pending' && was !== 'processing') continue;
			if (j.status === 'done') {
				toasts.show(i18n.t('dashboard.readyToast', { email: j.recipientEmail }), {
					href: `/app/jobs/${j.id}`,
					action: i18n.t('dashboard.seeJob')
				});
			} else if (j.status === 'failed') {
				toasts.show(i18n.t('dashboard.failedToast', { email: j.recipientEmail }), {
					kind: 'error',
					href: `/app/jobs/${j.id}`,
					action: i18n.t('dashboard.seeJob'),
					ms: 8000
				});
			}
		}
	}

	async function loadJobs() {
		try {
			const next = await api.listJobs(0, 200);
			if (!jobsLoading) announceFinished(jobs, next);
			jobs = next;
		} finally {
			jobsLoading = false;
		}
	}

	let pollHandle: ReturnType<typeof setInterval> | undefined;
	onMount(() => {
		loadJobs();
		loadUsage();
		try {
			const k = localStorage.getItem(KIND_KEY);
			if (k === 'logo' || k === 'text' || k === 'both') watermarkKind = k;
		} catch {
			// no storage - keep the default
		}
		pollHandle = setInterval(() => {
			if (document.hidden) return;
			if (jobs.some((j) => j.status === 'pending' || j.status === 'processing')) loadJobs();
		}, 4000);
	});
	onDestroy(() => {
		if (pollHandle) clearInterval(pollHandle);
		uploadAbort?.abort();
	});

	// --- upload form ------------------------------------------------------
	const KIND_KEY = 'sade:watermarkKind';
	const KINDS: WatermarkKind[] = ['both', 'logo', 'text'];

	let file = $state<File | null>(null);
	let fileInput = $state<HTMLInputElement | undefined>();
	let fileError = $derived(file && !isAllowedFile(file.name) ? i18n.t('dashboard.fileUnsupported') : '');
	let fileKind = $derived(file ? mediaTypeOf(file.name) : null);
	let recipientEmail = $state('');
	let watermarkKind = $state<WatermarkKind>('both');
	let watermarkText = $state('');
	let submitting = $state(false);
	let progress = $state(0);
	let uploadError = $state('');
	let dragging = $state(false);
	let uploadAbort: AbortController | null = null;

	function pickKind(k: WatermarkKind) {
		watermarkKind = k;
		try {
			localStorage.setItem(KIND_KEY, k);
		} catch {
			// best-effort
		}
	}

	function setFile(f: File | null) {
		file = f;
		uploadError = '';
	}

	function clearFile() {
		setFile(null);
		if (fileInput) fileInput.value = '';
	}

	function onFileChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		setFile(input.files?.[0] ?? null);
	}

	// dragenter/dragleave fire for every child element, so count them to know
	// when the pointer has really left the zone.
	let dragDepth = 0;
	function onDragEnter(e: DragEvent) {
		if (!e.dataTransfer?.types.includes('Files')) return;
		e.preventDefault();
		dragDepth++;
		dragging = true;
	}
	function onDragLeave() {
		dragDepth = Math.max(0, dragDepth - 1);
		if (dragDepth === 0) dragging = false;
	}
	function onDragOver(e: DragEvent) {
		if (!e.dataTransfer?.types.includes('Files')) return;
		e.preventDefault();
		e.dataTransfer.dropEffect = 'copy';
	}
	function onDrop(e: DragEvent) {
		e.preventDefault();
		dragDepth = 0;
		dragging = false;
		const f = e.dataTransfer?.files?.[0];
		if (f && !submitting) setFile(f);
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!file || fileError) return;
		uploadError = '';
		submitting = true;
		progress = 0;
		uploadAbort = new AbortController();
		const email = recipientEmail;
		try {
			const job = await api.createJob(
				{
					file,
					recipientEmail,
					watermarkKind,
					watermarkText: (watermarkKind !== 'logo' && watermarkText.trim()) || undefined
				},
				{ onProgress: (p) => (progress = p), signal: uploadAbort.signal }
			);
			// A poll that ran during the upload may already have fetched this job.
			jobs = [job, ...jobs.filter((j) => j.id !== job.id)];
			loadUsage();
			clearFile();
			recipientEmail = '';
			watermarkText = '';
			toasts.show(i18n.t('dashboard.uploadedToast', { email }), {
				href: `/app/jobs/${job.id}`,
				action: i18n.t('dashboard.seeJob')
			});
		} catch (err) {
			if (err instanceof DOMException && err.name === 'AbortError') {
				toasts.show(i18n.t('dashboard.uploadCanceled'), { kind: 'info' });
			} else {
				uploadError = uploadErrorMessage(err);
			}
		} finally {
			submitting = false;
			uploadAbort = null;
		}
	}

	// Server errors arrive in English; show the common ones in the UI language.
	function uploadErrorMessage(err: unknown): string {
		if (!(err instanceof ApiError)) return i18n.t('dashboard.uploadError');
		switch (err.status) {
			case 429:
				return i18n.t('dashboard.errorDailyLimit');
			case 413:
				return err.message.includes('quota')
					? i18n.t('dashboard.errorStorageQuota')
					: i18n.t('dashboard.errorTooLarge');
			case 415:
				return i18n.t('dashboard.errorUnsupported');
			case 400:
				return i18n.t('dashboard.errorInvalid');
			default:
				return i18n.t('dashboard.uploadError');
		}
	}

	// --- list controls ----------------------------------------------------
	// Status filters plus two delivery ones (recipient opened / paid).
	type StatusFilter = 'all' | JobStatus | 'opened' | 'paid';
	type Sort = 'newest' | 'oldest' | 'status';
	const PAGE_SIZE = 15;
	const FILTERS: StatusFilter[] = ['all', 'processing', 'pending', 'done', 'failed', 'opened', 'paid'];

	let search = $state('');
	let statusFilter = $state<StatusFilter>('all');
	let sort = $state<Sort>('newest');
	let currentPage = $state(1);

	let searchedJobs = $derived(
		search.trim()
			? jobs.filter((j) => j.recipientEmail.toLowerCase().includes(search.trim().toLowerCase()))
			: jobs
	);

	let statusCounts = $derived.by(() => {
		const c: Record<StatusFilter, number> = {
			all: searchedJobs.length,
			pending: 0,
			processing: 0,
			done: 0,
			failed: 0,
			opened: 0,
			paid: 0
		};
		for (const j of searchedJobs) {
			c[j.status]++;
			if (j.openedAt) c.opened++;
			if (j.paid) c.paid++;
		}
		return c;
	});

	// Only offer filters that would show something - an empty "Failed" chip is noise.
	let visibleFilters = $derived(FILTERS.filter((f) => f === 'all' || f === statusFilter || statusCounts[f] > 0));

	const statusOrder: Record<JobStatus, number> = { processing: 0, pending: 1, failed: 2, done: 3 };

	let sortedJobs = $derived(
		searchedJobs
			.filter((j) => {
				if (statusFilter === 'all') return true;
				if (statusFilter === 'opened') return !!j.openedAt;
				if (statusFilter === 'paid') return j.paid;
				return j.status === statusFilter;
			})
			.slice()
			.sort((a, b) => {
				if (sort === 'status') return statusOrder[a.status] - statusOrder[b.status];
				const diff = new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime();
				return sort === 'oldest' ? diff : -diff;
			})
	);

	let totalPages = $derived(Math.max(1, Math.ceil(sortedJobs.length / PAGE_SIZE)));
	let pageJobs = $derived(sortedJobs.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE));

	$effect(() => {
		// Narrowing the result set can strand currentPage past the last page.
		void search;
		void statusFilter;
		currentPage = 1;
	});
</script>

<svelte:head>
	<title>{i18n.t('dashboard.title')} · SADE</title>
</svelte:head>

<div class="container page">
	<h1>{i18n.t('dashboard.title')}</h1>
	<p class="subtitle muted">{i18n.t('dashboard.subtitle')}</p>
	<PasskeyBanner />
	<UsageMeter {usage} />

	<div class="layout">
		<!-- new job -->
		<section class="card new-job">
			<span class="section-label">{i18n.t('dashboard.newJob')}</span>
			<form onsubmit={submit}>
				<div class="field">
					<label for="file-pick">{i18n.t('dashboard.fileLabel')}</label>
					{#if file}
						<div class="file-card" class:has-error={!!fileError}>
							<span class="file-icon">
								{#if fileKind}<MediaIcon type={fileKind} size={18} />{/if}
							</span>
							<span class="file-meta">
								<span class="file-name" title={file.name}>{file.name}</span>
								<span class="faint mono file-size">
									{formatBytes(file.size)}{fileError ? ` · ${fileError}` : ''}
								</span>
							</span>
							{#if !submitting}
								<button type="button" class="link-btn" onclick={() => fileInput?.click()}>
									{i18n.t('dashboard.fileChange')}
								</button>
								<button type="button" class="icon-btn" aria-label={i18n.t('dashboard.fileRemove')} onclick={clearFile}>
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"
										><path d="M6 6 L18 18 M18 6 L6 18" /></svg
									>
								</button>
							{/if}
						</div>
					{:else}
						<button
							id="file-pick"
							type="button"
							class="dropzone"
							class:dragging
							onclick={() => fileInput?.click()}
							ondragenter={onDragEnter}
							ondragleave={onDragLeave}
							ondragover={onDragOver}
							ondrop={onDrop}
						>
							<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke={dragging ? 'var(--accent)' : 'var(--text-hint)'} stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"
								><path d="M12 15 V4 M8 8 L12 4 L16 8" /><path d="M5 15 v4 h14 v-4" /></svg
							>
							{#if dragging}
								<span class="accent">{i18n.t('dashboard.dropRelease')}</span>
							{:else}
								<span class="muted">
									{i18n.t('dashboard.dropText')}
									<span class="accent">{i18n.t('dashboard.dropBrowse')}</span>
								</span>
							{/if}
							<span class="faint mono types">{i18n.t('dashboard.allowedTypes')}</span>
						</button>
					{/if}
					<input
						type="file"
						accept="video/*,audio/*,image/*"
						bind:this={fileInput}
						onchange={onFileChange}
						style="display:none"
					/>
					{#if fileError}
						<span class="error-text small">{i18n.t('dashboard.fileChooseSupported')}</span>
					{/if}
				</div>

				<div class="field">
					<label for="recipient">{i18n.t('dashboard.recipientLabel')}</label>
					<input
						id="recipient"
						type="email"
						required
						autocomplete="email"
						placeholder={i18n.t('dashboard.recipientPlaceholder')}
						bind:value={recipientEmail}
						disabled={submitting}
					/>
				</div>

				<div class="field">
					<span class="label" id="kind-label">{i18n.t('dashboard.watermarkLabel')}</span>
					<div class="segmented" role="group" aria-labelledby="kind-label">
						{#each KINDS as k (k)}
							<button type="button" aria-pressed={watermarkKind === k} onclick={() => pickKind(k)} disabled={submitting}>
								{i18n.t(`dashboard.watermarkKind.${k}`)}
							</button>
						{/each}
					</div>
				</div>

				{#if watermarkKind !== 'logo'}
					<div class="field">
						<label for="text"
							>{i18n.t('dashboard.watermarkTextLabel')}
							<span class="optional faint">{i18n.t('dashboard.optional')}</span></label
						>
						<input
							id="text"
							type="text"
							maxlength="200"
							placeholder={i18n.t('dashboard.watermarkTextPlaceholder')}
							bind:value={watermarkText}
							disabled={submitting}
						/>
						<span class="faint small">{i18n.t('dashboard.watermarkHint')}</span>
					</div>
				{/if}

				{#if uploadError}
					<p class="error-banner" role="alert">{uploadError}</p>
				{/if}

				{#if submitting}
					<div class="progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.round(progress * 100)}>
						<div class="progress-head">
							<span>{i18n.t('dashboard.submitting')}</span>
							<span class="mono">{Math.round(progress * 100)}%</span>
						</div>
						<div class="track"><div class="fill" style="width:{progress * 100}%"></div></div>
						<button type="button" class="btn btn-ghost" style="width:100%" onclick={() => uploadAbort?.abort()}>
							{i18n.t('dashboard.cancelUpload')}
						</button>
					</div>
				{:else}
					<button class="btn" type="submit" disabled={!file || !!fileError} style="width:100%">
						{i18n.t('dashboard.submit')}
					</button>
				{/if}
			</form>
		</section>

		<!-- jobs area -->
		<section class="jobs">
			{#if jobsLoading}
				<div class="skeleton-list" aria-hidden="true">
					{#each [0, 1, 2] as i (i)}<div class="skeleton-row"></div>{/each}
				</div>
			{:else if jobs.length === 0}
				<div class="empty">
					<span class="empty-icon">
						<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="var(--accent-tint-text)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"
							><path d="M4 13 V6 a1 1 0 0 1 1 -1 h5 l2 2 h7 a1 1 0 0 1 1 1 v5" /><path
								d="M3 13 h4 l1.5 2.5 h7 L17 13 h4 l-1.4 6.1 a1 1 0 0 1 -1 0.9 H5.4 a1 1 0 0 1 -1 -0.9 Z"
							/></svg
						>
					</span>
					<span class="empty-title">{i18n.t('dashboard.emptyTitle')}</span>
					<span class="muted empty-body">{i18n.t('dashboard.emptyBody')}</span>
				</div>
			{:else}
				<div class="toolbar">
					<div class="toolbar-search">
						<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="var(--text-faint)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
							><circle cx="11" cy="11" r="7" /><path d="M20 20 L16 16" /></svg
						>
						<input
							type="search"
							class="search-input"
							aria-label={i18n.t('dashboard.searchPlaceholder')}
							placeholder={i18n.t('dashboard.searchPlaceholder')}
							bind:value={search}
						/>
					</div>
					<div class="toolbar-select">
						<span class="faint">{i18n.t('dashboard.sortLabel')}</span>
						<select bind:value={sort} aria-label={i18n.t('dashboard.sortLabel')}>
							<option value="newest">{i18n.t('dashboard.sort.newest')}</option>
							<option value="oldest">{i18n.t('dashboard.sort.oldest')}</option>
							<option value="status">{i18n.t('dashboard.sort.status')}</option>
						</select>
					</div>
				</div>

				{#if visibleFilters.length > 2}
					<div class="filter-chips">
						{#each visibleFilters as f (f)}
							<button type="button" class="filter-chip" aria-pressed={statusFilter === f} onclick={() => (statusFilter = f)}>
								{i18n.t(`dashboard.filter.${f}`)} <span class="count">{statusCounts[f]}</span>
							</button>
						{/each}
					</div>
				{/if}

				{#if sortedJobs.length === 0}
					<p class="muted no-matches">{i18n.t('dashboard.noMatches')}</p>
				{:else}
					<div class="job-list">
						{#each pageJobs as job (job.id)}
							<a
								class="job-row"
								class:accent-border={job.status === 'processing'}
								class:danger-border={job.status === 'failed'}
								href={`/app/jobs/${job.id}`}
							>
								<span class="row-icon"><MediaIcon type={job.mediaType} /></span>
								<span class="row-main">
									<span class="bold recipient">{job.recipientEmail}</span>
									<span class="meta-line">
										<span class="faint">
											{i18n.t(`media.${job.mediaType}`)} · {i18n.t(`dashboard.watermarkKind.${job.watermarkKind}`)}
										</span>
										{#if job.paid}
											<span class="signal paid" title={i18n.t('signal.paidTitle')}>
												<svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12.5 L10 17.5 L19 7" /></svg>
												{i18n.t('signal.paid')}
											</span>
										{/if}
										{#if job.openedAt}
											<span class="signal opened" title={i18n.t('signal.openedTitle', { date: fullDate(job.openedAt, i18n.locale) })}>
												<svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12 C5 6 19 6 22 12 C19 18 5 18 2 12 Z" /><circle cx="12" cy="12" r="3" /></svg>
												{i18n.t('signal.opened', { when: ago(job.openedAt, i18n.locale) })}
											</span>
										{:else if job.status === 'done' && !job.linksDisabled && !job.paid}
											<span class="signal unopened" title={i18n.t('signal.notOpenedTitle')}>{i18n.t('signal.notOpened')}</span>
										{/if}
										{#if job.linksDisabled}
											<span class="signal off" title={i18n.t('signal.linksOffTitle')}>{i18n.t('signal.linksOff')}</span>
										{/if}
									</span>
								</span>
								<StatusBadge status={job.status} />
								<span class="faint mono when" title={fullDate(job.createdAt, i18n.locale)}>
									{relativeTime(job.createdAt, i18n.locale)}
								</span>
								<svg class="chev" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="var(--text-hint)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
									><path d="M9 6 L15 12 L9 18" /></svg
								>
							</a>
						{/each}
					</div>
				{/if}

				{#if totalPages > 1}
					<div class="pagination">
						<span class="faint mono">
							{(currentPage - 1) * PAGE_SIZE + 1}–{Math.min(currentPage * PAGE_SIZE, sortedJobs.length)}
							{i18n.t('dashboard.of')}
							{sortedJobs.length}
						</span>
						<div class="pagination-controls">
							<button type="button" class="page-btn" disabled={currentPage <= 1} onclick={() => currentPage--}>
								{i18n.t('dashboard.prev')}
							</button>
							<span class="faint mono page-of">{currentPage} / {totalPages}</span>
							<button type="button" class="page-btn" disabled={currentPage >= totalPages} onclick={() => currentPage++}>
								{i18n.t('dashboard.next')}
							</button>
						</div>
					</div>
				{/if}
			{/if}
		</section>
	</div>
</div>

<style>
	.page {
		padding: 1.75rem 1.75rem 3rem;
	}

	@media (max-width: 520px) {
		.page {
			padding: 1.25rem 1rem 3rem;
		}
	}

	h1 {
		margin: 0;
		font-size: 1.55rem;
		font-weight: 700;
		letter-spacing: -0.01em;
	}

	.subtitle {
		margin: 0.2rem 0 1.5rem;
		font-size: 0.85rem;
	}

	.layout {
		display: grid;
		grid-template-columns: 340px minmax(0, 1fr);
		gap: 1.5rem;
		align-items: start;
	}

	@media (max-width: 860px) {
		.layout {
			grid-template-columns: minmax(0, 1fr);
		}
	}

	.new-job {
		display: flex;
		flex-direction: column;
		gap: 1.1rem;
	}

	@media (min-width: 861px) {
		.new-job {
			position: sticky;
			top: 1rem;
		}
	}

	.section-label {
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		color: var(--text-faint);
		text-transform: uppercase;
	}

	.label {
		display: block;
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.03em;
		margin-bottom: 0.35rem;
		color: var(--text-muted);
		text-transform: uppercase;
	}

	.optional {
		text-transform: none;
		font-weight: 400;
	}

	.dropzone {
		width: 100%;
		font-family: var(--font-sans);
		transition:
			border-color 0.15s,
			background 0.15s;
	}

	.dropzone:hover,
	.dropzone.dragging {
		border-color: var(--accent);
		background: var(--accent-tint);
	}

	.dropzone .types {
		font-size: 0.7rem;
	}

	.file-card {
		display: flex;
		align-items: center;
		gap: 0.65rem;
		padding: 0.65rem 0.6rem 0.65rem 0.7rem;
		border: 1.5px solid var(--border-strong);
		border-radius: var(--radius-sm);
		background: var(--bg);
	}

	.file-card.has-error {
		border-color: color-mix(in srgb, var(--danger) 55%, var(--border-strong));
		background: var(--danger-tint);
	}

	.file-icon {
		width: 34px;
		height: 34px;
		flex-shrink: 0;
		border-radius: 8px;
		background: var(--accent-tint);
		color: var(--accent-tint-text);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.file-meta {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}

	.file-name {
		font-size: 0.85rem;
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.file-size {
		font-size: 0.72rem;
	}

	.link-btn {
		appearance: none;
		background: none;
		border: none;
		color: var(--accent);
		font: inherit;
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
		padding: 0.25rem 0.3rem;
	}

	.icon-btn {
		appearance: none;
		background: none;
		border: none;
		color: var(--text-faint);
		cursor: pointer;
		padding: 0.3rem;
		display: inline-flex;
		border-radius: 6px;
	}

	.icon-btn:hover {
		color: var(--text);
		background: var(--border);
	}

	.accent {
		color: var(--accent);
		font-weight: 600;
	}

	.small {
		font-size: 0.75rem;
		display: block;
		margin-top: 0.3rem;
	}

	.error-banner {
		margin: 0 0 1rem;
	}

	.progress {
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
	}

	.progress-head {
		display: flex;
		justify-content: space-between;
		font-size: 0.82rem;
		color: var(--text-muted);
	}

	.track {
		height: 8px;
		border-radius: 999px;
		background: var(--border);
		overflow: hidden;
	}

	.fill {
		height: 100%;
		border-radius: inherit;
		background: var(--accent);
		transition: width 0.2s ease;
	}

	.jobs {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.skeleton-list {
		display: flex;
		flex-direction: column;
		gap: 0.65rem;
	}

	.skeleton-row {
		height: 64px;
		border-radius: var(--radius-sm);
		background: var(--surface);
		border: 1px solid var(--border);
		animation: pulse 1.2s ease-in-out infinite;
	}

	@keyframes pulse {
		50% {
			opacity: 0.5;
		}
	}

	.empty {
		background: var(--surface);
		border: 1.5px dashed var(--border-strong);
		border-radius: var(--radius);
		min-height: 320px;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.85rem;
		text-align: center;
		padding: 2.5rem;
	}

	.empty-icon {
		width: 48px;
		height: 48px;
		border-radius: 13px;
		background: var(--accent-tint);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.empty-title {
		font-size: 1rem;
		font-weight: 700;
	}

	.empty-body {
		max-width: 340px;
		font-size: 0.85rem;
		line-height: 1.5;
	}

	.toolbar {
		display: flex;
		gap: 0.6rem;
		align-items: stretch;
	}

	.toolbar-search {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		gap: 0.55rem;
		border: 1.5px solid var(--border-strong);
		border-radius: var(--radius-sm);
		padding: 0.5rem 0.7rem;
		background: var(--surface);
	}

	.toolbar-search:focus-within {
		border-color: var(--accent);
	}

	.search-input {
		flex: 1;
		min-width: 0;
		border: none;
		padding: 0;
		font-size: 0.9rem;
		background: transparent;
		color: var(--text);
		font-family: var(--font-sans);
	}

	.search-input:focus-visible {
		outline: none;
	}

	.filter-chips {
		display: flex;
		align-items: center;
		gap: 0.2rem;
		flex-wrap: wrap;
	}

	.no-matches {
		padding: 1.5rem 0;
		text-align: center;
	}

	.job-list {
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
	}

	.job-row {
		background: var(--surface);
		border: 1px solid var(--border);
		border-left: 3px solid var(--border-strong);
		border-radius: var(--radius-sm);
		padding: 0.8rem 1rem 0.8rem 0.85rem;
		display: grid;
		grid-template-columns: 34px minmax(0, 1fr) auto 2.5rem 16px;
		align-items: center;
		gap: 0.85rem;
		color: var(--text);
		transition:
			border-color 0.15s,
			transform 0.15s;
	}

	.job-row:hover {
		border-color: var(--border-strong);
		color: var(--text);
	}

	.job-row:hover .chev {
		transform: translateX(2px);
	}

	.chev {
		transition: transform 0.15s;
	}

	.job-row.accent-border {
		border-left-color: var(--accent);
	}

	.job-row.danger-border {
		border-left-color: var(--danger);
	}

	.row-icon {
		width: 34px;
		height: 34px;
		border-radius: 8px;
		background: var(--bg);
		color: var(--text-muted);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.row-main {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		min-width: 0;
		font-size: 0.85rem;
	}

	.meta-line {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.3rem 0.5rem;
	}

	.signal {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.05rem 0.45rem;
		border-radius: 999px;
		font-size: 0.72rem;
		font-weight: 600;
		white-space: nowrap;
		background: var(--bg);
		color: var(--text-muted);
		border: 1px solid var(--border);
	}

	.signal.paid {
		background: var(--success-tint);
		color: var(--success);
		border-color: transparent;
	}

	.signal.opened {
		background: var(--accent-tint);
		color: var(--accent-tint-text);
		border-color: transparent;
	}

	.signal.unopened {
		font-weight: 500;
		color: var(--text-faint);
	}

	.signal.off {
		background: var(--danger-tint);
		color: var(--danger);
		border-color: transparent;
	}

	.recipient {
		font-size: 0.9rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.when {
		font-size: 0.78rem;
		text-align: right;
	}

	@media (max-width: 520px) {
		.job-row {
			grid-template-columns: 34px minmax(0, 1fr) auto;
			gap: 0.7rem;
		}
		.when,
		.chev {
			display: none;
		}
	}

	.bold {
		font-weight: 600;
	}

	.pagination {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding-top: 0.5rem;
		flex-wrap: wrap;
	}

	.pagination-controls {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}

	.page-btn {
		appearance: none;
		background: none;
		border: none;
		color: var(--text-muted);
		font-size: 0.82rem;
		font-family: var(--font-sans);
		cursor: pointer;
		padding: 0.3rem 0.5rem;
		border-radius: var(--radius-sm);
	}

	.page-btn:hover:not(:disabled) {
		color: var(--text);
	}

	.page-btn:disabled {
		color: var(--text-hint);
		cursor: not-allowed;
	}

	.page-of {
		font-size: 0.8rem;
	}
</style>
