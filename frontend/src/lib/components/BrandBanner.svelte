<script lang="ts">
	// Brings back an operator who skipped the welcome setup: shown while the
	// account has neither a company name nor a logo, until dismissed.
	import { i18n } from '$lib/i18n.svelte';
	import { brand } from '$lib/stores.svelte';
	import { onMount } from 'svelte';

	const KEY = 'sade:brandBannerDismissed';
	let dismissed = $state(true);

	onMount(() => {
		try {
			dismissed = !!localStorage.getItem(KEY);
		} catch {
			dismissed = false; // no storage - still fine to show
		}
	});

	let show = $derived(
		!dismissed &&
			brand.status === 'ready' &&
			!!brand.value &&
			brand.value.onboarded &&
			!brand.value.companyName &&
			!brand.value.hasLogo
	);

	function dismiss() {
		dismissed = true;
		try {
			localStorage.setItem(KEY, '1');
		} catch {
			// best-effort
		}
	}
</script>

{#if show}
	<div class="banner">
		<span class="icon" aria-hidden="true">
			<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"
				><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M7 15 h6 M7 11 h10" /></svg
			>
		</span>
		<div class="text">
			<strong>{i18n.t('banner.brandTitle')}</strong>
			<span class="muted">{i18n.t('banner.brandBody')}</span>
		</div>
		<div class="actions">
			<a class="btn btn-ink btn-sm" href="/app/settings">{i18n.t('banner.brandCta')}</a>
			<button type="button" class="later" onclick={dismiss}>{i18n.t('banner.dismiss')}</button>
		</div>
	</div>
{/if}

<style>
	.banner {
		display: flex;
		align-items: center;
		gap: 0.9rem;
		padding: 12px 12px 12px 14px;
		margin-bottom: 20px;
		border-radius: var(--radius);
		background: var(--surface);
		box-shadow: var(--shadow-card);
	}

	@media (max-width: 640px) {
		.banner {
			flex-wrap: wrap;
		}
		.actions {
			width: 100%;
		}
	}

	.icon {
		width: 36px;
		height: 36px;
		flex-shrink: 0;
		border-radius: 50%;
		background: var(--accent-tint);
		color: var(--accent-tint-text);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.text {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		font-size: 0.9rem;
		line-height: 1.45;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}


	.later {
		appearance: none;
		background: none;
		border: none;
		color: var(--text-muted);
		font: inherit;
		font-size: 0.88rem;
		cursor: pointer;
		min-height: 44px;
		padding: 0 0.6rem;
	}

	.later:hover {
		color: var(--text);
	}
</style>
