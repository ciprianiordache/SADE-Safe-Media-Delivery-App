<script lang="ts">
	import type { MediaType } from '$lib/types';

	let { type, size = 16 }: { type: MediaType; size?: number } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
	{#if type === 'video'}
		<rect x="3" y="6" width="13" height="12" rx="2" /><path d="M16 10.5 L21 7.5 V16.5 L16 13.5" />
	{:else if type === 'audio'}
		<path d="M9 18 V6 L20 4 V16" /><circle cx="6.5" cy="18" r="2.5" /><circle cx="17.5" cy="16" r="2.5" />
	{:else}
		<rect x="3" y="4" width="18" height="16" rx="2" /><circle cx="9" cy="10" r="1.8" /><path d="M21 16 L15.5 10.5 L5 20" />
	{/if}
</svg>
