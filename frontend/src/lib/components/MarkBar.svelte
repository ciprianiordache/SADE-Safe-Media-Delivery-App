<script lang="ts">
	import logo from '$lib/assets/logo.png';
	import LangThemeToggle from './LangThemeToggle.svelte';

	let { size = 24 }: { size?: number } = $props();
</script>

<div class="mark-bar">
	<a class="mark" href="/">
		<img src={logo} alt="" width={size} height={size} />
		<span style="font-size: {size < 24 ? '0.75rem' : '0.85rem'}">SADE</span>
	</a>
	<LangThemeToggle />
</div>

<style>
	.mark-bar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.mark {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		color: var(--text);
	}

	.mark img {
		border-radius: 999px;
		display: block;
	}

	.mark span {
		font-weight: 600;
		letter-spacing: 0.14em;
	}
</style>
