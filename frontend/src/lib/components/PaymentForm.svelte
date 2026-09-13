<script lang="ts">
	// Card entry via Stripe's Payment Element, mounted directly into this
	// page - unlike Stripe's hosted Checkout, only the actual input fields
	// are Stripe's (required for PCI scope); everything else, including the
	// Elements' own colors/fonts/radius (via the `appearance` option below),
	// is SADE's design. The parent already has clientSecret from
	// api.paymentIntent and publishableKey from api.paymentStatus.
	import { loadStripe, type Stripe, type StripeElements } from '@stripe/stripe-js';
	import { i18n } from '$lib/i18n.svelte';
	import { onMount } from 'svelte';

	let {
		clientSecret,
		publishableKey,
		amountCents,
		currency,
		returnUrl,
		onSuccess
	}: {
		clientSecret: string;
		publishableKey: string;
		amountCents: number;
		currency: string;
		returnUrl: string;
		onSuccess: () => void;
	} = $props();

	let mountEl = $state<HTMLDivElement | undefined>();
	let ready = $state(false);
	let submitting = $state(false);
	let error = $state('');

	let stripe: Stripe | null = null;
	let elements: StripeElements | null = null;

	onMount(() => {
		let cancelled = false;
		(async () => {
			const loaded = await loadStripe(publishableKey);
			if (cancelled || !loaded) return;
			stripe = loaded;

			// Read the page's own live theme so the embedded card fields match
			// light/dark mode instead of only ever looking light.
			const styles = getComputedStyle(document.documentElement);
			const v = (name: string, fallback: string) => styles.getPropertyValue(name).trim() || fallback;

			elements = stripe.elements({
				clientSecret,
				appearance: {
					theme: 'stripe',
					variables: {
						colorPrimary: v('--accent', '#E8631C'),
						colorBackground: v('--surface', '#FFFFFF'),
						colorText: v('--text', '#211C18'),
						colorTextPlaceholder: v('--text-hint', '#B4A997'),
						colorDanger: v('--danger', '#C1443A'),
						fontFamily: 'IBM Plex Sans, ui-sans-serif, system-ui, sans-serif',
						borderRadius: '9px',
						spacingUnit: '4px'
					}
				}
			});
			const paymentElement = elements.create('payment');
			if (mountEl) paymentElement.mount(mountEl);
			ready = true;
		})();
		return () => {
			cancelled = true;
		};
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!stripe || !elements) return;
		error = '';
		submitting = true;
		const { error: confirmError, paymentIntent } = await stripe.confirmPayment({
			elements,
			confirmParams: { return_url: returnUrl },
			redirect: 'if_required'
		});
		if (confirmError) {
			error = confirmError.message ?? i18n.t('preview.unlockError');
			submitting = false;
			return;
		}
		// No redirect happened (the common case for a card that doesn't need
		// extra authentication) - the outcome is already known.
		if (paymentIntent?.status === 'succeeded') {
			onSuccess();
		} else {
			submitting = false;
		}
	}

	function formatPrice(cents: number, currency: string): string {
		try {
			return new Intl.NumberFormat(i18n.locale === 'ro' ? 'ro-RO' : 'en-US', {
				style: 'currency',
				currency: currency.toUpperCase()
			}).format(cents / 100);
		} catch {
			return `${(cents / 100).toFixed(2)} ${currency.toUpperCase()}`;
		}
	}
</script>

<form onsubmit={submit} class="payment-form">
	<span class="section-label">{i18n.t('preview.payWithCard')}</span>
	<div bind:this={mountEl} class="element-mount"></div>
	{#if error}
		<p class="error-text">{error}</p>
	{/if}
	<button class="btn" type="submit" disabled={!ready || submitting} style="width: 100%">
		{submitting ? i18n.t('preview.processingPayment') : `${i18n.t('preview.payButton')} — ${formatPrice(amountCents, currency)}`}
	</button>
	<p class="disclaimer muted small">
		<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
			><rect x="5" y="11" width="14" height="9" rx="1.5" /><path d="M8 11 V8 a4 4 0 0 1 8 0 v3" /></svg
		>
		{i18n.t('preview.cardDisclaimer')}
	</p>
</form>

<style>
	.payment-form {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.section-label {
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		color: var(--text-faint);
		text-transform: uppercase;
	}

	.element-mount {
		min-height: 90px;
	}

	.small {
		font-size: 0.72rem;
		line-height: 1.4;
	}

	.disclaimer {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		margin: 0;
	}
</style>
