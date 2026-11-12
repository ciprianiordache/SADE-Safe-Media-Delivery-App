<script lang="ts">
	// The "⋯" button of a desktop toolbar: the rare and the risky actions
	// (switch the links off, delete) live here, out of the way of the
	// everyday ones. Arrow keys move, Escape closes and returns focus.
	import { i18n } from '$lib/i18n.svelte';
	import { tick } from 'svelte';

		type Item = { label: string; onclick: () => void; danger?: boolean; disabled?: boolean; title?: string; sep?: boolean };
	let { items, label }: { items: Item[]; label?: string } = $props();

	let open = $state(false);
	let btn = $state<HTMLButtonElement | undefined>();
	let menu = $state<HTMLDivElement | undefined>();

	async function toggle() {
		open = !open;
		if (open) {
			await tick();
			menu?.querySelector<HTMLElement>('button:not(:disabled)')?.focus();
		}
	}

	function close(refocus = false) {
		open = false;
		if (refocus) btn?.focus();
	}

	function pick(it: Item) {
		close(true);
		it.onclick();
	}

	function onKey(e: KeyboardEvent) {
		const list = [...(menu?.querySelectorAll<HTMLElement>('button:not(:disabled)') ?? [])];
		const i = list.indexOf(document.activeElement as HTMLElement);
		if (e.key === 'Escape') {
			e.stopPropagation();
			close(true);
		} else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			e.preventDefault();
			list[(i + (e.key === 'ArrowDown' ? 1 : list.length - 1)) % list.length]?.focus();
		} else if (e.key === 'Tab') {
			close();
		}
	}
</script>

<svelte:window onpointerdown={(e) => open && !menu?.contains(e.target as Node) && !btn?.contains(e.target as Node) && close()} />

<div class="more">
	<button
		type="button"
		class="more-btn"
		bind:this={btn}
		aria-haspopup="menu"
		aria-expanded={open}
		aria-label={label ?? i18n.t('common.more')}
		title={label ?? i18n.t('common.more')}
		onclick={toggle}
	>
		<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" /></svg>
	</button>
	{#if open}
		<div class="more-menu" role="menu" tabindex="-1" bind:this={menu} onkeydown={onKey}>
						{#each items as it (it.label)}
				{#if it.sep}<hr />{/if}
				<button type="button" role="menuitem" class:danger={it.danger} disabled={it.disabled} title={it.title} onclick={() => pick(it)}>
					{it.label}
				</button>
			{/each}
		</div>
	{/if}
</div>

<style>
	.more {
		position: relative;
		flex: none;
	}

	.more-btn {
		appearance: none;
		position: relative;
		width: 36px;
		height: 36px;
		display: grid;
		place-items: center;
		border: 1px solid var(--line-2);
		border-radius: 999px;
		background: transparent;
		color: var(--text);
		cursor: pointer;
		transition: background 0.15s var(--ease), border-color 0.15s var(--ease);
	}

	/* Drawn 36px, tapped 48px (-4px measured 42 once neighbours took theirs). */
	.more-btn::after {
		content: '';
		position: absolute;
		inset: -6px;
	}

	.more-btn:hover,
	.more-btn[aria-expanded='true'] {
		background: var(--fill);
	}

	.more-menu {
		position: absolute;
		right: 0;
		top: calc(100% + 6px);
		z-index: 40;
		min-width: 220px;
		padding: 6px;
		display: flex;
		flex-direction: column;
		border: 1px solid var(--line);
		border-radius: var(--radius);
		background: var(--surface);
		box-shadow: 0 10px 30px -8px rgb(0 0 0 / 0.22), 0 2px 6px rgb(0 0 0 / 0.08);
		animation: more-in 0.12s var(--ease);
	}

	@keyframes more-in {
		from {
			opacity: 0;
			transform: translateY(-3px);
		}
	}

	.more-menu button {
		appearance: none;
		min-height: 34px;
		padding: 0 10px;
		border: none;
		border-radius: var(--radius-xs);
		background: none;
		font: inherit;
		font-size: 0.9rem;
		color: var(--text);
		text-align: left;
		white-space: nowrap;
		cursor: pointer;
	}

	.more-menu button:hover:not(:disabled),
	.more-menu button:focus-visible {
		background: var(--fill);
		outline: none;
	}

		.more-menu button.danger {
		color: var(--danger);
	}

	.more-menu hr {
		margin: 5px 4px;
		border: none;
		border-top: 1px solid var(--line);
	}

	.more-menu button:disabled {
		color: var(--text-muted);
		cursor: not-allowed;
	}
</style>
