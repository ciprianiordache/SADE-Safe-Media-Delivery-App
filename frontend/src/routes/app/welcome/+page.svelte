<script lang="ts">
	// First-run setup: company name, then logo + placement, then done. The
	// /app layout sends a new account here until it is finished or skipped.
	// Name and position are kept locally and saved once - on Continue from
	// step 2, or on Skip - so quick clicks never race each other.
	import { goto } from '$app/navigation';
	import { i18n } from '$lib/i18n.svelte';
	import { api } from '$lib/api';
	import { brand } from '$lib/stores.svelte';
	import { toasts } from '$lib/toast.svelte';
	import { renderWatermarkTextFit } from '$lib/format';
	import type { WatermarkPosition } from '$lib/types';
	import BrandPreview from '$lib/components/BrandPreview.svelte';
	import LogoPicker from '$lib/components/LogoPicker.svelte';
	import PositionPicker from '$lib/components/PositionPicker.svelte';
	import { onMount, tick } from 'svelte';

	let step = $state(1);
	let company = $state('');
	let position = $state<WatermarkPosition>('bottom-right');
	let busy = $state(false);
	let seeded = false;
	let heading = $state<HTMLHeadingElement | null>(null);

	let b = $derived(brand.value);

	// Seed the local fields once, from the first brand we get.
	$effect(() => {
		if (b && !seeded) {
			seeded = true;
			company = b.companyName;
			position = b.position;
		}
	});

	onMount(() => {
		if (brand.status !== 'ready') brand.load();
	});

	async function saveFields() {
		if (!b) return;
		if (company.trim() === b.companyName && position === b.position) return;
		brand.set(
			await api.updateBrand({
				companyName: company.trim(),
				position,
				opacityPct: b.opacityPct,
				logoScalePct: b.logoScalePct,
				textTemplate: b.textTemplate
			})
		);
	}

	async function goTo(n: number) {
		step = n;
		await tick();
		heading?.focus(); // screen readers hear the new step
	}

	async function next() {
		busy = true;
		try {
			if (step === 2) {
				await saveFields();
				brand.set(await api.completeOnboarding());
			}
			await goTo(step + 1);
		} catch {
			toasts.show(i18n.t('common.error'), { kind: 'error' });
		} finally {
			busy = false;
		}
	}

	async function skip() {
		busy = true;
		try {
			await saveFields(); // keep what was already typed or picked
			brand.set(await api.completeOnboarding());
		} catch {
			// Not fatal - the dashboard works either way; say so and go on.
			toasts.show(i18n.t('common.error'), { kind: 'error' });
		}
		goto('/app');
	}

	// The preview shortens like the server: company first, never the recipient.
	let sampleFor = $derived((budget: number) =>
		b ? renderWatermarkTextFit(b.textTemplate, company, i18n.t('brand.sampleRecipient'), budget) : ''
	);
</script>

<svelte:head>
	<title>{i18n.t('welcome.title')} · SADE</title>
</svelte:head>

<div class="container page">
	<div class="card wizard">
		{#if brand.status === 'error' && !b}
			<div class="head" role="alert">
				<h1>{i18n.t('welcome.title')}</h1>
				<p class="muted">{i18n.t('brand.loadError')}</p>
			</div>
			<div class="actions">
				<a class="link-btn" href="/app">{i18n.t('welcome.skip')}</a>
				<button class="btn" type="button" onclick={() => brand.load()}>{i18n.t('common.retry')}</button>
			</div>
		{:else if !b}
			<div class="skeleton" aria-busy="true" aria-label={i18n.t('common.loading')}>
				<span class="sk sk-line"></span>
				<span class="sk sk-title"></span>
				<span class="sk sk-input"></span>
			</div>
		{:else}
			<div class="top">
				<span class="muted step" aria-hidden="true">
					{step === 1 ? i18n.t('welcome.eyebrow', { n: step }) : i18n.t('welcome.step', { n: step })}
				</span>
				<div
					class="bar"
					role="progressbar"
					aria-valuemin={1}
					aria-valuemax={3}
					aria-valuenow={step}
					aria-valuetext={i18n.t('welcome.step', { n: step })}
				>
					<div class="fill" style="width:{(step / 3) * 100}%"></div>
				</div>
			</div>

			{#if step === 1}
				<form
					class="body"
					onsubmit={(e) => {
						e.preventDefault();
						next();
					}}
				>
					<h1 id="company-q" tabindex="-1" bind:this={heading}>{i18n.t('welcome.step1Title')}</h1>
					<p class="muted lead">{i18n.t('welcome.step1Body')}</p>
					<!-- svelte-ignore a11y_autofocus -->
					<input
						id="company"
						type="text"
						maxlength="80"
						autofocus
						aria-labelledby="company-q"
						aria-describedby="company-hint"
						placeholder={i18n.t('brand.companyPlaceholder')}
						bind:value={company}
					/>
					<span id="company-hint" class="muted hint">{i18n.t('welcome.step1Hint')}</span>
					<div class="preview-1">
						<BrandPreview logoUrl="" {position} opacityPct={b.opacityPct} logoScalePct={b.logoScalePct} textFor={sampleFor} />
					</div>
					<div class="actions">
						<button type="button" class="link-btn" onclick={skip} disabled={busy}>{i18n.t('welcome.skip')}</button>
						<button class="btn" type="submit" disabled={busy}>{i18n.t('welcome.next')}</button>
					</div>
				</form>
			{:else if step === 2}
				<div class="head">
					<h1 tabindex="-1" bind:this={heading}>{i18n.t('welcome.step2Title')}</h1>
					<p class="muted">{i18n.t('welcome.step2Body')}</p>
				</div>
				<div class="split">
					<div class="body">
						<LogoPicker brand={b} onchange={(nb) => brand.set(nb)} />
						<span class="label" id="pos-label">{i18n.t('brand.position')}</span>
						<PositionPicker value={position} onchange={(p) => (position = p)} labelledby="pos-label" />
					</div>
					<div class="body">
						<BrandPreview
							logoUrl={b.hasLogo ? api.brandLogoUrl(b.logoVersion) : ''}
							{position}
							opacityPct={b.opacityPct}
							logoScalePct={b.logoScalePct}
							textFor={sampleFor}
						/>
						{#if !b.hasLogo}
							<span class="muted hint">{i18n.t('brand.noLogoNote')}</span>
						{/if}
					</div>
				</div>
				<div class="actions">
					<button type="button" class="link-btn" onclick={() => goTo(1)} disabled={busy}>{i18n.t('welcome.back')}</button>
					<button class="btn" type="button" onclick={next} disabled={busy}>{i18n.t('welcome.next')}</button>
				</div>
			{:else}
				<span class="done-icon" aria-hidden="true">
					<svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="var(--success)" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"
						><path d="M5 12.5 L10 17.5 L19 7" /></svg
					>
				</span>
				<div class="head">
					<h1 tabindex="-1" bind:this={heading}>{i18n.t('welcome.step3Title')}</h1>
					{#if b.hasLogo}
						<p class="muted">{i18n.t('welcome.step3Body')}</p>
					{:else}
						<p class="muted">
							{i18n.t('welcome.step3BodyNoLogo')}
							<a href="/app/settings">{i18n.t('welcome.addLogoLater')}</a>
						</p>
					{/if}
				</div>
				<div class="actions end">
					<a class="btn" href="/app?upload=1">{i18n.t('welcome.start')}</a>
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
		font-size: 0.8rem;
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

	.lead {
		margin: 0 0 0.35rem;
		font-size: 0.9rem;
		line-height: 1.5;
	}

	.preview-1 {
		margin-top: 0.6rem;
		max-width: 520px;
	}

	h1:focus {
		outline: none;
	}

	.skeleton {
		display: flex;
		flex-direction: column;
		gap: 0.8rem;
	}

	.sk {
		display: block;
		border-radius: var(--radius-sm);
		background: var(--border);
	}

	.sk-line {
		width: 30%;
		height: 0.7rem;
	}

	.sk-title {
		width: 60%;
		height: 1.5rem;
	}

	.sk-input {
		width: 100%;
		height: 2.6rem;
	}

	.link-btn {
		min-height: 44px;
		display: inline-flex;
		align-items: center;
		text-decoration: none;
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
