<script lang="ts">
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { brand } from '$lib/stores.svelte';
	import { toasts } from '$lib/toast.svelte';
	import { renderWatermarkTextFit } from '$lib/format';
	import type { Brand, BrandUpdate } from '$lib/types';
	import SettingsTabs from '$lib/components/SettingsTabs.svelte';
	import BrandPreview from '$lib/components/BrandPreview.svelte';
	import LogoPicker from '$lib/components/LogoPicker.svelte';
	import PositionPicker from '$lib/components/PositionPicker.svelte';
	import { onMount } from 'svelte';
	import { beforeNavigate } from '$app/navigation';

	let current = $state<Brand | null>(null);
	let draft = $state<BrandUpdate | null>(null);
	let saving = $state(false);
	let templateInput = $state<HTMLInputElement | undefined>();

	function edit(b: Brand) {
		current = b;
		brand.set(b);
		draft = {
			companyName: b.companyName,
			position: b.position,
			opacityPct: b.opacityPct,
			logoScalePct: b.logoScalePct,
			textTemplate: b.textTemplate
		};
	}

	async function load() {
		const b = await brand.load();
		if (b) edit(b);
	}

	onMount(() => {
		load();
		// Closing or reloading the tab with unsaved edits: the browser asks.
		const onUnload = (e: BeforeUnloadEvent) => {
			if (dirty) e.preventDefault();
		};
		window.addEventListener('beforeunload', onUnload);
		return () => window.removeEventListener('beforeunload', onUnload);
	});

	// Leaving inside the app with unsaved edits asks first - except a forced
	// trip to the sign-in page (session ended, signed out), which must win.
	beforeNavigate((nav) => {
		if (!dirty || nav.type === 'leave' || nav.to?.url.pathname === '/login') return;
		if (!confirm(i18n.t('brand.leaveConfirm'))) nav.cancel();
	});

	let dirty = $derived(
		!!current &&
			!!draft &&
			(draft.companyName !== current.companyName ||
				draft.position !== current.position ||
				draft.opacityPct !== current.opacityPct ||
				draft.logoScalePct !== current.logoScalePct ||
				draft.textTemplate !== current.textTemplate)
	);

	// A logo upload saves on its own; keep the rest of the draft as typed.
	function logoChanged(b: Brand) {
		const keep = draft;
		edit(b);
		if (keep) draft = keep;
	}

	async function save() {
		if (!draft) return;
		saving = true;
		try {
			edit(await api.updateBrand(draft));
			toasts.show(i18n.t('brand.saved'));
		} catch (err) {
			toasts.show(err instanceof ApiError && err.status === 400 ? i18n.t('dashboard.errorInvalid') : i18n.t('common.error'), {
				kind: 'error'
			});
		} finally {
			saving = false;
		}
	}

	// Insert a {variable} at the cursor of the template field.
	function insert(token: string) {
		if (!draft) return;
		const el = templateInput;
		const t = draft.textTemplate;
		const at = el?.selectionStart ?? t.length;
		const end = el?.selectionEnd ?? at;
		draft.textTemplate = (t.slice(0, at) + token + t.slice(end)).slice(0, 120);
		requestAnimationFrame(() => {
			el?.focus();
			el?.setSelectionRange(at + token.length, at + token.length);
		});
	}

	// The preview shortens like the server: company first, never the recipient.
	let sampleFor = $derived((budget: number) =>
		draft ? renderWatermarkTextFit(draft.textTemplate, draft.companyName, i18n.t('brand.sampleRecipient'), budget) : ''
	);
</script>

<svelte:head>
	<title>{i18n.t('brand.title')} · SADE</title>
</svelte:head>

<div class="container page">
	<SettingsTabs />

	{#if (!current || !draft) && brand.status === 'error'}
		<div class="card load-error" role="alert">
			<p class="muted">{i18n.t('brand.loadError')}</p>
			<button class="btn" type="button" onclick={load}>{i18n.t('common.retry')}</button>
		</div>
	{:else if !current || !draft}
		<p class="muted" aria-busy="true">{i18n.t('common.loading')}</p>
	{:else}
		<p class="muted intro">{i18n.t('brand.subtitle')}</p>
		<div class="layout">
			<section class="card form">
				<div class="field">
					<label for="company">{i18n.t('brand.company')}</label>
					<input id="company" type="text" maxlength="80" placeholder={i18n.t('brand.companyPlaceholder')} bind:value={draft.companyName} />
					<span class="muted hint">{i18n.t('brand.companyHint')}</span>
				</div>

				<div class="field">
					<span class="label" id="logo">{i18n.t('brand.logo')}</span>
					<LogoPicker brand={current} onchange={logoChanged} />
				</div>

				<div class="field">
					<span class="label" id="pos-label">{i18n.t('brand.position')}</span>
					<PositionPicker value={draft.position} onchange={(p) => draft && (draft.position = p)} labelledby="pos-label" />
					{#if draft.position === 'tiled'}
						<span class="muted hint">{i18n.t('brand.tiledHint')}</span>
					{/if}
				</div>

				<div class="sliders">
					<div class="field">
						<label for="opacity">{i18n.t('brand.opacity')} <span class="value mono">{draft.opacityPct}%</span></label>
						<input id="opacity" type="range" min="10" max="100" step="5" bind:value={draft.opacityPct} />
					</div>
					<div class="field">
						<label for="size">{i18n.t('brand.size')} <span class="value mono">{draft.logoScalePct}%</span></label>
						<input id="size" type="range" min="5" max="50" step="1" bind:value={draft.logoScalePct} disabled={!current.hasLogo} />
						{#if current.hasLogo}
							<span class="muted hint">{i18n.t('brand.sizeHint')}</span>
						{:else}
							<span class="muted hint"><a href="#logo">{i18n.t('brand.sizeNoLogo')}</a></span>
						{/if}
					</div>
				</div>

				<div class="field">
					<label for="template">{i18n.t('brand.text')}</label>
					<input id="template" class="mono" type="text" maxlength="120" bind:this={templateInput} bind:value={draft.textTemplate} />
					<div class="vars">
						<span class="muted">{i18n.t('brand.insert')}</span>
						<button type="button" onclick={() => insert('{company}')}>{i18n.t('brand.var.company')}</button>
						<button type="button" onclick={() => insert('{recipient}')}>{i18n.t('brand.var.recipient')}</button>
						<button type="button" onclick={() => insert('{date}')}>{i18n.t('brand.var.date')}</button>
					</div>
					<span class="muted hint">{i18n.t('brand.textHint')}</span>
				</div>

				<div class="save-row" class:pending={dirty}>
					{#if dirty}<span class="muted hint">{i18n.t('brand.unsaved')}</span>{/if}
					<button class="btn" type="button" onclick={save} disabled={!dirty || saving}>
						{saving ? i18n.t('brand.saving') : i18n.t('brand.save')}
					</button>
				</div>
			</section>

			<section class="card preview">
				<span class="section-label">{i18n.t('brand.preview')}</span>
				<BrandPreview
					logoUrl={current.hasLogo ? api.brandLogoUrl(current.logoVersion) : ''}
					position={draft.position}
					opacityPct={draft.opacityPct}
					logoScalePct={draft.logoScalePct}
					textFor={sampleFor}
				/>
				<span class="muted hint">{i18n.t('brand.previewHint')}</span>
				{#if !current.hasLogo}
					<span class="muted hint">{i18n.t('brand.noLogoNote')}</span>
				{/if}
			</section>
		</div>
	{/if}
</div>

<style>
	.page {
		padding: 1.5rem 1.75rem 3rem;
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
	}

	@media (max-width: 520px) {
		.page {
			padding: 1.25rem 1rem 3rem;
		}
	}

	.intro {
		margin: 0;
		font-size: 0.88rem;
	}

	.layout {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1.1fr);
		gap: 1.25rem;
		align-items: start;
	}

	/* Phones/tablets: the preview stays in view (compact, pinned to the top)
	   while the sliders below it move, and Save is pinned to the bottom while
	   there is something to save. */
	@media (max-width: 860px) {
		.layout {
			grid-template-columns: minmax(0, 1fr);
		}
		.preview {
			order: -1;
			position: sticky;
			top: 0;
			z-index: 5;
			max-height: 35vh;
			overflow: hidden;
			padding-top: 0.6rem;
			padding-bottom: 0.6rem;
		}
		.preview .hint {
			display: none;
		}
		.save-row.pending {
			position: sticky;
			bottom: 0;
			z-index: 5;
			margin: 0.5rem -1rem -1rem;
			padding: 0.7rem 1rem;
			background: var(--surface);
			border-top: 1px solid var(--border);
		}
	}

	@media (min-width: 861px) {
		.preview {
			position: sticky;
			top: 1rem;
		}
	}

	.form,
	.preview {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.preview {
		gap: 0.75rem;
	}

	.label {
		display: block;
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.03em;
		margin-bottom: 0.35rem;
		color: var(--text-muted);
		text-transform: uppercase;
	}

	.section-label {
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		color: var(--text-muted);
		text-transform: uppercase;
	}

	.load-error {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}

	.load-error p {
		margin: 0;
	}

	.hint {
		display: block;
		font-size: 0.75rem;
		line-height: 1.45;
		margin-top: 0.3rem;
	}

	.value {
		float: right;
		text-transform: none;
		color: var(--text);
	}

	.sliders {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0 1.25rem;
	}

	@media (max-width: 520px) {
		.sliders {
			grid-template-columns: 1fr;
		}
	}

	input[type='range'] {
		width: 100%;
		accent-color: var(--accent);
	}

	.vars {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.35rem;
		margin-top: 0.45rem;
		font-size: 0.78rem;
	}

	.vars button {
		appearance: none;
		border: 1px solid var(--border-strong);
		background: var(--bg);
		color: var(--text);
		border-radius: 999px;
		padding: 0.15rem 0.6rem;
		font: inherit;
		font-size: 0.75rem;
		cursor: pointer;
	}

	.vars button:hover {
		border-color: var(--accent);
		color: var(--accent);
	}

	.save-row {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 0.5rem;
	}

	.save-row .hint {
		margin: 0;
	}
</style>
