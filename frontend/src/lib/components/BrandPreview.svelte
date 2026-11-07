<script lang="ts">
	// Instant preview of the watermark on a sample picture. Mirrors the
	// server's layout (internals/ffmpeg/filters.go): logo width as a % of the
	// frame, text ~2.2% of the width, a 2% margin, and "tiled" as a 3x3 logo
	// grid with the text on the 2x2 points between.
	import type { WatermarkPosition } from '$lib/types';

	let {
		logoUrl = '',
		position,
		opacityPct,
		logoScalePct,
		text = '',
		showLogo = true,
		showText = true
	}: {
		logoUrl?: string;
		position: WatermarkPosition;
		opacityPct: number;
		logoScalePct: number;
		text?: string;
		showLogo?: boolean;
		showText?: boolean;
	} = $props();

	let logo = $derived(showLogo && !!logoUrl);
	let txt = $derived(showText && !!text);
	let textAlpha = $derived(Math.min(0.95, opacityPct / 100 + 0.3));
	const CELLS = [1, 3, 5];
	const MID = [1, 2];
</script>

<div class="frame" aria-hidden="true">
	<div class="scene">
		<span class="sun"></span>
		<span class="hill back"></span>
		<span class="hill front"></span>
	</div>

	{#if position === 'tiled'}
		{#if logo}
			{#each CELLS as row (row)}
				{#each CELLS as col (col)}
					<img
						class="logo tile"
						src={logoUrl}
						alt=""
						style="left:{(col / 6) * 100}%;top:{(row / 6) * 100}%;width:{logoScalePct}%;opacity:{opacityPct / 100}"
					/>
				{/each}
			{/each}
		{/if}
		{#if txt}
			{#each MID as row (row)}
				{#each MID as col (col)}
					<span class="text tile" style="left:{(col / 3) * 100}%;top:{(row / 3) * 100}%;--a:{textAlpha}">{text}</span>
				{/each}
			{/each}
		{/if}
	{:else if logo || txt}
		<!-- Logo and text share the spot; the text sits clear of the logo
		     (above it at the bottom, below it at the top and centre). -->
		<div class="stack pos-{position}" class:up={position.startsWith('bottom')} class:right={position.endsWith('right')}>
			{#if logo}
				<img class="logo" src={logoUrl} alt="" style="width:{logoScalePct}cqw;opacity:{opacityPct / 100}" />
			{/if}
			{#if txt}
				<span class="text" class:hang={logo && position === 'center'} style="--a:{textAlpha}">{text}</span>
			{/if}
		</div>
	{/if}
</div>

<style>
	.frame {
		position: relative;
		aspect-ratio: 16 / 9;
		width: 100%;
		border-radius: var(--radius-sm);
		overflow: hidden;
		container-type: inline-size;
		background: #2c4660;
	}

	.scene {
		position: absolute;
		inset: 0;
		background: linear-gradient(180deg, #5d86ad 0%, #e7b48c 62%, #d98f62 100%);
	}

	.sun {
		position: absolute;
		width: 14%;
		aspect-ratio: 1;
		left: 64%;
		top: 30%;
		border-radius: 999px;
		background: #ffe2b8;
		box-shadow: 0 0 40px 10px rgb(255 226 184 / 0.5);
	}

	.hill {
		position: absolute;
		left: -10%;
		right: -10%;
		border-radius: 50% 50% 0 0;
	}

	.hill.back {
		bottom: -18%;
		height: 62%;
		background: #6b5a6e;
		transform: translateX(-18%);
	}

	.hill.front {
		bottom: -30%;
		height: 60%;
		background: #3a3346;
		transform: translateX(22%);
	}

	.logo {
		height: auto;
		display: block;
	}

	.logo.tile {
		position: absolute;
	}

	.stack {
		position: absolute;
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 1cqw;
	}

	.stack.up {
		flex-direction: column-reverse;
	}

	.stack.right {
		align-items: flex-end;
	}

	.stack.pos-center {
		align-items: center;
	}

	.text {
		font-family: var(--font-sans);
		font-size: 2.2cqw;
		line-height: 1.2;
		white-space: nowrap;
		color: rgb(255 255 255 / var(--a));
		background: rgb(0 0 0 / calc(var(--a) * 0.4));
		padding: 0.35cqw 0.7cqw;
	}

	.tile {
		position: absolute;
		transform: translate(-50%, -50%);
	}

	/* Corners: a 2% margin, like the server. */
	.pos-top-left {
		left: 2%;
		top: 2%;
	}
	.pos-top-right {
		right: 2%;
		top: 2%;
	}
	.pos-bottom-left {
		left: 2%;
		bottom: 2%;
	}
	.pos-bottom-right {
		right: 2%;
		bottom: 2%;
	}
	.pos-center {
		left: 50%;
		top: 50%;
		transform: translate(-50%, -50%);
	}

	/* Centre with a logo: the logo is centred, the text hangs below it. */
	.text.hang {
		position: absolute;
		top: calc(100% + 1cqw);
		left: 50%;
		transform: translateX(-50%);
	}
</style>
