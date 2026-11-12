<script lang="ts">
	import { formatBytes } from '$lib/format';
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
	import type { PaymentStatus, ShareInfo } from '$lib/types';
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

	// The link's own state comes first, so a dead link never shows a player:
	// 'checking' until /api/share/info answers, then 'ok', or why it is not.
	type LinkState = 'checking' | 'ok' | 'expired' | 'invalid' | 'disabled';
	let linkState = $state<LinkState>('checking');
	let info = $state<ShareInfo>({});

	let payment = $state<PaymentStatus | null>(null);
	let paymentLoading = $state(true);
	let clientSecret = $state('');
	let intentError = $state('');
	let showSide = $derived(paymentLoading || !!payment?.enabled);
	// Once the original is unlocked, the watermarked copy stops being the
	// first thing to download.
	let paidOriginal = $derived(!!(payment?.enabled && payment.paid && payment.originalUrl));

	async function loadStatus() {
		payment = await api.paymentStatus(token);
	}

	onMount(async () => {
		try {
			info = await api.shareInfo(token);
			linkState = 'ok';
		} catch (err) {
			if (err instanceof ApiError && err.status === 410) {
				linkState = err.message === 'disabled' ? 'disabled' : 'expired';
				paymentLoading = false;
				return;
			}
			if (err instanceof ApiError && err.status === 404) {
				linkState = 'invalid';
				paymentLoading = false;
				return;
			}
			// Network trouble or an older server: try the file anyway.
			linkState = 'ok';
		}
		try {
			await loadStatus();
			if (payment?.enabled && !payment.paid) {
				const intent = await api.paymentIntent(token);
				clientSecret = intent.clientSecret;
			}
		} catch (err) {
			if (err instanceof ApiError && err.status === 410) {
				linkState = 'disabled';
				return;
			}
			// Paid in the meantime (another tab, or between the two calls):
			// just show the unlocked state.
			if (err instanceof ApiError && err.status === 409) {
				await loadStatus().catch(() => {});
				return;
			}
			// Payment is a bonus feature on top of the preview - if any of this
			// fails, just leave the section hidden rather than break the page
			// the recipient actually came for.
			if (payment?.enabled) {
				// Never the server's own words: they may be English or technical.
				intentError = i18n.t('preview.unlockError');
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

<svelte:head>
	<title>{info.fileName || i18n.t('preview.title')} · SADE</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="shell">
	<MarkBar siteLink={false} />

	<main class="container content">
		{#if linkState === 'checking'}
			<div class="intro" aria-busy="true" aria-label={i18n.t('preview.checkingLink')}>
				<span class="sk" style="height:2.6rem;width:min(520px,80%)"></span>
				<span class="sk" style="height:1.1rem;width:min(360px,60%)"></span>
			</div>
		{:else if linkState !== 'ok'}
			<div class="win state-card">
				<h1>{i18n.t(`preview.${linkState}Title`)}</h1>
				<p class="muted">{i18n.t(`preview.${linkState}Body`)}</p>
			</div>
		{:else}
			<div class="intro">
				<h1 class="file-title">{info.fileName || i18n.t('preview.title')}</h1>
				{#if info.sender}
					<p class="sender">{i18n.t('preview.sentBy')} <span class="mono">{info.sender}</span></p>
				{/if}
				<p class="muted context">
					{i18n.t('preview.context')}
					{#if payment?.enabled && !payment.paid}{i18n.t('preview.contextPay')}{/if}
				</p>
			</div>

			<!-- The window of the marketing page's hero: the file left, the unlock right. -->
			<section class="win viewer" class:single={!showSide}>
				<div class="win-bar">
					<span class="win-dots" aria-hidden="true"><i></i><i></i><i></i></span>
					<b>{i18n.t('preview.barTitle')}</b>
					<span class="sp"></span>
					{#if !paidOriginal}
						<a class="bar-dl" href={downloadUrl} aria-label={i18n.t('preview.download')}>
							<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 4 V15 M8 11 L12 15 L16 11" /><path d="M5 19 h14" /></svg>
							<span>{i18n.t('preview.download')}</span>
						</a>
					{/if}
				</div>
				<div class="viewer-body">
					<div class="media" class:audio={stage === 'audio'}>
						{#if stage === 'video'}
							<!-- svelte-ignore a11y_media_has_caption -->
							<video controls src={previewUrl} onerror={next}>
								<track kind="captions" />
							</video>
						{:else if stage === 'audio'}
							<audio controls src={previewUrl} onerror={next}></audio>
						{:else if stage === 'image'}
							<img src={previewUrl} alt={i18n.t('preview.imageAlt')} onerror={next} />
						{:else}
							<p class="stage-note">{i18n.t('preview.unsupported')}</p>
							<a class="btn btn-ghost" href={previewUrl} target="_blank" rel="noreferrer">
								{i18n.t('preview.openDirect')}
							</a>
						{/if}
					</div>

					{#if showSide}
						<div class="side-col">
							{#if paymentLoading}
								<div class="sk-block" aria-busy="true" aria-label={i18n.t('preview.checkingPayment')}>
									<span class="sk" style="height:1.4rem;width:70%"></span>
									<span class="sk" style="height:44px"></span>
									<span class="sk" style="height:44px"></span>
								</div>
							{:else if payment?.enabled}
								<div class="unlock">
									{#if payment.paid && payment.originalUrl}
										<p class="ok">
										<i aria-hidden="true"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12.5 L10 17.5 L19 7" /></svg></i>
										{i18n.t('preview.unlocked')}
									</p>
										<a class="btn btn-ink btn-block" href={payment.originalUrl}>
											{i18n.t('preview.downloadOriginal')}
										</a>
										<p class="muted small">{i18n.t('preview.keepsWorking')}</p>
										<a class="secondary-link" href={downloadUrl}>{i18n.t('preview.downloadWatermarked')}</a>
									{:else}
										<h2 class="unlock-title">
										{payment.originalSize
											? i18n.t('preview.unlockTitleSize', { size: formatBytes(payment.originalSize) })
											: i18n.t('preview.unlockTitle')}
									</h2>
										<p class="muted small">{i18n.t('preview.unlockIntro')}</p>
										<p class="muted small">{i18n.t('preview.afterPay')}</p>
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
			</section>
		{/if}
	</main>

	<footer class="footer muted">{i18n.t('preview.footer')}</footer>
</div>

<style>
	.shell {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
	}

	.content {
		flex: 1;
		width: 100%;
		display: flex;
		flex-direction: column;
		gap: 28px;
		padding-top: 48px;
		padding-bottom: 48px;
	}

	@media (max-width: 639px) {
		.content {
			padding-top: 24px;
			gap: 20px;
		}
	}

	.intro {
		display: flex;
		flex-direction: column;
		gap: 12px;
		max-width: 64ch;
	}

	h1 {
		margin: 0;
		font-size: clamp(2rem, 1.3rem + 2.4vw, 3.2rem);
		line-height: 1.04;
		letter-spacing: -0.035em;
	}

	/* A file name can be one long word ("nunta-andreea-final-v3.mp4"). */
	.file-title {
		font-size: clamp(1.7rem, 1.2rem + 2vw, 2.8rem);
		overflow-wrap: anywhere;
	}

	.sender {
		margin: 0;
		font-size: 1.04rem;
		overflow-wrap: anywhere;
	}

	/* Text on the dark stage: light, not the page's muted ink (~2:1 there). */
	.stage-note {
		margin: 0;
		padding: 0 20px;
		text-align: center;
		color: #e9e1d6;
	}

	.media.audio .stage-note {
		color: var(--text-muted);
	}

	.context {
		margin: 0;
		font-size: 1.04rem;
		line-height: 1.55;
		text-wrap: pretty;
	}

	.bar-dl {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 6px;
		min-height: 44px;
		min-width: 44px;
		color: var(--text);
		font-weight: 500;
	}

	.bar-dl:hover {
		color: var(--accent-ink);
	}

	@media (max-width: 559px) {
		.bar-dl span {
			display: none;
		}
	}

	.viewer-body {
		display: grid;
		grid-template-columns: minmax(0, 1.55fr) minmax(300px, 1fr);
	}

	.viewer.single .viewer-body {
		grid-template-columns: minmax(0, 1fr);
	}

	@media (max-width: 899px) {
		.viewer-body {
			grid-template-columns: minmax(0, 1fr);
		}
	}

	/* The file on a dark stage, in its own shape: a phone video stays tall,
	   a landscape one fills the width. */
	.media {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.75rem;
		min-height: 240px;
		background: #17130f;
	}

	.media.audio {
		background: var(--fill);
		padding: 32px 20px;
	}

	.media video,
	.media img {
		display: block;
		width: auto;
		height: auto;
		max-width: 100%;
		max-height: 74svh;
	}

	.media video {
		min-width: min(100%, 280px);
	}

	.media audio {
		width: 100%;
	}

	.side-col {
		padding: 22px;
		border-left: 1px solid var(--line);
	}

	@media (max-width: 899px) {
		.side-col {
			border-left: none;
			border-top: 1px solid var(--line);
			padding: 18px 16px 20px;
		}
	}

	.unlock,
	.sk-block {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.unlock-title {
		margin: 0;
		font-size: 1.15rem;
		font-weight: 500;
	}

	/* "Plată primită", as in the hero window. */
	.ok {
		display: flex;
		align-items: center;
		gap: 8px;
		margin: 0;
		font-weight: 500;
		color: var(--success);
	}

	.ok i {
		width: 22px;
		height: 22px;
		border-radius: 50%;
		background: var(--success-tint);
		display: grid;
		place-items: center;
		font-style: normal;
	}

	.small {
		font-size: 0.88rem;
		line-height: 1.5;
		margin: 0;
	}

	.state-card {
		max-width: 460px;
		margin: 24px auto 0;
		padding: 32px;
		text-align: center;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.state-card h1 {
		font-size: 1.8rem;
	}

	.state-card p {
		margin: 0;
		line-height: 1.5;
	}

	.secondary-link {
		align-self: center;
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		font-size: 0.9rem;
		font-weight: 500;
		color: var(--text-muted);
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.secondary-link:hover {
		color: var(--text);
	}

	.footer {
		text-align: center;
		font-size: 0.85rem;
		padding: 16px 20px 28px;
	}
</style>
