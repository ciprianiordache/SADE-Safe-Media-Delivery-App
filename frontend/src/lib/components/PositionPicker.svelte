<script lang="ts">
	// Where the watermark sits: four corners, centre, or tiled over the
	// whole picture. Each option is a tiny drawing of the result.
	import { i18n, type MessageKey } from '$lib/i18n.svelte';
	import type { WatermarkPosition } from '$lib/types';

	let { value, onchange }: { value: WatermarkPosition; onchange: (p: WatermarkPosition) => void } = $props();

	const OPTIONS: WatermarkPosition[] = ['top-left', 'top-right', 'center', 'bottom-left', 'bottom-right', 'tiled'];
	const DOTS: Record<WatermarkPosition, [number, number][]> = {
		'top-left': [[7, 6]],
		'top-right': [[25, 6]],
		center: [[16, 10]],
		'bottom-left': [[7, 14]],
		'bottom-right': [[25, 14]],
		tiled: [
			[6, 5], [16, 5], [26, 5],
			[6, 10], [16, 10], [26, 10],
			[6, 15], [16, 15], [26, 15]
		]
	};
</script>

<div class="positions" role="radiogroup" aria-label={i18n.t('brand.position')}>
	{#each OPTIONS as p (p)}
		<button
			type="button"
			role="radio"
			aria-checked={value === p}
			class:on={value === p}
			onclick={() => onchange(p)}
			title={i18n.t(`brand.pos.${p}` as MessageKey)}
		>
			<svg width="34" height="20" viewBox="0 0 32 20" aria-hidden="true">
				<rect x="0.5" y="0.5" width="31" height="19" rx="2.5" fill="none" stroke="currentColor" stroke-opacity="0.35" />
				{#each DOTS[p] as [x, y], i (i)}
					<rect x={x - (p === 'tiled' ? 2.5 : 4)} y={y - (p === 'tiled' ? 1.5 : 2.5)} width={p === 'tiled' ? 5 : 8} height={p === 'tiled' ? 3 : 5} rx="1" fill="currentColor" />
				{/each}
			</svg>
			<span>{i18n.t(`brand.pos.${p}` as MessageKey)}</span>
		</button>
	{/each}
</div>

<style>
	.positions {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.45rem;
	}

	button {
		appearance: none;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.3rem;
		padding: 0.55rem 0.3rem;
		border-radius: var(--radius-sm);
		border: 1.5px solid var(--border-strong);
		background: var(--surface);
		color: var(--text-muted);
		font: inherit;
		font-size: 0.72rem;
		line-height: 1.2;
		text-align: center;
		cursor: pointer;
	}

	button:hover {
		border-color: var(--accent);
	}

	button.on {
		border-color: var(--accent);
		background: var(--accent-tint);
		color: var(--accent-tint-text);
		font-weight: 600;
	}
</style>
