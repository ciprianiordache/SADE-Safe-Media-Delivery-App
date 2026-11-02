<script lang="ts">
	// The operator's view of one job's delivery: the recipient's link (copy),
	// resend the email, switch the links off/on, and what the recipient did
	// (internals/app/share.OperatorHandler).
	import { i18n, type MessageKey } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { toasts } from '$lib/toast.svelte';
	import { fullDate, relativeTime } from '$lib/format';
	import type { Delivery, DeliveryEventKind, JobStatus } from '$lib/types';
	import { onDestroy, onMount } from 'svelte';

	let { jobId, status, recipient }: { jobId: string; status: JobStatus; recipient: string } = $props();

	let d = $state<Delivery | null>(null);
	let resending = $state(false);
	let switching = $state(false);
	let armed = $state(false);

	async function load() {
		try {
			d = await api.delivery(jobId);
		} catch {
			// The rest of the page still works without this card's data.
		}
	}

	let poll: ReturnType<typeof setInterval> | undefined;
	onMount(() => {
		load();
		// The recipient may open the link while the operator is watching.
		poll = setInterval(() => {
			if (!document.hidden) load();
		}, 15000);
	});
	onDestroy(() => clearInterval(poll));

	// Reload as soon as processing finishes - that is when the link and the
	// "sent" entry appear.
	let lastStatus = '';
	$effect(() => {
		if (lastStatus && lastStatus !== status) load();
		lastStatus = status;
	});

	async function copy() {
		if (!d?.previewUrl) return;
		try {
			await navigator.clipboard.writeText(d.previewUrl);
			toasts.show(i18n.t('delivery.copied'));
		} catch {
			toasts.show(i18n.t('delivery.copyFailed'), { kind: 'error' });
		}
	}

	async function resend() {
		resending = true;
		try {
			await api.resend(jobId);
			toasts.show(i18n.t('delivery.resent', { email: recipient }));
			await load();
		} catch (err) {
			toasts.show(
				err instanceof ApiError && err.status === 429 ? i18n.t('delivery.resendLimit') : i18n.t('common.error'),
				{ kind: 'error' }
			);
		} finally {
			resending = false;
		}
	}

	async function setEnabled(enabled: boolean) {
		if (!enabled && !armed) {
			armed = true;
			return;
		}
		armed = false;
		switching = true;
		try {
			await api.setLinks(jobId, enabled);
			toasts.show(i18n.t(enabled ? 'delivery.enabledToast' : 'delivery.disabledToast'));
			await load();
		} catch (err) {
			toasts.show(
				err instanceof ApiError && err.status === 409 ? i18n.t('delivery.paidNote') : i18n.t('common.error'),
				{ kind: 'error' }
			);
		} finally {
			switching = false;
		}
	}

	const ICON: Record<DeliveryEventKind, string> = {
		sent: 'mail',
		resent: 'mail',
		viewed: 'eye',
		downloaded: 'down',
		paid: 'card',
		original_downloaded: 'down',
		links_disabled: 'off',
		links_enabled: 'on'
	};

	function eventText(kind: DeliveryEventKind, detail?: string): string {
		return i18n.t(`delivery.event.${kind}` as MessageKey, { detail: detail ?? '' });
	}
</script>

<section class="card delivery">
	<div class="head">
		<span class="section-label">{i18n.t('delivery.title')}</span>
		{#if d}
			<span class="pill" class:off={!d.enabled}>
				{d.enabled ? i18n.t('delivery.active') : i18n.t('delivery.disabled')}
			</span>
		{/if}
	</div>

	{#if !d}
		<p class="faint small">{i18n.t('common.loading')}</p>
	{:else}
		{#if d.enabled}
			{#if d.ready && d.previewUrl}
				<div class="link-box">
					<label for="share-link">{i18n.t('delivery.linkLabel')}</label>
					<div class="link-row">
						<input id="share-link" class="mono" type="text" readonly value={d.previewUrl} onfocus={(e) => e.currentTarget.select()} />
						<button type="button" class="btn copy" onclick={copy}>{i18n.t('delivery.copy')}</button>
					</div>
					{#if d.expiresAt}
						<span class="faint small">{i18n.t('delivery.expires', { date: fullDate(d.expiresAt, i18n.locale) })}</span>
					{/if}
				</div>
				<button type="button" class="btn btn-ghost" onclick={resend} disabled={resending}>
					<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
						><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M3.5 6.5 L12 13 L20.5 6.5" /></svg
					>
					{resending ? i18n.t('delivery.resending') : i18n.t('delivery.resend')}
				</button>
			{:else}
				<p class="faint small">{i18n.t('delivery.notReady')}</p>
			{/if}

			{#if d.paid}
				<p class="faint small">{i18n.t('delivery.paidNote')}</p>
			{:else}
				<div class="switch-row">
					<button type="button" class="text-btn danger" class:armed onclick={() => setEnabled(false)} onblur={() => setTimeout(() => (armed = false), 200)} disabled={switching}>
						{armed ? i18n.t('delivery.disableConfirm') : i18n.t('delivery.disable')}
					</button>
					{#if armed}<span class="faint small">{i18n.t('delivery.disableHint')}</span>{/if}
				</div>
			{/if}
		{:else}
			<button type="button" class="btn" onclick={() => setEnabled(true)} disabled={switching}>
				{i18n.t('delivery.enable')}
			</button>
		{/if}

		<div class="history">
			<span class="section-label">{i18n.t('delivery.history')}</span>
			{#if d.events.length === 0}
				<p class="faint small">{i18n.t('delivery.historyEmpty')}</p>
			{:else}
				<ol class="timeline">
					{#each d.events as e, i (i)}
						<li class:paid={e.kind === 'paid'}>
							<span class="dot" aria-hidden="true">
								{#if ICON[e.kind] === 'mail'}
									<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M3.5 6.5 L12 13 L20.5 6.5" /></svg>
								{:else if ICON[e.kind] === 'eye'}
									<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12 C5 6 19 6 22 12 C19 18 5 18 2 12 Z" /><circle cx="12" cy="12" r="3" /></svg>
								{:else if ICON[e.kind] === 'down'}
									<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4 V15 M8 11 L12 15 L16 11" /><path d="M5 19 h14" /></svg>
								{:else if ICON[e.kind] === 'card'}
									<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="5" width="20" height="14" rx="2" /><path d="M2 10 h20" /></svg>
								{:else}
									<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 14 a4 4 0 0 0 5.7 0 l3-3 a4 4 0 0 0 -5.7 -5.7 l-1 1 M14 10 a4 4 0 0 0 -5.7 0 l-3 3 a4 4 0 0 0 5.7 5.7 l1 -1" /></svg>
								{/if}
							</span>
							<span class="ev">
								<span class="ev-text">{eventText(e.kind, e.detail)}</span>
								<span class="faint mono when" title={fullDate(e.createdAt, i18n.locale)}>{relativeTime(e.createdAt, i18n.locale)}</span>
							</span>
						</li>
					{/each}
				</ol>
			{/if}
		</div>
	{/if}
</section>

<style>
	.delivery {
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
	}

	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.section-label {
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		color: var(--text-faint);
		text-transform: uppercase;
	}

	.pill {
		padding: 0.15rem 0.55rem;
		border-radius: 999px;
		background: var(--success-tint);
		color: var(--success);
		font-size: 0.72rem;
		font-weight: 600;
	}

	.pill.off {
		background: var(--danger-tint);
		color: var(--danger);
	}

	.small {
		font-size: 0.78rem;
		line-height: 1.45;
		margin: 0;
	}

	.link-box {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.link-box label {
		margin: 0;
	}

	.link-row {
		display: flex;
		gap: 0.4rem;
	}

	.link-row input {
		flex: 1;
		min-width: 0;
		font-size: 0.75rem;
		padding: 0.5rem 0.6rem;
		background: var(--bg);
	}

	.copy {
		padding: 0.5rem 0.8rem;
		font-size: 0.8rem;
	}

	.switch-row {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.text-btn {
		appearance: none;
		background: none;
		border: none;
		padding: 0;
		font: inherit;
		font-size: 0.82rem;
		font-weight: 600;
		cursor: pointer;
		text-align: left;
		color: var(--text-muted);
	}

	.text-btn.danger:hover,
	.text-btn.armed {
		color: var(--danger);
	}

	.history {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		padding-top: 0.9rem;
		border-top: 1px solid var(--border);
	}

	.timeline {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
	}

	.timeline li {
		display: flex;
		gap: 0.6rem;
		align-items: flex-start;
	}

	.dot {
		width: 22px;
		height: 22px;
		flex-shrink: 0;
		border-radius: 999px;
		background: var(--bg);
		border: 1px solid var(--border);
		color: var(--text-muted);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.timeline li.paid .dot {
		background: var(--success-tint);
		border-color: transparent;
		color: var(--success);
	}

	.ev {
		flex: 1;
		min-width: 0;
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
		font-size: 0.82rem;
		line-height: 1.4;
		padding-top: 0.15rem;
	}

	.ev-text {
		overflow-wrap: anywhere;
	}

	.when {
		font-size: 0.72rem;
		white-space: nowrap;
	}
</style>
