<script lang="ts">
	import { authedSrc, webSrc } from '$lib/media';
	// Upload / replace / remove the account's logo (click or drag & drop).
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import type { Brand } from '$lib/types';

	let { brand, onchange }: { brand: Brand; onchange: (b: Brand) => void } = $props();

	let input = $state<HTMLInputElement | undefined>();
	let busy = $state(false);
	let error = $state('');
	let dragging = $state(false);
	let confirming = $state(false);

	async function upload(file: File | undefined) {
		if (!file) return;
		error = '';
		if (!/^image\/(png|jpeg)$/.test(file.type)) {
			error = i18n.t('brand.logoErrorType');
			return;
		}
		if (file.size > 3 * 1024 * 1024) {
			error = i18n.t('brand.logoErrorSize');
			return;
		}
		busy = true;
		try {
			onchange(await api.uploadLogo(file));
		} catch (err) {
			error =
				err instanceof ApiError && err.status === 413
					? i18n.t('brand.logoErrorSize')
					: err instanceof ApiError && err.status === 415
						? i18n.t('brand.logoErrorType')
						: err instanceof ApiError && err.status === 429
							? i18n.t('brand.logoErrorLimit')
							: i18n.t('common.error');
		} finally {
			busy = false;
			if (input) input.value = '';
		}
	}

	// Removing deletes the stored file for good, so it takes a second click.
	async function remove() {
		confirming = false;
		busy = true;
		try {
			onchange(await api.deleteLogo());
		} catch {
			error = i18n.t('common.error');
		} finally {
			busy = false;
		}
	}

	function drop(e: DragEvent) {
		e.preventDefault();
		dragging = false;
		upload(e.dataTransfer?.files?.[0]);
	}
</script>

<div class="logo-picker" aria-busy={busy}>
	{#if brand.hasLogo}
		<div class="current">
			<span class="thumb"><img src={webSrc(api.brandLogoUrl(brand.logoVersion))} use:authedSrc={api.brandLogoUrl(brand.logoVersion)} alt="" /></span>
			<div class="actions">
				<button type="button" class="btn btn-ghost small" onclick={() => input?.click()} disabled={busy}>
					{busy ? i18n.t('brand.logoUploading') : i18n.t('brand.logoChange')}
				</button>
				{#if confirming}
					<span class="confirm" role="group" aria-label={i18n.t('brand.logoRemoveConfirm')}>
						<span class="muted small-text">{i18n.t('brand.logoRemoveConfirm')}</span>
						<button type="button" class="text-btn danger" onclick={remove} disabled={busy}>{i18n.t('brand.logoRemoveYes')}</button>
						<button type="button" class="text-btn" onclick={() => (confirming = false)}>{i18n.t('brand.logoRemoveNo')}</button>
					</span>
				{:else}
					<button type="button" class="text-btn" onclick={() => (confirming = true)} disabled={busy}>{i18n.t('brand.logoRemove')}</button>
				{/if}
			</div>
		</div>
	{:else}
		<button
			type="button"
			class="dropzone"
			class:dragging
			onclick={() => input?.click()}
			ondragover={(e) => {
				e.preventDefault();
				dragging = true;
			}}
			ondragleave={() => (dragging = false)}
			ondrop={drop}
			disabled={busy}
		>
			<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="var(--text-hint)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"
				><rect x="3" y="4" width="18" height="16" rx="2" /><circle cx="9" cy="10" r="1.8" /><path d="M21 16 L15.5 10.5 L5 20" /></svg
			>
			<span class="muted">
				{#if busy}
					{i18n.t('brand.logoUploading')}
				{:else}
					{i18n.t('brand.logoDrop')} <span class="accent">{i18n.t('brand.logoBrowse')}</span>
				{/if}
			</span>
		</button>
	{/if}
	<span class="muted hint">{i18n.t('brand.logoHint')} {i18n.t('brand.logoSavesNow')}</span>
	{#if error}<span class="error-text hint" role="alert">{error}</span>{/if}
	<input bind:this={input} type="file" accept="image/png,image/jpeg" style="display:none" onchange={(e) => upload(e.currentTarget.files?.[0])} />
</div>

<style>
	.logo-picker {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.current {
		display: flex;
		align-items: center;
		gap: 0.9rem;
	}

	.thumb {
		width: 120px;
		height: 64px;
		flex-shrink: 0;
		border-radius: var(--radius-sm);
		box-shadow: inset 0 0 0 1px var(--line-2);
		/* A checkerboard shows off transparency. */
		background:
			repeating-conic-gradient(var(--border) 0 25%, var(--surface) 0 50%) 0 0 / 12px 12px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 6px;
	}

	.thumb img {
		max-width: 100%;
		max-height: 100%;
		display: block;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}

	.small {
		padding: 0.45rem 0.8rem;
		font-size: 0.82rem;
	}

	.text-btn {
		appearance: none;
		background: none;
		border: none;
		font: inherit;
		font-size: 0.82rem;
		color: var(--text-muted);
		cursor: pointer;
		padding: 0.3rem;
		min-height: 44px;
	}

	.text-btn:hover,
	.text-btn.danger {
		color: var(--danger);
	}

	.text-btn.danger {
		font-weight: 500;
	}

	.confirm {
		display: inline-flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.25rem;
	}

	.small-text {
		font-size: 0.8rem;
	}

	.dropzone {
		width: 100%;
		font-family: var(--font-sans);
	}

	.dropzone.dragging,
	.dropzone:hover {
		border-color: var(--accent);
		background: var(--accent-tint);
	}

	.accent {
		color: var(--accent-ink);
		font-weight: 500;
	}

	.hint {
		font-size: 0.75rem;
		line-height: 1.45;
	}
</style>
