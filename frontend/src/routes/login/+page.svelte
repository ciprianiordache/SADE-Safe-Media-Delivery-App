<script lang="ts">
	import { page } from '$app/state';
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';

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

<div class="container">
	<div class="card form-card">
		<h1>{i18n.t('login.title')}</h1>
		<p class="muted">{i18n.t('login.subtitle')}</p>

		{#if linkError}
			<p class="error-text">{i18n.t('login.errorInvalidLink')}</p>
		{/if}

		{#if sent}
			<p>{i18n.t('login.sent')}</p>
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
				<button class="btn" type="submit" disabled={submitting}>
					{submitting ? i18n.t('login.submitting') : i18n.t('login.submit')}
				</button>
			</form>
		{/if}
	</div>
</div>

<style>
	.form-card {
		max-width: 420px;
		margin: 3rem auto;
	}

	h1 {
		margin-top: 0;
	}
</style>
