<script lang="ts">
	// Public, login-free page for the link SADE emails a recipient. The token
	// itself is the whole authorization (internals/app/share), so this page
	// makes no /api call for the media itself - it just points a player at
	// the Go-served stream. We don't know the media kind up front, so we try
	// <video>, then <audio>, then <img>, and fall back to a plain link.
	//
	// The unlock section is separate: it does call the API (payment status +
	// a PaymentIntent for Stripe Elements), authorised by this same token -
	// see internals/app/payment.
	import { page } from '$app/state';
	import { i18n } from '$lib/i18n.svelte';
	import { api, API_BASE, ApiError } from '$lib/api';
	import type { PaymentStatus } from '$lib/types';
	import MarkBar from '$lib/components/MarkBar.svelte';
	import PaymentForm from '$lib/components/PaymentForm.svelte';
	import { onMount } from 'svelte';

	const token = page.params.token as string;
	const previewUrl = `${API_BASE}/p/${encodeURIComponent(token)}`;
	const downloadUrl = `${API_BASE}/d/${encodeURIComponent(token)}`;
	const returnUrl = `${typeof window !== 'undefined' ? window.location.origin : ''}/preview/${encodeURIComponent(token)}?paid=1`;

	type Stage = 'video' | 'audio' | 'image' | 'none';
	let stage = $state<Stage>('video');

	function next() {
		if (stage === 'video') stage = 'audio';
		else if (stage === 'audio') stage = 'image';
		else stage = 'none';
	}

	let payment = $state<PaymentStatus | null>(null);
	let paymentLoading = $state(true);
	let clientSecret = $state('');
	let intentError = $state('');
	let showSide = $derived(paymentLoading || !!payment?.enabled);

	async function loadStatus() {
		payment = await api.paymentStatus(token);
	}

	onMount(async () => {
		try {
			await loadStatus();
			if (payment?.enabled && !payment.paid) {
				const intent = await api.paymentIntent(token);
				clientSecret = intent.clientSecret;
			}
		} catch (err) {
			// Payment is a bonus feature on top of the preview - if any of this
			// fails, just leave the section hidden rather than break the page
			// the recipient actually came for.
			if (payment?.enabled) {
				intentError = err instanceof ApiError ? err.message : i18n.t('preview.unlockError');
			} else {
				payment = null;
			}
		} finally {
			paymentLoading = false;
		}
	});

	async function onPaid() {
		await loadStatus();
	}
</script>

<div class="shell">
	<div class="top wrap">
		<MarkBar size={20} />
	</div>

	<div class="content wrap">
		<h1>{i18n.t('preview.title')}</h1>

		<div class="layout" class:single={!showSide}>
			<div class="media-col">
				<div class="media">
					{#if stage === 'video'}
						<!-- svelte-ignore a11y_media_has_caption -->
						<video controls src={previewUrl} onerror={next}>
							<track kind="captions" />
						</video>
					{:else if stage === 'audio'}
						<audio controls src={previewUrl} onerror={next}></audio>
					{:else if stage === 'image'}
						<img src={previewUrl} alt="" onerror={next} />
					{:else}
						<p class="muted">{i18n.t('preview.unsupported')}</p>
						<a class="btn btn-ghost" href={previewUrl} target="_blank" rel="noreferrer">
							{i18n.t('preview.openDirect')}
						</a>
					{/if}
				</div>

				<a class="btn btn-ghost" href={downloadUrl} style="width: 100%">{i18n.t('preview.download')}</a>
			</div>

			{#if showSide}
				<div class="side-col">
					{#if paymentLoading}
						<p class="muted small center">{i18n.t('preview.checkingPayment')}</p>
					{:else if payment?.enabled}
						<div class="unlock-card">
							{#if payment.paid && payment.originalUrl}
								<div class="unlocked-head">
									<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--success)" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"
										><path d="M5 12.5 L10 17.5 L19 7" /></svg
									>
									<span class="unlocked-title">{i18n.t('preview.unlocked')}</span>
								</div>
								<a class="btn" href={payment.originalUrl} style="width: 100%">
									{i18n.t('preview.downloadOriginal')}
								</a>
								<p class="muted small">{i18n.t('preview.keepsWorking')}</p>
							{:else}
								<p class="muted small">{i18n.t('preview.unlockIntro')}</p>
								{#if intentError}
									<p class="error-text">{intentError}</p>
								{:else if clientSecret && payment.publishableKey}
									<PaymentForm
										{clientSecret}
										publishableKey={payment.publishableKey}
										amountCents={payment.amountCents}
										currency={payment.currency}
										{returnUrl}
										onSuccess={onPaid}
									/>
								{/if}
							{/if}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	</div>

	<div class="footer faint">DELIVERED SECURELY BY SADE</div>
</div>

<style>
	.shell {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
	}

	.wrap {
		width: 100%;
		max-width: 480px;
		margin: 0 auto;
	}

	@media (min-width: 860px) {
		.wrap {
			max-width: 1080px;
		}
	}

	.top {
		padding: 1.35rem 1.25rem 0;
	}

	.content {
		flex: 1;
		display: flex;
		flex-direction: column;
		justify-content: center;
		gap: 1.25rem;
		padding: 1.25rem;
	}

	h1 {
		margin: 0;
		font-size: 1.2rem;
		font-weight: 700;
	}

	@media (min-width: 860px) {
		h1 {
			font-size: 1.6rem;
		}
	}

	.layout {
		display: flex;
		flex-direction: column;
		gap: 1.1rem;
	}

	@media (min-width: 860px) {
		.layout:not(.single) {
			display: grid;
			grid-template-columns: minmax(0, 1fr) 360px;
			align-items: start;
			gap: 2.25rem;
		}
	}

	.media-col {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.media {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.75rem;
	}

	.media video,
	.media img {
		width: 100%;
		border-radius: var(--radius);
		display: block;
		background: #000;
	}

	.media video {
		aspect-ratio: 16 / 9;
	}

	@media (min-width: 860px) {
		.media video,
		.media img {
			max-height: 640px;
			object-fit: contain;
		}
	}

	.media audio {
		width: 100%;
	}

	@media (min-width: 860px) {
		.media audio {
			padding: 1.5rem;
			border: 1px solid var(--border);
			border-radius: var(--radius);
			background: var(--surface);
		}
	}

	.side-col {
		display: flex;
		flex-direction: column;
	}

	.unlock-card {
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 1.1rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		background: var(--surface);
	}

	@media (min-width: 860px) {
		.unlock-card {
			padding: 1.5rem;
		}
	}

	.unlocked-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.unlocked-title {
		font-weight: 700;
		color: var(--success);
	}

	.small {
		font-size: 0.78rem;
		line-height: 1.5;
		margin: 0;
	}

	.center {
		text-align: center;
	}

	.footer {
		text-align: center;
		font-size: 0.68rem;
		letter-spacing: 0.04em;
		padding: 1rem 1.25rem 1.6rem;
	}
</style>
