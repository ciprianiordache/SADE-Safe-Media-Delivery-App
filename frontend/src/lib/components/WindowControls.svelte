<script lang="ts">
	// The window's own controls, drawn the way the system the app runs on
	// draws them (the window has no system frame):
	// - macOS: three lights on the left, grey until the pointer reaches them;
	// - Windows: minimise / maximise / close caption buttons on the right,
	//   full bar height, close turning red, as Windows 11 draws them;
	// - Linux: round GNOME-style buttons on the right.
	// Every bar places one instance at each end (side="left" / "right"); only
	// the one on this system's side renders. On the web, the left one is the
	// marketing picture's decorative dots.
	import { IS_APP, desktopOS } from '$lib/platform';
	import { i18n } from '$lib/i18n.svelte';
	import { closeWindow, minimizeWindow, toggleMaximizeWindow, watchMaximized } from '$lib/window';
	import { onMount } from 'svelte';

	let { side = 'left' }: { side?: 'left' | 'right' } = $props();

	const os = desktopOS();
	const mine = IS_APP ? (os === 'mac' ? 'left' : 'right') : 'left';

	// Maximised, the button restores - and says so, as Windows' own does.
	let maximized = $state(false);
	let maxLabel = $derived(i18n.t(maximized ? 'window.restore' : 'window.maximize'));
	onMount(() => {
		if (!IS_APP || side !== mine) return;
		let stop: (() => void) | undefined;
		let gone = false;
		watchMaximized((m) => (maximized = m)).then((u) => (gone ? u() : (stop = u)));
		return () => {
			gone = true;
			stop?.();
		};
	});
</script>

{#if side === mine}
	{#if !IS_APP}
		<span class="win-dots" aria-hidden="true"><i></i><i></i><i></i></span>
	{:else if os === 'mac'}
		<div class="mac" role="group" aria-label={i18n.t('window.controls')}>
			<button type="button" tabindex="-1" class="light close" aria-label={i18n.t('window.close')} title={i18n.t('window.close')} onclick={closeWindow}>
				<svg viewBox="0 0 8 8" aria-hidden="true"><path d="M2 2 6 6M6 2 2 6" /></svg>
			</button>
			<button type="button" tabindex="-1" class="light min" aria-label={i18n.t('window.minimize')} title={i18n.t('window.minimize')} onclick={minimizeWindow}>
				<svg viewBox="0 0 8 8" aria-hidden="true"><path d="M1.5 4h5" /></svg>
			</button>
			<button type="button" tabindex="-1" class="light max" aria-label={maxLabel} title={maxLabel} onclick={toggleMaximizeWindow}>
				<svg viewBox="0 0 8 8" aria-hidden="true"><path d="M2 6 6 2M3.2 2H6v2.8M4.8 6H2V3.2" /></svg>
			</button>
		</div>
	{:else if os === 'windows'}
		<div class="caps" role="group" aria-label={i18n.t('window.controls')}>
			<button type="button" tabindex="-1" class="cap" aria-label={i18n.t('window.minimize')} title={i18n.t('window.minimize')} onclick={minimizeWindow}>
				<svg viewBox="0 0 10 10" aria-hidden="true"><path d="M0 5.5h10" /></svg>
			</button>
			<button type="button" tabindex="-1" class="cap" aria-label={maxLabel} title={maxLabel} onclick={toggleMaximizeWindow}>
				{#if maximized}
					<svg viewBox="0 0 10 10" aria-hidden="true"><rect x="0.5" y="2.5" width="7" height="7" rx="1" /><path d="M2.5 2.5V1.5a1 1 0 0 1 1-1h5a1 1 0 0 1 1 1v5a1 1 0 0 1-1 1h-1" /></svg>
				{:else}
					<svg viewBox="0 0 10 10" aria-hidden="true"><rect x="0.5" y="0.5" width="9" height="9" rx="1" /></svg>
				{/if}
			</button>
			<button type="button" tabindex="-1" class="cap close" aria-label={i18n.t('window.close')} title={i18n.t('window.close')} onclick={closeWindow}>
				<svg viewBox="0 0 10 10" aria-hidden="true"><path d="M0.5 0.5 9.5 9.5M9.5 0.5 0.5 9.5" /></svg>
			</button>
		</div>
	{:else}
		<div class="gnome" role="group" aria-label={i18n.t('window.controls')}>
			<button type="button" tabindex="-1" class="round" aria-label={i18n.t('window.minimize')} title={i18n.t('window.minimize')} onclick={minimizeWindow}>
				<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M4.5 10.5h7" /></svg>
			</button>
			<button type="button" tabindex="-1" class="round" aria-label={maxLabel} title={maxLabel} onclick={toggleMaximizeWindow}>
				<svg viewBox="0 0 16 16" aria-hidden="true"><rect x="4.5" y="4.5" width="7" height="7" rx="1" /></svg>
			</button>
			<button type="button" tabindex="-1" class="round" aria-label={i18n.t('window.close')} title={i18n.t('window.close')} onclick={closeWindow}>
				<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M5 5 11 11M11 5 5 11" /></svg>
			</button>
		</div>
	{/if}
{/if}

<style>
	button {
		appearance: none;
		border: none;
		padding: 0;
		cursor: default;
	}

	svg {
		fill: none;
		stroke: currentColor;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	button:focus-visible {
		outline: 2px solid var(--accent-bright);
		outline-offset: -2px;
	}

	/* ---- macOS: the lights ---------------------------------------------- */
	.mac {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		flex: none;
		margin-right: 4px;
	}

	.light {
		position: relative;
		width: 12px;
		height: 12px;
		border-radius: 50%;
		background: var(--line-2);
		display: grid;
		place-items: center;
		transition: background 0.12s var(--ease);
	}

	.light::after {
		content: '';
		position: absolute;
		inset: -6px -4px;
	}

	.light svg {
		width: 8px;
		height: 8px;
		stroke: rgb(0 0 0 / 0.55);
		stroke-width: 1.2;
		opacity: 0;
	}

	.mac:hover .close,
	.mac:focus-within .close {
		background: #ff5f57;
	}
	.mac:hover .min,
	.mac:focus-within .min {
		background: #febc2e;
	}
	.mac:hover .max,
	.mac:focus-within .max {
		background: #28c840;
	}
	.mac:hover svg,
	.mac:focus-within svg {
		opacity: 1;
	}

	.light:focus-visible {
		outline-offset: 2px;
		border-radius: 50%;
	}

	/* ---- Windows: caption buttons, the bar's full height, flush right ---- */
	.caps {
		display: flex;
		align-self: stretch;
		flex: none;
		margin-right: calc(-1 * var(--bar-pad-right, 0px));
	}

	.cap {
		width: 46px;
		display: grid;
		place-items: center;
		background: transparent;
		color: var(--text);
		transition: background 0.08s linear;
	}

	.cap svg {
		width: 10px;
		height: 10px;
		stroke-width: 1;
		stroke-linecap: butt;
	}

	.cap:hover {
		background: color-mix(in oklab, var(--text) 9%, transparent);
	}

	.cap:active {
		background: color-mix(in oklab, var(--text) 6%, transparent);
	}

	.cap.close:hover {
		background: #c42b1c;
		color: #fff;
	}

	.cap.close:active {
		background: #c84031;
		color: #fff;
	}

	/* ---- Linux (GNOME): round buttons on the right ----------------------- */
	.gnome {
		display: inline-flex;
		align-items: center;
		gap: 10px;
		flex: none;
		margin-left: 4px;
	}

	.round {
		position: relative;
		width: 24px;
		height: 24px;
		border-radius: 50%;
		background: color-mix(in oklab, var(--text) 9%, transparent);
		color: var(--text);
		display: grid;
		place-items: center;
		transition: background 0.12s var(--ease);
	}

	.round::after {
		content: '';
		position: absolute;
		inset: -6px -5px;
	}

	.round:hover {
		background: color-mix(in oklab, var(--text) 16%, transparent);
	}

	.round svg {
		width: 16px;
		height: 16px;
		stroke-width: 1.4;
	}
</style>
