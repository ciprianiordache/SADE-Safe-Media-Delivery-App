<script module lang="ts">
	let instances = 0;
</script>

<script lang="ts">
	// Where the watermark sits, picked on a small frame shaped like the
	// picture: each spot is clicked where it will appear. "Whole picture"
	// (tiled) is a separate choice below the frame. Real radio buttons, one
	// group: Tab enters once, arrow keys move between the six choices.
	import { i18n, type MessageKey } from '$lib/i18n.svelte';
	import type { WatermarkPosition } from '$lib/types';

	let {
		value,
		onchange,
				labelledby,
		showTiled = true
	}: { value: WatermarkPosition; onchange: (p: WatermarkPosition) => void; labelledby?: string; showTiled?: boolean } = $props();

	const name = `position-${++instances}`;
	const SPOTS: WatermarkPosition[] = ['top-left', 'top-right', 'center', 'bottom-left', 'bottom-right'];
	const label = (p: WatermarkPosition) => i18n.t(`brand.pos.${p}` as MessageKey);
</script>

<div class="picker" role="radiogroup" aria-labelledby={labelledby}>
	<div class="frame">
		{#each SPOTS as p (p)}
			<label class="spot {p}" class:on={value === p} title={label(p)}>
				<input type="radio" {name} value={p} checked={value === p} onchange={() => onchange(p)} />
				<span class="mark" aria-hidden="true"></span>
				<span class="sr-only">{label(p)}</span>
			</label>
		{/each}
	</div>
		{#if showTiled}
	<label class="tiled" class:on={value === 'tiled'}>
		<input type="radio" {name} value="tiled" checked={value === 'tiled'} onchange={() => onchange('tiled')} />
		<svg width="22" height="14" viewBox="0 0 22 14" aria-hidden="true">
			{#each [2, 9, 16] as x (x)}
				{#each [2, 6, 10] as y (y)}
					<rect {x} {y} width="4" height="2" rx="0.5" fill="currentColor" />
				{/each}
			{/each}
		</svg>
				<span>{label('tiled')}</span>
	</label>
	{/if}
	<span class="chosen muted" aria-hidden="true">{label(value)}</span>
</div>

<style>
	.picker {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.frame {
		position: relative;
		width: 100%;
		max-width: 280px;
		aspect-ratio: 16 / 10;
				border-radius: var(--radius);
		background: var(--fill);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.spot {
		position: absolute;
		width: 44px;
		height: 44px;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		border-radius: var(--radius-sm);
	}

	.spot.top-left {
		top: 0;
		left: 0;
	}
	.spot.top-right {
		top: 0;
		right: 0;
	}
	.spot.center {
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
	}
	.spot.bottom-left {
		bottom: 0;
		left: 0;
	}
	.spot.bottom-right {
		bottom: 0;
		right: 0;
	}

	.mark {
		width: 24px;
		height: 14px;
				border-radius: 4px;
		background: var(--surface);
		box-shadow: var(--shadow-sm);
		transition: background 0.15s var(--ease), transform 0.15s var(--ease);
	}

	.spot:hover .mark {
		transform: scale(1.08);
	}

	.spot.on .mark {
		background: var(--accent-bright);
		box-shadow: none;
	}

	/* These are <label>s for their radio inputs: undo the global form-label look. */
	.spot,
	.tiled {
		margin: 0;
		text-transform: none;
		letter-spacing: normal;
		font-weight: 400;
	}

	.tiled {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		min-height: 44px;
		max-width: 280px;
		padding: 0 0.75rem;
				border-radius: 999px;
		border: 1px solid var(--line-2);
		background: transparent;
		color: var(--text);
		font-size: 0.88rem;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.15s var(--ease);
	}

	.tiled:hover {
		background: var(--fill);
	}

	.tiled.on {
		border-color: transparent;
		background: var(--ink-btn);
		color: var(--ink-btn-text);
	}

	input {
		position: absolute;
		opacity: 0;
		width: 1px;
		height: 1px;
		margin: 0;
		pointer-events: none;
	}

	.spot:has(input:focus-visible),
	.tiled:has(input:focus-visible) {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

		.chosen {
		font-size: 0.85rem;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}
</style>
