<script lang="ts">
	import { authedSrc, webSrc } from '$lib/media';
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { brand } from '$lib/stores.svelte';
	import { burnedDate, renderWatermarkTextFit } from '$lib/format';
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
	// Saved on its own, like a program's preferences: 'saved' shows a quiet
	// tick, 'error' offers a retry. Nothing to press.
	let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
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
		// Closing or reloading the tab in the short wait before a save: the
		// browser asks.
		const onUnload = (e: BeforeUnloadEvent) => {
			if (dirty) e.preventDefault();
		};
		window.addEventListener('beforeunload', onUnload);
		return () => {
			window.removeEventListener('beforeunload', onUnload);
			clearTimeout(saveTimer);
		};
	});

	// Leaving inside the app saves what is waiting instead of asking.
	beforeNavigate((nav) => {
		if (dirty && nav.to?.url.pathname !== '/login') save();
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

		// Every change saves itself after a short pause (typing in a field waits
	// for the pause; a switch or a slider settles at once). The draft is never
	// replaced by the answer, so keys pressed during a save are not lost.
	let saveTimer: ReturnType<typeof setTimeout> | undefined;
	let saveError = $state('');

	$effect(() => {
		if (!dirty || !draft) return;
		// Read every field so a change to any of them restarts the pause.
		void [draft.companyName, draft.textTemplate, draft.position, draft.opacityPct, draft.logoScalePct];
		clearTimeout(saveTimer);
		saveTimer = setTimeout(save, 700);
	});

	async function save() {
		clearTimeout(saveTimer);
		if (!draft || saving) return;
		const sent = { ...draft };
		saving = true;
		saveState = 'saving';
		try {
			const b = await api.updateBrand(sent);
			current = b;
			brand.set(b);
			saveState = 'saved';
			saveError = '';
		} catch (err) {
			saveState = 'error';
			saveError = err instanceof ApiError && err.status === 400 ? i18n.t('dashboard.errorInvalid') : i18n.t('brand.saveFailed');
		} finally {
			saving = false;
		}
		// Changed again while that save was on its way: the effect only reruns
		// on a change, so pick it up here.
		if (saveState === 'saved' && dirty) saveTimer = setTimeout(save, 300);
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

	// The toggle list of the marketing page's "Filigranul contului": what goes
	// on every file. The text pieces are tokens in the template; the
	// recipient's address only turns on - it is what ties a leaked copy to
	// the person it was sent to.
	const SEP = ' · ';
	const ORDER = ['{company}', '{recipient}', '{date}'];

	function setToken(token: string, on: boolean) {
		if (!draft) return;
		const t = draft.textTemplate;
		if (on === t.includes(token)) return;
		if (!on) {
			draft.textTemplate = t.replace(SEP + token, '').replace(token + SEP, '').replace(token, '').trim();
			return;
		}
		// A template made only of tokens is rebuilt in the usual order; one with
		// the operator's own words gets the token added at the end.
		const pure = t
			.split(SEP)
			.map((x) => x.trim())
			.filter(Boolean)
			.every((x) => ORDER.includes(x));
		draft.textTemplate = pure ? ORDER.filter((x) => t.includes(x) || x === token).join(SEP) : t ? t + SEP + token : token;
	}

	let hasCompany = $derived(draft?.textTemplate.includes('{company}') ?? false);
	let hasRecipient = $derived(draft?.textTemplate.includes('{recipient}') ?? false);
	let hasDate = $derived(draft?.textTemplate.includes('{date}') ?? false);
	let tiled = $derived(draft?.position === 'tiled');
	let lastCorner: BrandUpdate['position'] = 'bottom-right';

	function setTiled(on: boolean) {
		if (!draft) return;
		if (on) {
			if (draft.position !== 'tiled') lastCorner = draft.position;
			draft.position = 'tiled';
		} else {
			draft.position = lastCorner;
		}
	}

	let onCount = $derived([current?.hasLogo, hasCompany, hasRecipient, hasDate, tiled].filter(Boolean).length);
	// As it appears on the file (the preview beside it shows the same).
	let today = burnedDate();
	let advancedOpen = $state(false);

	// The preview shortens like the server: company first, never the recipient.
	let sampleFor = $derived((budget: number) =>
		draft ? renderWatermarkTextFit(draft.textTemplate, draft.companyName, i18n.t('brand.sampleRecipient'), budget) : ''
	);
</script>

<svelte:head>
	<title>{i18n.t('brand.title')} · SADE</title>
</svelte:head>

<div class="container page">
	<SettingsTabs lead={i18n.t('brand.subtitle')} />

	{#if (!current || !draft) && brand.status === 'error'}
		<div class="win load-error" role="alert">
			<p class="muted">{i18n.t('brand.loadError')}</p>
			<button class="btn" type="button" onclick={load}>{i18n.t('common.retry')}</button>
		</div>
	{:else if !current || !draft}
		<div class="layout" aria-busy="true" aria-label={i18n.t('common.loading')}>
			<span class="sk" style="height:560px"></span>
			<span class="sk" style="height:380px"></span>
		</div>
	{:else}
		<div class="layout">
			<section class="win form" aria-labelledby="brand-form-title">
				<div class="win-bar">
					<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3 L20 6 V12 C20 16.5 16.5 20 12 21 C7.5 20 4 16.5 4 12 V6 Z" /></svg>
					<h2 class="bar-title" id="brand-form-title">{i18n.t('brand.formTitle')}</h2>
					<span class="sp"></span>
										<span class="save-state" aria-live="polite">
						{#if saveState === 'saving' || (dirty && saveState !== 'error')}
							<span class="muted">{i18n.t('brand.saving')}</span>
						{:else if saveState === 'saved'}
							<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12.5 L10 17.5 L19 7" /></svg>
							{i18n.t('brand.savedShort')}
												{:else if saveState === 'error'}
							<span class="save-err" role="alert">{saveError}</span>
							<button type="button" class="link-u retry" onclick={save}>{i18n.t('common.retry')}</button>
						{:else}
							<!-- At rest it still says how it works: nothing to press. -->
							<span class="muted">{i18n.t('brand.autosave')}</span>
						{/if}
					</span>
					<span class="on-count">{i18n.t('brand.onCount', { n: onCount })}</span>
				</div>

				<ul class="toggles rows">
					<li class="trow">
						<span class="ttext">
							{#if current.hasLogo}
								<b>{i18n.t('brand.row.logo', { pos: i18n.t(`brand.pos.${draft.position}`).toLowerCase() })}</b>
								<small>{i18n.t('brand.row.logoSub', { o: draft.opacityPct, s: draft.logoScalePct })}</small>
							{:else}
																<b>{i18n.t('brand.row.noLogo')}</b>
								<small>{i18n.t('brand.row.noLogoSub')}</small>
							{/if}
						</span>
						{#if current.hasLogo}
							<span class="tthumb" aria-hidden="true"><img src={webSrc(api.brandLogoUrl(current.logoVersion))} use:authedSrc={api.brandLogoUrl(current.logoVersion)} alt="" /></span>
						{/if}
											</li>
					<li class="tsub" id="logo">
						<LogoPicker brand={current} onchange={logoChanged} />
					</li>
					<li class="trow">
						<span class="ttext">
							<b id="t-company">{i18n.t('brand.row.company')}</b>
							<small>{i18n.t('brand.companyHint')}</small>
						</span>
												<button type="button" class="switch" role="switch" aria-checked={hasCompany} aria-labelledby="t-company" onclick={() => setToken('{company}', !hasCompany)}></button>
					</li>
					<li class="tsub">
						<label for="company" class="sr-only">{i18n.t('brand.company')}</label>
						<input id="company" type="text" maxlength="80" placeholder={i18n.t('brand.companyPlaceholder')} bind:value={draft.companyName} />
					</li>
					<li class="trow">
						<span class="ttext">
							<b id="t-recipient">{i18n.t('brand.row.recipient')}</b>
							<small>{hasRecipient ? i18n.t('brand.row.recipientSub') : i18n.t('brand.row.recipientOff')}</small>
						</span>
												{#if hasRecipient}
							<span class="always">
								<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="11" width="14" height="9" rx="1.5" /><path d="M8 11 V8 a4 4 0 0 1 8 0 v3" /></svg>
								{i18n.t('brand.row.always')}
							</span>
						{:else}
							<button type="button" class="switch" role="switch" aria-checked="false" aria-labelledby="t-recipient" onclick={() => setToken('{recipient}', true)}></button>
						{/if}
					</li>
					<li class="trow">
						<span class="ttext">
							<b id="t-date">{i18n.t('brand.row.date')}</b>
							<small class="mono">{today}</small>
						</span>
						<button type="button" class="switch" role="switch" aria-checked={hasDate} aria-labelledby="t-date" onclick={() => setToken('{date}', !hasDate)}></button>
					</li>
					<li class="trow">
						<span class="ttext">
							<b id="t-tiled">{i18n.t('brand.row.tiled')}</b>
							<small>{i18n.t('brand.row.tiledSub')}</small>
						</span>
						<button type="button" class="switch" role="switch" aria-checked={tiled} aria-labelledby="t-tiled" onclick={() => setTiled(!tiled)}></button>
					</li>
				</ul>

				<div class="win-body details">
					

					<h2 class="group-title">{i18n.t('brand.group.placement')}</h2>
					<div class="field">
						<span class="label" id="pos-label">{i18n.t('brand.position')}</span>
												<PositionPicker value={draft.position} onchange={(p) => draft && (draft.position = p)} labelledby="pos-label" showTiled={false} />
						{#if draft.position === 'tiled'}
							<span class="muted hint">{i18n.t('brand.tiledOnHint')}</span>
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

					<details class="advanced" bind:open={advancedOpen}>
						<summary>{i18n.t('brand.advanced')}</summary>
						<div class="field">
							<label for="template">{i18n.t('brand.text')}</label>
							<input id="template" class="code-mono" type="text" maxlength="120" bind:this={templateInput} bind:value={draft.textTemplate} />
							<div class="vars">
								<span class="muted">{i18n.t('brand.insert')}</span>
								<button type="button" onclick={() => insert('{company}')}>{i18n.t('brand.var.company')}</button>
								<button type="button" onclick={() => insert('{recipient}')}>{i18n.t('brand.var.recipient')}</button>
								<button type="button" onclick={() => insert('{date}')}>{i18n.t('brand.var.date')}</button>
							</div>
							<span class="muted hint">{i18n.t('brand.textHint')}</span>
						</div>
					</details>
				</div>
				
			</section>

			<section class="win preview" aria-labelledby="brand-preview-title">
				<div class="win-bar">
					<span class="win-dots" aria-hidden="true"><i></i><i></i><i></i></span>
					<h2 class="bar-title" id="brand-preview-title">{i18n.t('brand.preview')}</h2>
				</div>
				<div class="win-body preview-body">
				<BrandPreview
					logoUrl={current.hasLogo ? api.brandLogoUrl(current.logoVersion) : ''}
					position={draft.position}
					opacityPct={draft.opacityPct}
					logoScalePct={draft.logoScalePct}
					textFor={sampleFor}
				/>
				<span class="muted hint">{i18n.t('brand.previewHint')}</span>
				
				</div>
			</section>
		</div>
	{/if}
</div>

<style>
	.layout {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1.1fr);
		gap: 24px;
		align-items: start;
	}

	/* Phones/tablets: the preview stays in view (compact, pinned under the
	   header) while the sliders below it move, and Save is pinned to the
	   bottom while there is something to save. */
	@media (max-width: 860px) {
		.layout {
			grid-template-columns: minmax(0, 1fr);
		}
		.preview {
			order: -1;
			position: sticky;
			top: 68px;
			z-index: 5;
			max-height: 38vh;
		}
		.preview .win-bar {
			display: none;
		}
		.preview-body {
			padding: 10px;
		}
		.preview .hint {
			display: none;
		}
		
	}

	@media (min-width: 861px) {
		.preview {
			position: sticky;
			top: 96px;
		}
	}

	.on-count {
		color: var(--text-muted);
		font-variant-numeric: tabular-nums;
	}

	/* The mockup's toggle rows: title over its value, the switch on the right. */
	.toggles {
		padding: 0 20px;
		border-bottom: 1px solid var(--line);
	}

	.trow {
		display: flex;
		align-items: center;
		gap: 14px;
		min-height: 60px;
		padding: 8px 0;
	}

	.ttext {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.ttext b {
		font-weight: 500;
		font-size: 0.95rem;
	}

	.ttext small {
		font-size: 0.85rem;
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.tthumb {
		width: 44px;
		height: 30px;
		flex: none;
		display: grid;
		place-items: center;
		padding: 4px;
		border-radius: var(--radius-xs);
		background: var(--fill);
	}

	.tthumb img {
		max-width: 100%;
		max-height: 100%;
		display: block;
	}

	

		.details {
		padding-top: 8px;
	}

	/* A row's own control, right under it (the name field, the logo picker). */
	.toggles > li.tsub {
		border-top: none;
		padding: 0 0 14px;
	}

	.always {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 28px;
		padding: 0 10px;
		border-radius: 999px;
		background: var(--fill-2);
		color: var(--text);
		font-size: 0.85rem;
		font-weight: 500;
		white-space: nowrap;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		margin: -1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}

	.group-title {
		margin: 18px 0 14px;
		font-size: 0.95rem;
		font-weight: 500;
		color: var(--text);
	}

	.advanced {
		border-top: 1px solid var(--line);
		padding-top: 4px;
	}

	.advanced summary {
		display: flex;
		align-items: center;
		gap: 8px;
		min-height: 44px;
		font-size: 0.95rem;
		font-weight: 500;
		cursor: pointer;
		list-style: none;
	}

	.advanced summary::-webkit-details-marker {
		display: none;
	}

	.advanced summary::before {
		content: '';
		width: 7px;
		height: 7px;
		border-right: 1.6px solid var(--text-muted);
		border-bottom: 1.6px solid var(--text-muted);
		transform: rotate(-45deg);
		transition: transform 0.15s var(--ease);
	}

	.advanced[open] summary::before {
		transform: rotate(45deg);
	}

	.preview-body {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.label {
		display: block;
		font-size: 0.85rem;
		font-weight: 500;
		margin-bottom: 0.4rem;
		color: var(--text-muted);
	}

	.load-error {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 20px;
	}

	.load-error p {
		margin: 0;
	}

	.form .field {
		margin-bottom: 22px;
	}

	.hint {
		display: block;
		font-size: 0.82rem;
		line-height: 1.45;
		margin-top: 0.4rem;
	}

	.value {
		float: right;
		color: var(--text);
	}

	.sliders {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0 24px;
	}

	@media (max-width: 520px) {
		.sliders {
			grid-template-columns: 1fr;
		}
	}

	input[type='range'] {
		width: 100%;
				accent-color: var(--ink-btn);
	}

	.vars {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 6px;
		margin-top: 8px;
		font-size: 0.85rem;
	}

	.vars span {
		margin-right: 4px;
	}

	.vars button {
		appearance: none;
		position: relative;
		border: 1px solid var(--line-2);
		background: transparent;
		color: var(--text);
		border-radius: 999px;
		min-height: 32px;
		padding: 0 0.8rem;
		font: inherit;
		font-size: 0.85rem;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.15s var(--ease);
	}

	.vars button::after {
		content: '';
		position: absolute;
		inset: -6px -2px;
	}

	.vars button:hover {
		background: var(--fill);
	}

		.save-state {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 0.85rem;
		color: var(--success);
	}

	.save-err {
		color: var(--danger);
	}

	.retry {
		font: inherit;
	}

	/* A hint that is a way forward, not a warning: muted, underlined. */
	.hint a {
		color: inherit;
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.hint a:hover {
		color: var(--text);
	}
</style>
