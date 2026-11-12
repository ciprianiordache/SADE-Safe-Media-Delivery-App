<script lang="ts">
	import { toasts } from '$lib/toast.svelte';
	import { i18n } from '$lib/i18n.svelte';
</script>

<div class="toasts" aria-live="polite">
	{#each toasts.items as t (t.id)}
		<div
			class="toast toast-{t.kind}"
			role={t.kind === 'error' ? 'alert' : 'status'}
			onmouseenter={() => toasts.pause(t.id)}
			onmouseleave={() => toasts.resume(t.id)}
			onfocusin={() => toasts.pause(t.id)}
			onfocusout={() => toasts.resume(t.id)}
		>
			<span class="dot"></span>
			<span class="msg">{t.message}</span>
			{#if t.href}
				<a href={t.href} onclick={() => toasts.dismiss(t.id)}>{t.action ?? i18n.t('common.open')}</a>
			{/if}
			<button type="button" aria-label={i18n.t('common.close')} onclick={() => toasts.dismiss(t.id)}>
				<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"
					><path d="M6 6 L18 18 M18 6 L6 18" /></svg
				>
			</button>
		</div>
	{/each}
</div>

<style>
	.toasts {
		position: fixed;
		bottom: 1.25rem;
		right: 1.25rem;
		left: auto;
		z-index: 100;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		max-width: min(380px, calc(100vw - 2rem));
	}

	@media (max-width: 520px) {
		.toasts {
			left: 1rem;
			right: 1rem;
			bottom: 1rem;
			max-width: none;
		}
	}

	.toast {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.35rem 0.75rem 0.35rem 0.9rem;
				background: var(--surface);
		border-radius: var(--radius);
		box-shadow: var(--shadow-card);
		font-size: 0.9rem;
		animation: slide-in 0.18s ease-out;
	}

	@keyframes slide-in {
		from {
			opacity: 0;
			transform: translateY(8px);
		}
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 999px;
		flex-shrink: 0;
		background: var(--success);
	}

	.toast-error .dot {
		background: var(--danger);
	}

	.toast-info .dot {
		background: var(--accent-bright);
	}

	.msg {
		flex: 1;
		line-height: 1.4;
	}

	a {
		font-weight: 500;
		white-space: nowrap;
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		padding: 0 0.25rem;
	}

	button {
		appearance: none;
		background: none;
		border: none;
		width: 44px;
		height: 44px;
		margin: -0.5rem -0.4rem -0.5rem 0;
		color: var(--text-muted);
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-sm);
	}

	button:hover {
		color: var(--text);
	}
</style>
