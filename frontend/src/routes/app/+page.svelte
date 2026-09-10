<script lang="ts">
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { isAllowedFile } from '$lib/types';
	import type { Job, JobStatus, WatermarkKind } from '$lib/types';
	import { relativeTime } from '$lib/format';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import StatusDot from '$lib/components/StatusDot.svelte';
	import { onDestroy, onMount } from 'svelte';

	// --- data -----------------------------------------------------------
	let jobs = $state<Job[]>([]);
	let jobsLoading = $state(true);

	async function loadJobs() {
		try {
			jobs = await api.listJobs(0, 200);
		} finally {
			jobsLoading = false;
		}
	}

	let pollHandle: ReturnType<typeof setInterval> | undefined;
	onMount(() => {
		loadJobs();
		pollHandle = setInterval(() => {
			if (jobs.some((j) => j.status === 'pending' || j.status === 'processing')) loadJobs();
		}, 4000);
	});
	onDestroy(() => {
		if (pollHandle) clearInterval(pollHandle);
	});

	// --- upload form ------------------------------------------------------
	let file = $state<File | null>(null);
	let fileInput = $state<HTMLInputElement | undefined>();
	let fileError = $derived(file && !isAllowedFile(file.name) ? i18n.t('dashboard.fileUnsupported') : '');
	let recipientEmail = $state('');
	let watermarkKind = $state<WatermarkKind>('both');
	let watermarkText = $state('');
	let submitting = $state(false);
	let uploadError = $state('');

	function onFileChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		file = input.files?.[0] ?? null;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!file || fileError) return;
		uploadError = '';
		submitting = true;
		try {
			const job = await api.createJob({
				file,
				recipientEmail,
				watermarkKind,
				watermarkText: watermarkText || undefined
			});
			jobs = [job, ...jobs];
			file = null;
			recipientEmail = '';
			watermarkText = '';
			if (fileInput) fileInput.value = '';
		} catch (err) {
			uploadError = err instanceof ApiError ? err.message : i18n.t('dashboard.uploadError');
		} finally {
			submitting = false;
		}
	}

	// --- list controls ----------------------------------------------------
	type StatusFilter = 'all' | JobStatus;
	type Sort = 'newest' | 'oldest' | 'status';

	let search = $state('');
	let statusFilter = $state<StatusFilter>('all');
	let sort = $state<Sort>('newest');
	let density = $state<'comfortable' | 'compact'>('comfortable');
	let view = $state<'list' | 'grid'>('list');
	let rowsPerPage = $state(10);
	let currentPage = $state(1);

	let searchedJobs = $derived(
		search.trim()
			? jobs.filter((j) => j.recipientEmail.toLowerCase().includes(search.trim().toLowerCase()))
			: jobs
	);

	let statusCounts = $derived({
		all: searchedJobs.length,
		pending: searchedJobs.filter((j) => j.status === 'pending').length,
		processing: searchedJobs.filter((j) => j.status === 'processing').length,
		done: searchedJobs.filter((j) => j.status === 'done').length,
		failed: searchedJobs.filter((j) => j.status === 'failed').length
	});

	const statusOrder: Record<JobStatus, number> = { processing: 0, pending: 1, failed: 2, done: 3 };

	let filteredJobs = $derived(
		statusFilter === 'all' ? searchedJobs : searchedJobs.filter((j) => j.status === statusFilter)
	);

	let sortedJobs = $derived(
		[...filteredJobs].sort((a, b) => {
			if (sort === 'status') return statusOrder[a.status] - statusOrder[b.status];
			const diff = new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime();
			return sort === 'oldest' ? diff : -diff;
		})
	);

	let totalPages = $derived(Math.max(1, Math.ceil(sortedJobs.length / rowsPerPage)));
	let pageJobs = $derived(
		sortedJobs.slice((currentPage - 1) * rowsPerPage, (currentPage - 1) * rowsPerPage + rowsPerPage)
	);

	$effect(() => {
		// Any of these narrowing the result set can strand currentPage past
		// the new last page - search/statusFilter/rowsPerPage are the reactive
		// deps that matter here.
		void search;
		void statusFilter;
		void rowsPerPage;
		currentPage = 1;
	});
</script>

<div class="container page">
	<h1>{i18n.t('dashboard.title')}</h1>
	<p class="subtitle muted">{i18n.t('dashboard.subtitle')}</p>

	<div class="layout">
		<!-- new job -->
		<section class="card new-job">
			<span class="section-label">{i18n.t('dashboard.newJob')}</span>
			<form onsubmit={submit}>
				<div class="field">
					<label for="recipient">{i18n.t('dashboard.recipientLabel')}</label>
					<input id="recipient" type="email" required bind:value={recipientEmail} />
				</div>

				<div class="field">
					<label for="file">{i18n.t('dashboard.fileLabel')}</label>
					<button
						type="button"
						class="dropzone"
						class:has-error={!!fileError}
						onclick={() => fileInput?.click()}
					>
						<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke={fileError ? 'var(--danger)' : 'var(--text-hint)'} stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"
							><path d="M12 15 V4 M8 8 L12 4 L16 8" /><path d="M5 15 v4 h14 v-4" /></svg
						>
						{#if file}
							<span class="filename" class:error-text={!!fileError}>
								{file.name}{fileError ? ` — ${fileError}` : ''}
							</span>
						{:else}
							<span class="muted">
								{i18n.t('dashboard.dropText')}
								<span class="accent">{i18n.t('dashboard.dropBrowse')}</span>
							</span>
						{/if}
						<span class="faint mono types">{i18n.t('dashboard.allowedTypes')}</span>
					</button>
					<input
						id="file"
						type="file"
						required
						bind:this={fileInput}
						onchange={onFileChange}
						style="display:none"
					/>
					{#if fileError}
						<span class="error-text small">{i18n.t('dashboard.fileChooseSupported')}</span>
					{/if}
				</div>

				<div class="field">
					<label for="kind">{i18n.t('dashboard.watermarkLabel')}</label>
					<div class="segmented" id="kind">
						<button
							type="button"
							aria-pressed={watermarkKind === 'both'}
							onclick={() => (watermarkKind = 'both')}>{i18n.t('dashboard.watermarkKind.both')}</button
						>
						<button
							type="button"
							aria-pressed={watermarkKind === 'logo'}
							onclick={() => (watermarkKind = 'logo')}>{i18n.t('dashboard.watermarkKind.logo')}</button
						>
						<button
							type="button"
							aria-pressed={watermarkKind === 'text'}
							onclick={() => (watermarkKind = 'text')}>{i18n.t('dashboard.watermarkKind.text')}</button
						>
					</div>
				</div>

				<div class="field">
					<label for="text"
						>{i18n.t('dashboard.watermarkTextLabel')}
						<span class="optional faint">{i18n.t('dashboard.optional')}</span></label
					>
					<input
						id="text"
						type="text"
						placeholder={i18n.t('dashboard.watermarkTextPlaceholder')}
						bind:value={watermarkText}
					/>
					<span class="faint small">{i18n.t('dashboard.watermarkHint')}</span>
				</div>

				{#if uploadError}
					<p class="error-text">{uploadError}</p>
				{/if}
				<button class="btn" type="submit" disabled={submitting || !file || !!fileError} style="width:100%">
					{submitting ? i18n.t('dashboard.submitting') : i18n.t('dashboard.submit')}
				</button>
			</form>
		</section>

		<!-- jobs area -->
		<section class="jobs">
			{#if jobsLoading}
				<p class="muted">{i18n.t('common.loading')}</p>
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
				<div class="toolbar-search">
					<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="var(--text-faint)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
						><circle cx="11" cy="11" r="7" /><path d="M20 20 L16 16" /></svg
					>
					<input
						type="text"
						class="search-input"
						placeholder={i18n.t('dashboard.searchPlaceholder')}
						bind:value={search}
					/>
				</div>

				<div class="toolbar-row">
					<div class="filter-chips">
						<button
							type="button"
							class="filter-chip"
							aria-pressed={statusFilter === 'all'}
							onclick={() => (statusFilter = 'all')}
						>
							{i18n.t('dashboard.filter.all')} <span class="count">{statusCounts.all}</span>
						</button>
						<button
							type="button"
							class="filter-chip"
							aria-pressed={statusFilter === 'pending'}
							onclick={() => (statusFilter = 'pending')}
						>
							{i18n.t('dashboard.filter.pending')} <span class="count">{statusCounts.pending}</span>
						</button>
						<button
							type="button"
							class="filter-chip"
							aria-pressed={statusFilter === 'processing'}
							onclick={() => (statusFilter = 'processing')}
						>
							{i18n.t('dashboard.filter.processing')}
							<span class="count">{statusCounts.processing}</span>
						</button>
						<button
							type="button"
							class="filter-chip"
							aria-pressed={statusFilter === 'done'}
							onclick={() => (statusFilter = 'done')}
						>
							{i18n.t('dashboard.filter.done')} <span class="count">{statusCounts.done}</span>
						</button>
						<button
							type="button"
							class="filter-chip"
							aria-pressed={statusFilter === 'failed'}
							onclick={() => (statusFilter = 'failed')}
						>
							{i18n.t('dashboard.filter.failed')} <span class="count">{statusCounts.failed}</span>
						</button>
					</div>

					<div class="toolbar-controls">
						<div class="toolbar-select">
							<span class="faint">{i18n.t('dashboard.sortLabel')}</span>
							<select bind:value={sort}>
								<option value="newest">{i18n.t('dashboard.sort.newest')}</option>
								<option value="oldest">{i18n.t('dashboard.sort.oldest')}</option>
								<option value="status">{i18n.t('dashboard.sort.status')}</option>
							</select>
						</div>
						<div class="icon-toggle" title={i18n.t('dashboard.densityComfortable')}>
							<button
								type="button"
								aria-pressed={density === 'comfortable'}
								aria-label={i18n.t('dashboard.densityComfortable')}
								onclick={() => (density = 'comfortable')}
							>
								<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"
									><path d="M4 6 H20 M4 12 H20 M4 18 H20" /></svg
								>
							</button>
							<button
								type="button"
								aria-pressed={density === 'compact'}
								aria-label={i18n.t('dashboard.densityCompact')}
								onclick={() => (density = 'compact')}
							>
								<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"
									><path d="M4 5 H20 M4 9.5 H20 M4 14 H20 M4 18.5 H20" /></svg
								>
							</button>
						</div>
						<div class="icon-toggle">
							<button
								type="button"
								aria-pressed={view === 'list'}
								aria-label={i18n.t('dashboard.viewList')}
								onclick={() => (view = 'list')}
							>
								<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
									><path d="M8 6 H21 M8 12 H21 M8 18 H21 M3.5 6 h.01 M3.5 12 h.01 M3.5 18 h.01" /></svg
								>
							</button>
							<button
								type="button"
								aria-pressed={view === 'grid'}
								aria-label={i18n.t('dashboard.viewGrid')}
								onclick={() => (view = 'grid')}
							>
								<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
									><rect x="3" y="3" width="8" height="8" rx="1.5" /><rect
										x="13"
										y="3"
										width="8"
										height="8"
										rx="1.5"
									/><rect x="3" y="13" width="8" height="8" rx="1.5" /><rect
										x="13"
										y="13"
										width="8"
										height="8"
										rx="1.5"
									/></svg
								>
							</button>
						</div>
					</div>
				</div>

				{#if sortedJobs.length === 0}
					<p class="muted no-matches">{i18n.t('dashboard.noMatches')}</p>
				{:else if view === 'grid'}
					<div class="grid">
						{#each pageJobs as job (job.id)}
							<a class="grid-card" href={`/app/jobs/${job.id}`}>
								<div class="grid-thumb">
									<span class="thumb-dot"><StatusDot status={job.status} /></span>
								</div>
								<div class="grid-body">
									<div class="grid-top">
										<span class="mono grid-recipient">{job.recipientEmail}</span>
										<span class="faint mono">{relativeTime(job.createdAt, i18n.locale)}</span>
									</div>
									<span class="faint grid-meta">{job.mediaType}</span>
									<StatusBadge status={job.status} />
								</div>
							</a>
						{/each}
					</div>
				{:else if density === 'compact'}
					<div class="card compact-list">
						{#each pageJobs as job (job.id)}
							<a class="compact-row" href={`/app/jobs/${job.id}`}>
								<StatusDot status={job.status} />
								<span class="compact-meta">
									<span class="bold">{job.recipientEmail}</span>
									<span class="faint"> · {job.mediaType} · {job.watermarkKind}</span>
								</span>
								<StatusBadge status={job.status} />
								<span class="faint mono">{relativeTime(job.createdAt, i18n.locale)}</span>
							</a>
						{/each}
					</div>
				{:else}
					<div class="comfortable-list">
						{#each pageJobs as job (job.id)}
							<a
								class="comfortable-row"
								class:accent-border={job.status === 'processing'}
								class:danger-border={job.status === 'failed'}
								href={`/app/jobs/${job.id}`}
							>
								<div class="row-main">
									<span class="bold">{job.recipientEmail}</span>
									<span class="faint">{job.mediaType} · {job.watermarkKind}</span>
								</div>
								<StatusBadge status={job.status} />
								<span class="faint mono">{relativeTime(job.createdAt, i18n.locale)}</span>
								<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="var(--text-hint)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
									><path d="M9 6 L15 12 L9 18" /></svg
								>
							</a>
						{/each}
					</div>
				{/if}

				<div class="pagination">
					<span class="faint mono">
						{(currentPage - 1) * rowsPerPage + 1}–{Math.min(currentPage * rowsPerPage, sortedJobs.length)}
						{i18n.t('dashboard.of')}
						{sortedJobs.length}
					</span>
					<div class="pagination-controls">
						<div class="toolbar-select">
							<span class="faint">{i18n.t('dashboard.rowsLabel')}</span>
							<select bind:value={rowsPerPage}>
								<option value={10}>10</option>
								<option value={25}>25</option>
								<option value={50}>50</option>
							</select>
						</div>
						<button
							type="button"
							class="page-btn"
							disabled={currentPage <= 1}
							onclick={() => (currentPage = Math.max(1, currentPage - 1))}
						>
							{i18n.t('dashboard.prev')}
						</button>
						<span class="faint mono page-of">{currentPage} / {totalPages}</span>
						<button
							type="button"
							class="page-btn"
							disabled={currentPage >= totalPages}
							onclick={() => (currentPage = Math.min(totalPages, currentPage + 1))}
						>
							{i18n.t('dashboard.next')}
						</button>
					</div>
				</div>
			{/if}
		</section>
	</div>
</div>

<style>
	.page {
		padding: 1.75rem 1.75rem 3rem;
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
			grid-template-columns: 1fr;
		}
	}

	.new-job {
		display: flex;
		flex-direction: column;
		gap: 1.1rem;
	}

	.section-label {
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		color: var(--text-faint);
		text-transform: uppercase;
	}

	.optional {
		text-transform: none;
		font-weight: 400;
	}

	.dropzone {
		width: 100%;
		font-family: var(--font-sans);
	}

	.dropzone .filename {
		font-size: 0.85rem;
		font-weight: 600;
	}

	.dropzone .types {
		font-size: 0.7rem;
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

	.jobs {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
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
		max-width: 320px;
		font-size: 0.85rem;
		line-height: 1.5;
	}

	.toolbar-search {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		border: 1.5px solid var(--border-strong);
		border-radius: var(--radius-sm);
		padding: 0.6rem 0.7rem;
		background: var(--surface);
	}

	.search-input {
		flex: 1;
		border: none;
		padding: 0;
		font-size: 0.9rem;
	}

	.search-input:focus-visible {
		outline: none;
	}

	.toolbar-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.filter-chips {
		display: flex;
		align-items: center;
		gap: 0.2rem;
		flex-wrap: wrap;
	}

	.toolbar-controls {
		display: flex;
		align-items: center;
		gap: 0.45rem;
	}

	.no-matches {
		padding: 1.5rem 0;
		text-align: center;
	}

	.comfortable-list {
		display: flex;
		flex-direction: column;
		gap: 0.65rem;
	}

	.comfortable-row {
		background: var(--surface);
		border: 1px solid var(--border);
		border-left: 3px solid var(--border-strong);
		border-radius: var(--radius-sm);
		padding: 0.9rem 1rem;
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto auto 16px;
		align-items: center;
		gap: 1rem;
		color: var(--text);
	}

	.comfortable-row.accent-border {
		border-left-color: var(--accent);
	}

	.comfortable-row.danger-border {
		border-left-color: var(--danger);
	}

	.row-main {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		min-width: 0;
	}

	.bold {
		font-weight: 600;
	}

	.compact-list {
		display: flex;
		flex-direction: column;
	}

	.compact-row {
		display: grid;
		grid-template-columns: 8px minmax(0, 1fr) auto auto;
		align-items: center;
		gap: 0.75rem;
		padding: 0.6rem 0.85rem;
		border-bottom: 1px solid var(--border);
		color: var(--text);
		font-size: 0.85rem;
	}

	.compact-row:last-child {
		border-bottom: none;
	}

	.compact-meta {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
		gap: 0.75rem;
	}

	.grid-card {
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		overflow: hidden;
		display: flex;
		flex-direction: column;
		color: var(--text);
	}

	.grid-thumb {
		position: relative;
		aspect-ratio: 16 / 9;
		background: var(--text);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.grid-thumb .thumb-dot {
		position: absolute;
		top: 6px;
		right: 6px;
		display: inline-flex;
		border-radius: 999px;
		box-shadow: 0 0 0 3px rgba(0, 0, 0, 0.35);
	}

	.grid-body {
		padding: 0.6rem 0.65rem;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.grid-top {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.4rem;
	}

	.grid-recipient {
		font-size: 0.75rem;
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.grid-meta {
		font-size: 0.72rem;
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
