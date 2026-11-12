<script lang="ts">
	// Nudges an operator with no passkey to add one. Shown only where
	// passkeys actually work (server origin + browser), and dismissable.
	import { i18n } from '$lib/i18n.svelte';
	import { api } from '$lib/api';
	import { passkeysSupported } from '$lib/webauthn';
	import { onMount } from 'svelte';

	const KEY = 'sade:passkeyBannerDismissed';
	let show = $state(false);

	onMount(async () => {
		try {
			if (localStorage.getItem(KEY)) return;
		} catch {
			// no storage - still fine to show
		}
		if (!passkeysSupported()) return;
		try {
			const [cfg, list] = await Promise.all([api.authConfig(), api.passkeys()]);
			show = cfg.passkeys && list.length === 0;
		} catch {
			show = false;
		}
	});

	function dismiss() {
		show = false;
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
				><circle cx="8" cy="15" r="4" /><path d="M10.8 12.2 L19 4 M16 7 l3 3 M14 9 l2 2" /></svg
			>
		</span>
		<div class="text">
			<strong>{i18n.t('banner.passkeyTitle')}</strong>
			<span class="muted">{i18n.t('banner.passkeyBody')}</span>
		</div>
		<div class="actions">
			<a class="btn btn-ink btn-sm" href="/app/settings/security#passkeys">{i18n.t('banner.passkeyCta')}</a>
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
