<script lang="ts">
	// First-run setup: company name, then logo + placement, then done. The
	// /app layout sends a new account here until it is finished or skipped.
	import { goto } from '$app/navigation';
	import { i18n } from '$lib/i18n.svelte';
	import { api } from '$lib/api';
	import { brand } from '$lib/stores.svelte';
	import { toasts } from '$lib/toast.svelte';
	import { renderWatermarkText } from '$lib/format';
	import type { Brand } from '$lib/types';
	import BrandPreview from '$lib/components/BrandPreview.svelte';
	import LogoPicker from '$lib/components/LogoPicker.svelte';
	import PositionPicker from '$lib/components/PositionPicker.svelte';
	import { onMount } from 'svelte';

	let step = $state(1);
	let b = $state<Brand | null>(null);
	let company = $state('');
	let busy = $state(false);

	onMount(async () => {
		b = brand.value ?? (await brand.load());
		if (b) company = b.companyName;
	});

	async function saveBrand(patch: Partial<Brand>) {
		if (!b) return;
		const next = await api.updateBrand({
			companyName: patch.companyName ?? b.companyName,
			position: patch.position ?? b.position,
			opacityPct: b.opacityPct,
			logoScalePct: b.logoScalePct,
			textTemplate: b.textTemplate
		});
		b = next;
		brand.set(next);
	}

	async function next() {
		busy = true;
		try {
			if (step === 1) await saveBrand({ companyName: company.trim() });
			if (step === 2) {
				const done = await api.completeOnboarding();
				b = done;
				brand.set(done);
			}
			step++;
		} catch {
			toasts.show(i18n.t('common.error'), { kind: 'error' });
		} finally {
			busy = false;
		}
	}

	async function skip() {
		try {
			brand.set(await api.completeOnboarding());
		} catch {
			// Not fatal - the dashboard works either way.
		}
		goto('/app');
	}

	async function setPosition(p: Brand['position']) {
		if (!b) return;
		b.position = p; // instant preview; saved with the step
		try {
			await saveBrand({ position: p });
		} catch {
			toasts.show(i18n.t('common.error'), { kind: 'error' });
		}
	}

	let sampleText = $derived(
		b ? renderWatermarkText(b.textTemplate, company, i18n.t('brand.sampleRecipient')) : ''
	);
</script>

<svelte:head>
	<title>{i18n.t('welcome.title')} · SADE</title>
</svelte:head>

<div class="container page">
	<div class="card wizard">
		{#if !b}
			<p class="muted">{i18n.t('common.loading')}</p>
		{:else}
			<div class="top">
				<span class="faint step">{i18n.t('welcome.step', { n: step })}</span>
				<div class="bar"><div class="fill" style="width:{(step / 3) * 100}%"></div></div>
			</div>

			{#if step === 1}
				<div class="head">
					<h1>{i18n.t('welcome.title')}</h1>
					<p class="muted">{i18n.t('welcome.subtitle')}</p>
				</div>
				<form
					class="body"
					onsubmit={(e) => {
						e.preventDefault();
						next();
					}}
				>
					<label for="company">{i18n.t('welcome.step1Title')}</label>
					<!-- svelte-ignore a11y_autofocus -->
					<input id="company" type="text" maxlength="80" autofocus placeholder={i18n.t('brand.companyPlaceholder')} bind:value={company} />
					<span class="faint hint">{i18n.t('welcome.step1Body')}</span>
					<div class="actions">
						<button type="button" class="link-btn" onclick={skip}>{i18n.t('welcome.skip')}</button>
						<button class="btn" type="submit" disabled={busy || !company.trim()}>{i18n.t('welcome.next')}</button>
					</div>
				</form>
			{:else if step === 2}
				<div class="head">
					<h1>{i18n.t('welcome.step2Title')}</h1>
					<p class="muted">{i18n.t('welcome.step2Body')}</p>
				</div>
				<div class="split">
					<div class="body">
						<LogoPicker brand={b} onchange={(nb) => ((b = nb), brand.set(nb))} />
						<span class="label">{i18n.t('brand.position')}</span>
						<PositionPicker value={b.position} onchange={setPosition} />
					</div>
					<BrandPreview
						logoUrl={b.hasLogo ? api.brandLogoUrl(b.logoVersion) : ''}
						position={b.position}
						opacityPct={b.opacityPct}
						logoScalePct={b.logoScalePct}
						text={sampleText}
					/>
				</div>
				<div class="actions">
					<button type="button" class="link-btn" onclick={() => (step = 1)}>{i18n.t('welcome.back')}</button>
					<button class="btn" type="button" onclick={next} disabled={busy}>{i18n.t('welcome.next')}</button>
				</div>
			{:else}
				<span class="done-icon" aria-hidden="true">
					<svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="var(--success)" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"
						><path d="M5 12.5 L10 17.5 L19 7" /></svg
					>
				</span>
				<div class="head">
					<h1>{i18n.t('welcome.step3Title')}</h1>
					<p class="muted">{i18n.t('welcome.step3Body')}</p>
				</div>
				<div class="actions end">
					<a class="btn" href="/app">{i18n.t('welcome.start')}</a>
				</div>
			{/if}
		{/if}
	</div>
</div>

<style>
	.page {
		padding: 2.5rem 1.75rem 3rem;
	}

	@media (max-width: 520px) {
		.page {
			padding: 1.25rem 1rem 3rem;
		}
	}

	.wizard {
		max-width: 760px;
		margin: 0 auto;
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
	}

	.top {
		display: flex;
		flex-direction: column;
		gap: 0.45rem;
	}

	.step {
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
	}

	.bar {
		height: 4px;
		border-radius: 999px;
		background: var(--border);
		overflow: hidden;
	}

	.fill {
		height: 100%;
		background: var(--accent);
		transition: width 0.25s ease;
	}

	.head {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	h1 {
		margin: 0;
		font-size: 1.45rem;
		font-weight: 700;
	}

	.head p {
		margin: 0;
		font-size: 0.9rem;
		line-height: 1.5;
	}

	.body {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.label {
		display: block;
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.03em;
		margin-top: 0.6rem;
		color: var(--text-muted);
		text-transform: uppercase;
	}

	.hint {
		font-size: 0.78rem;
		line-height: 1.45;
	}

	.split {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1.2fr);
		gap: 1.25rem;
		align-items: start;
	}

	@media (max-width: 700px) {
		.split {
			grid-template-columns: minmax(0, 1fr);
		}
	}

	.actions {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		margin-top: 0.5rem;
	}

	.actions.end {
		justify-content: flex-end;
	}

	.link-btn {
		appearance: none;
		background: none;
		border: none;
		font: inherit;
		font-size: 0.88rem;
		color: var(--text-muted);
		cursor: pointer;
		padding: 0.3rem;
	}

	.link-btn:hover {
		color: var(--text);
	}

	.done-icon {
		width: 52px;
		height: 52px;
		border-radius: 14px;
		background: var(--success-tint);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
</style>
