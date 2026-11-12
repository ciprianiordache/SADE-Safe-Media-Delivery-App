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

			elements = stripe.elements({
				clientSecret,
				// Stripe's own labels and card errors in the page's language.
				locale: i18n.locale === 'en' ? 'en' : 'ro',
				appearance: appearance(),
				fonts: [{ cssSrc: 'https://fonts.googleapis.com/css2?family=Reddit+Sans:wght@400..600&display=swap' }]
			});
			// Romania first: the country field starts on RO, not on a guess
			// from the browser.
			const paymentElement = elements.create('payment', {
				defaultValues: { billingDetails: { address: { country: 'RO' } } }
			});
			if (mountEl) paymentElement.mount(mountEl);
			ready = true;
		})();

		// Follow the page's theme: the toggle sets data-theme on <html>, and
		// "system" follows the OS setting.
		const restyle = () => elements?.update({ appearance: appearance() });
		const observer = new MutationObserver(restyle);
		observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
		const media = matchMedia('(prefers-color-scheme: dark)');
		media.addEventListener('change', restyle);

		return () => {
			cancelled = true;
			observer.disconnect();
			media.removeEventListener('change', restyle);
		};
	});

	// The card fields read the page's live tokens, so they match light/dark.
	function appearance() {
		const styles = getComputedStyle(document.documentElement);
		const v = (name: string, fallback: string) => styles.getPropertyValue(name).trim() || fallback;
		return {
			theme: 'stripe' as const,
			variables: {
				colorPrimary: v('--accent', '#C2501A'),
				colorBackground: v('--surface', '#FFFFFF'),
				colorText: v('--text', '#211C18'),
				colorTextSecondary: v('--text-muted', '#6E655B'),
				colorTextPlaceholder: v('--text-faint', '#9C9287'),
				colorDanger: v('--danger', '#C1443A'),
				fontFamily: 'Reddit Sans, ui-sans-serif, system-ui, sans-serif',
				borderRadius: '8px',
				spacingUnit: '4px'
			}
		};
	}

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
			const whole = cents % 100 === 0;
			return new Intl.NumberFormat(i18n.locale === 'ro' ? 'ro-RO' : 'en-US', {
				style: 'currency',
				currency: currency.toUpperCase(),
				currencyDisplay: 'narrowSymbol',
				minimumFractionDigits: whole ? 0 : 2,
				maximumFractionDigits: whole ? 0 : 2
			}).format(cents / 100);
		} catch {
			return `${(cents / 100).toFixed(2)} ${currency.toUpperCase()}`;
		}
	}
</script>

<form onsubmit={submit} class="payment-form" aria-label={i18n.t('preview.payWithCard')}>
	<div bind:this={mountEl} class="element-mount" class:loading={!ready}></div>
	{#if error}
		<p class="error-text">{error}</p>
	{/if}
	<button class="btn btn-block" type="submit" disabled={!ready || submitting}>
		{submitting ? i18n.t('preview.processingPayment') : `${i18n.t('preview.payButton')} ${formatPrice(amountCents, currency)}`}
	</button>
	<p class="disclaimer muted small">
		<svg class="lock" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
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

	/* Holds the card fields' place while Stripe.js loads. */
	.element-mount {
		min-height: 128px;
		border-radius: var(--radius-sm);
	}

	.element-mount.loading {
		background: var(--fill);
		animation: sk-pulse 1.2s ease-in-out infinite;
	}

	.small {
		font-size: 0.85rem;
		line-height: 1.45;
	}

	.disclaimer {
		display: flex;
		align-items: flex-start;
		gap: 0.45rem;
		margin: 0;
	}

	.lock {
		flex: none;
		margin-top: 0.2em;
	}
</style>
