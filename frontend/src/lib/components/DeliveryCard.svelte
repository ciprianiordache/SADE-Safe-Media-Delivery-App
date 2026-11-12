<script lang="ts">
	// The operator's view of one job's delivery: the recipient's link (copy),
	// resend the email, switch the links off/on, and what the recipient did
	// (internals/app/share.OperatorHandler). Laid out as the open delivery of
	// the marketing page's list: a row of pills, then the history as rows.
	import { i18n, type MessageKey } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { toasts } from '$lib/toast.svelte';
		import { formatPrice, fullDate, relativeTime } from '$lib/format';
	import OverflowMenu from './OverflowMenu.svelte';
	import type { Delivery, DeliveryEventKind, JobStatus } from '$lib/types';
	import { onDestroy, onMount } from 'svelte';

		// ondelete: the file page passes its delete, so the delivery's one "..."
	// menu holds every rare action (the pane beside the list does not).
	let {
		jobId,
		status,
		recipient,
		ondelete,
		deleteDisabled = false,
		deleteTitle
	}: {
		jobId: string;
		status: JobStatus;
		recipient: string;
		ondelete?: () => void;
		deleteDisabled?: boolean;
		deleteTitle?: string;
	} = $props();

	let d = $state<Delivery | null>(null);
	let resending = $state(false);
	let switching = $state(false);
	let armed = $state(false);
	let loadFailed = $state(false);

	async function load() {
		try {
			d = await api.delivery(jobId);
			loadFailed = false;
		} catch {
			// The rest of the page still works without this card's data; say so
			// instead of "Loading..." forever.
			if (!d) loadFailed = true;
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
			toasts.show(err instanceof ApiError && err.status === 429 ? i18n.t('delivery.resendLimit') : i18n.t('common.error'), {
				kind: 'error'
			});
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
			toasts.show(err instanceof ApiError && err.status === 409 ? i18n.t('delivery.paidNote') : i18n.t('common.error'), {
				kind: 'error'
			});
		} finally {
			switching = false;
		}
	}

	// The key moments get the accent dot, as in the mockup's history.
	const KEY: Partial<Record<DeliveryEventKind, true>> = { paid: true, original_downloaded: true };

		function eventText(kind: DeliveryEventKind, detail?: string): string {
		return i18n.t(`delivery.event.${kind}` as MessageKey, { detail: detail ?? '' });
	}

	// Five "opened the preview" rows in a row say one thing: fold a run of
	// the same event into one row ("· de 5 ori", the latest time), which
	// opens on a click. Sent, paid and the switches always keep their row.
	const FOLD: Partial<Record<DeliveryEventKind, true>> = { viewed: true, downloaded: true, original_downloaded: true };
	type Ev = Delivery['events'][number];
	type Run = { key: string; kind: DeliveryEventKind; items: Ev[] };
	let runs = $derived.by(() => {
		const out: Run[] = [];
		for (const e of d?.events ?? []) {
			const last = out[out.length - 1];
			if (last && FOLD[e.kind] && last.kind === e.kind) last.items.push(e);
			else out.push({ key: `${out.length}:${e.kind}`, kind: e.kind, items: [e] });
		}
		return out;
	});
		let unfolded = $state<Record<string, boolean>>({});

	let menuItems = $derived.by(() => {
		const items: { label: string; onclick: () => void; danger?: boolean; disabled?: boolean; title?: string; sep?: boolean }[] = [];
		if (d && d.enabled && d.ready && d.previewUrl && !d.paid) {
			items.push({ label: i18n.t('delivery.disableMenu'), danger: true, onclick: () => (armed = true) });
		}
		if (ondelete) {
			items.push({ label: i18n.t('job.deleteMenu'), danger: true, sep: items.length > 0, disabled: deleteDisabled, title: deleteTitle, onclick: ondelete });
		}
		return items;
	});
</script>

<section class="delivery" aria-labelledby="delivery-title">
	<div class="head">
		<h2 class="section-label" id="delivery-title">{i18n.t('delivery.title')}</h2>
				{#if d}
			<span class="head-end">
				<span class="chip" class:chip-ok={d.enabled} class:chip-bad={!d.enabled}>
					{d.enabled ? i18n.t('delivery.active') : i18n.t('delivery.disabled')}
				</span>
				{#if menuItems.length}<OverflowMenu items={menuItems} />{/if}
			</span>
		{/if}
	</div>

	{#if !d && loadFailed}
		<p class="small load-error" role="alert">
			{i18n.t('delivery.loadError')}
			<button type="button" class="btn btn-line btn-sm" onclick={load}>{i18n.t('common.retry')}</button>
		</p>
	{:else if !d}
		<div class="sk-block" aria-busy="true" aria-label={i18n.t('common.loading')}>
			<span class="sk" style="height:36px;width:70%"></span>
			<span class="sk" style="height:120px"></span>
		</div>
		{:else}
		{#if d.payments && d.amountCents && d.currency}
			{@const price = formatPrice(d.amountCents, d.currency, i18n.locale)}
			<!-- What the delivery is worth: the third part of the package. -->
			<div class="pay" class:is-paid={d.paid}>
				<span class="pay-label">{i18n.t('delivery.payment')}</span>
				{#if d.paid}
					<span class="pay-state">
						<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12.5 L10 17.5 L19 7" /></svg>
						{i18n.t('delivery.paidOn', { price })}
						{#if d.paidAt}<span class="muted">{i18n.t('delivery.paidWhen', { date: fullDate(d.paidAt, i18n.locale) })}</span>{/if}
					</span>
				{:else}
					<span class="pay-state">
						<b>{i18n.t('delivery.unpaid')}</b>
						<span class="muted">{i18n.t('delivery.unpaidPrice', { price })}</span>
					</span>
				{/if}
			</div>
		{/if}
		{#if d.enabled}
			{#if d.ready && d.previewUrl}
				<div class="acts">
					<button type="button" class="btn btn-ink btn-sm" onclick={copy}>
						<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="8" y="8" width="12" height="12" rx="2" /><path d="M16 8 V6 a2 2 0 0 0 -2 -2 H6 a2 2 0 0 0 -2 2 v8 a2 2 0 0 0 2 2 h2" /></svg>
						{i18n.t('delivery.copy')}
					</button>
					<button type="button" class="btn btn-line btn-sm" onclick={resend} disabled={resending} title={i18n.t('delivery.resendNote')}>
						<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M3.5 6.5 L12 13 L20.5 6.5" /></svg>
						{resending ? i18n.t('delivery.resending') : i18n.t('delivery.resend')}
					</button>
										
				</div>
				{#if armed}
					<!-- Two steps, and it stays armed until the operator decides: tabbing
					     away to read the hint must not cancel it. -->
					<div class="armed" role="alert">
						<p class="small" id="disable-hint">{i18n.t('delivery.disableHint')}</p>
						<div class="acts">
							<button type="button" class="btn btn-sm btn-danger" onclick={() => setEnabled(false)} aria-describedby="disable-hint" disabled={switching}>
								{i18n.t('delivery.disable')}
							</button>
							<button type="button" class="btn btn-ghost btn-sm" onclick={() => (armed = false)}>{i18n.t('common.cancel')}</button>
						</div>
					</div>
				{/if}

				<div class="link-box">
					<label for="share-link">{i18n.t('delivery.linkLabel')}</label>
					<input id="share-link" class="mono" type="text" readonly value={d.previewUrl} onfocus={(e) => e.currentTarget.select()} />
										{#if d.expiresAt}
						<span class="muted small">{i18n.t('delivery.expires', { date: fullDate(d.expiresAt, i18n.locale) })}</span>
					{/if}
				</div>
			{:else}
				<p class="muted small">{i18n.t('delivery.notReady')}</p>
			{/if}

			{#if d.paid}
				<p class="muted small">{i18n.t('delivery.paidNote')}</p>
			{/if}
		{:else}
			<div class="acts">
				<button type="button" class="btn btn-ink btn-sm" onclick={() => setEnabled(true)} disabled={switching}>
					{i18n.t('delivery.enable')}
				</button>
			</div>
		{/if}

		<div class="history">
			<h3 class="section-label">{i18n.t('delivery.history')}</h3>
			{#if d.events.length === 0}
				<p class="muted small">{i18n.t('delivery.historyEmpty')}</p>
			{:else}
				<ol class="tl">
										{#each runs as r (r.key)}
						{@const e = r.items[0]}
						{#if r.items.length > 1}
							<li class:is-key={KEY[e.kind]}>
								<button type="button" class="ev-text fold" aria-expanded={!!unfolded[r.key]} onclick={() => (unfolded[r.key] = !unfolded[r.key])}>
									{eventText(e.kind, e.detail)}
									<span class="muted times">{i18n.t('delivery.times', { n: r.items.length })}</span>
									<svg class="chev" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9 L12 15 L18 9" /></svg>
								</button>
								<time class="mono" datetime={e.createdAt} title={fullDate(e.createdAt, i18n.locale)}>{relativeTime(e.createdAt, i18n.locale)}</time>
							</li>
							{#if unfolded[r.key]}
								{#each r.items as x, j (j)}
									<li class="sub">
										<span class="ev-text muted">{fullDate(x.createdAt, i18n.locale)}</span>
										<time class="mono" datetime={x.createdAt}>{relativeTime(x.createdAt, i18n.locale)}</time>
									</li>
								{/each}
							{/if}
						{:else}
							<li class:is-key={KEY[e.kind]}>
								<span class="ev-text">{eventText(e.kind, e.detail)}</span>
								<time class="mono" datetime={e.createdAt} title={fullDate(e.createdAt, i18n.locale)}>{relativeTime(e.createdAt, i18n.locale)}</time>
							</li>
						{/if}
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
		gap: 16px;
		min-width: 0;
	}

		.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.head-end {
		display: inline-flex;
		align-items: center;
		gap: 8px;
	}

		h2,
	h3 {
		margin: 0;
	}

		/* Clear steps over the 0.9rem body: the section 22px, its parts 18px. */
	h2 {
		font-size: 1.375rem;
	}

	h3 {
		font-size: 1.125rem;
	}

	

	.pay {
		display: flex;
		align-items: baseline;
		gap: 12px;
		padding: 10px 12px;
		border-radius: 8px;
		background: var(--fill);
		font-size: 0.9rem;
	}

	.pay-label {
		flex: none;
		color: var(--text-muted);
	}

		/* The state, then what it means on its own line. */
	.pay-state {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		align-items: center;
		gap: 2px 6px;
		min-width: 0;
	}

	.pay-state > :not(svg) {
		grid-column: 2;
	}

		.pay-state > b {
		grid-column: 1 / -1;
	}

	.pay:not(.is-paid) .pay-state > .muted {
		grid-column: 1 / -1;
	}

	.pay-state b {
		font-weight: 500;
	}

	.pay.is-paid {
		background: var(--success-tint);
	}

	.pay.is-paid .pay-state {
		color: var(--success);
	}

	.pay.is-paid .pay-label,
	.pay.is-paid .muted {
		color: color-mix(in oklab, var(--success) 75%, var(--text));
	}

	.armed {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 12px;
		border-radius: 8px;
		background: var(--danger-tint);
	}

	.armed p {
		color: var(--text);
	}

	.btn-danger {
		background: var(--danger);
		color: #fff;
		border-color: transparent;
	}

	.small {
		font-size: 0.85rem;
		line-height: 1.45;
		margin: 0;
	}

	.acts {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	.link-box {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.link-box label {
		margin: 0;
	}

	.link-box input {
		font-size: 0.82rem;
		background: var(--fill);
		color: var(--text-muted);
	}

	

	.load-error {
		color: var(--text);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 12px;
	}

	.sk-block {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.history {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	/* The mockup's history: hairline rows, a dot, the time on the right. */
	.tl {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.tl li {
		display: grid;
		grid-template-columns: 14px minmax(0, 1fr) auto;
		gap: 10px;
		align-items: center;
		min-height: 40px;
		padding: 8px 0;
		border-top: 1px solid var(--line);
		font-size: 0.9rem;
	}

	.tl li:last-child {
		border-bottom: 1px solid var(--line);
	}

	.tl li::before {
		content: '';
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: color-mix(in oklab, var(--text) 22%, var(--surface));
		justify-self: center;
	}

	.tl li.is-key::before {
		background: var(--accent-bright);
	}

	.ev-text {
		overflow-wrap: anywhere;
	}

	.fold {
		appearance: none;
		position: relative;
		display: inline-flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0 6px;
		padding: 0;
		border: none;
		background: none;
		font: inherit;
		color: inherit;
		text-align: left;
		cursor: pointer;
	}

	.fold::after {
		content: '';
		position: absolute;
		inset: -10px -4px;
	}

	.fold:hover .chev,
	.fold:focus-visible .chev {
		color: var(--text);
	}

		.times {
		white-space: nowrap;
	}

	.chev {
		color: var(--text-muted);
		transition: transform 0.16s var(--ease);
	}

	.fold[aria-expanded='true'] .chev {
		transform: rotate(180deg);
	}

	.tl li.sub {
		min-height: 32px;
		padding: 4px 0;
		font-size: 0.85rem;
		border-top-style: dashed;
	}

	.tl li.sub::before {
		width: 4px;
		height: 4px;
	}

	time {
		font-size: 0.85rem;
		color: var(--text-muted);
		white-space: nowrap;
	}
</style>
