<script lang="ts">
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import type { Job, WatermarkKind } from '$lib/types';
	import { onDestroy, onMount } from 'svelte';

	let jobs = $state<Job[]>([]);
	let jobsLoading = $state(true);

	let file = $state<File | null>(null);
	let fileInput = $state<HTMLInputElement | undefined>();
	let recipientEmail = $state('');
	let watermarkKind = $state<WatermarkKind>('both');
	let watermarkText = $state('');
	let submitting = $state(false);
	let uploadError = $state('');

	async function loadJobs() {
		try {
			jobs = await api.listJobs();
		} finally {
			jobsLoading = false;
		}
	}

	function onFileChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		file = input.files?.[0] ?? null;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!file) return;
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

	let pollHandle: ReturnType<typeof setInterval> | undefined;

	onMount(() => {
		loadJobs();
		pollHandle = setInterval(() => {
			if (jobs.some((j) => j.status === 'pending' || j.status === 'processing')) {
				loadJobs();
			}
		}, 4000);
	});

	onDestroy(() => {
		if (pollHandle) clearInterval(pollHandle);
	});
</script>

<div class="container">
	<h1>{i18n.t('dashboard.title')}</h1>

	<section class="card">
		<h2>{i18n.t('dashboard.uploadTitle')}</h2>
		<form onsubmit={submit}>
			<div class="field">
				<label for="file">{i18n.t('dashboard.fileLabel')}</label>
				<input id="file" type="file" required bind:this={fileInput} onchange={onFileChange} />
			</div>
			<div class="field">
				<label for="recipient">{i18n.t('dashboard.recipientLabel')}</label>
				<input id="recipient" type="email" required bind:value={recipientEmail} />
			</div>
			<div class="field">
				<label for="kind">{i18n.t('dashboard.watermarkKindLabel')}</label>
				<select id="kind" bind:value={watermarkKind}>
					<option value="both">{i18n.t('dashboard.watermarkKind.both')}</option>
					<option value="logo">{i18n.t('dashboard.watermarkKind.logo')}</option>
					<option value="text">{i18n.t('dashboard.watermarkKind.text')}</option>
				</select>
			</div>
			<div class="field">
				<label for="text">{i18n.t('dashboard.watermarkTextLabel')}</label>
				<input
					id="text"
					type="text"
					placeholder={i18n.t('dashboard.watermarkTextPlaceholder')}
					bind:value={watermarkText}
				/>
			</div>
			{#if uploadError}
				<p class="error-text">{uploadError}</p>
			{/if}
			<button class="btn" type="submit" disabled={submitting || !file}>
				{submitting ? i18n.t('dashboard.submitting') : i18n.t('dashboard.submit')}
			</button>
		</form>
	</section>

	<section class="jobs">
		<h2>{i18n.t('dashboard.jobsTitle')}</h2>
		{#if jobsLoading}
			<p class="muted">{i18n.t('common.loading')}</p>
		{:else if jobs.length === 0}
			<p class="muted">{i18n.t('dashboard.jobsEmpty')}</p>
		{:else}
			<table>
				<thead>
					<tr>
						<th>{i18n.t('dashboard.col.file')}</th>
						<th>{i18n.t('dashboard.col.type')}</th>
						<th>{i18n.t('dashboard.col.status')}</th>
						<th>{i18n.t('dashboard.col.created')}</th>
					</tr>
				</thead>
				<tbody>
					{#each jobs as job (job.id)}
						<tr>
							<td><a href={`/app/jobs/${job.id}`}>{job.recipientEmail}</a></td>
							<td>{job.mediaType}</td>
							<td><span class={`badge badge-${job.status}`}>{i18n.t(`status.${job.status}`)}</span
								></td>
							<td>{new Date(job.createdAt).toLocaleString(i18n.locale)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</section>
</div>

<style>
	h1 {
		margin-top: 0.5rem;
	}

	.jobs {
		margin-top: 2rem;
	}

	form .field:last-of-type {
		margin-bottom: 1.25rem;
	}
</style>
