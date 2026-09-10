<script lang="ts">
	import { page } from '$app/state';
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import MarkBar from '$lib/components/MarkBar.svelte';

	let email = $state('');
	let submitting = $state(false);
	let sent = $state(false);
	let error = $state('');

	const linkError = page.url.searchParams.get('error') === 'invalid_link';

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		submitting = true;
		try {
			await api.requestLink(email);
			sent = true;
		} catch (err) {
			error = err instanceof ApiError ? err.message : i18n.t('login.errorGeneric');
		} finally {
			submitting = false;
		}
	}
</script>

<div class="container top">
	<MarkBar />
</div>

<div class="container">
	<div class="card form-card">
		<div class="card-head">
			<h1>{i18n.t('login.title')}</h1>
			<p class="muted">{i18n.t('login.subtitle')}</p>
		</div>

		{#if linkError}
			<div class="error-banner">
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" style="flex-shrink:0;margin-top:2px;"
					><path
						d="M12 8 v5 M12 16 h.01 M10.3 4 L3 17 a2 2 0 0 0 1.7 3 h14.6 a2 2 0 0 0 1.7 -3 L13.7 4 a2 2 0 0 0 -3.4 0 Z"
					/></svg
				>
				<span>{i18n.t('login.errorInvalidLink')}</span>
			</div>
		{/if}

		{#if sent}
			<p class="sent-text">{i18n.t('login.sent')}</p>
		{:else}
			<form onsubmit={submit}>
				<div class="field">
					<label for="email">{i18n.t('login.emailLabel')}</label>
					<input
						id="email"
						type="email"
						required
						placeholder={i18n.t('login.emailPlaceholder')}
						bind:value={email}
					/>
				</div>
				{#if error}
					<p class="error-text">{error}</p>
				{/if}
				<button class="btn" type="submit" disabled={submitting} style="width:100%">
					{submitting ? i18n.t('login.submitting') : i18n.t('login.submit')}
				</button>
			</form>
		{/if}
	</div>
</div>

<style>
	.top {
		padding: 1.25rem 1.75rem 0;
	}

	.form-card {
		max-width: 396px;
		margin: 3.5rem auto;
		display: flex;
		flex-direction: column;
		gap: 1.1rem;
	}

	.card-head {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	h1 {
		margin: 0;
		font-size: 1.45rem;
		font-weight: 700;
		letter-spacing: -0.01em;
	}

	.card-head p {
		margin: 0;
		font-size: 0.85rem;
	}

	.sent-text {
		font-size: 0.9rem;
		line-height: 1.5;
	}
</style>
