<script lang="ts">
	import { theme } from '$lib/theme.svelte';
	import { i18n } from '$lib/i18n.svelte';

	let isDark = $derived(
		theme.value === 'dark' ||
			(theme.value === 'system' &&
				typeof matchMedia !== 'undefined' &&
				matchMedia('(prefers-color-scheme: dark)').matches)
	);
</script>

<div class="toggle-group">
	<div class="pill-group">
		<button type="button" aria-pressed={i18n.locale === 'ro'} onclick={() => i18n.set('ro')}>
			RO
		</button>
		<button type="button" aria-pressed={i18n.locale === 'en'} onclick={() => i18n.set('en')}>
			EN
		</button>
	</div>
	<div class="pill-group">
		<button type="button" aria-label="Light theme" aria-pressed={!isDark} onclick={() => theme.set('light')}>
			<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
				><circle cx="12" cy="12" r="4.2" /><path
					d="M12 3 v2 M12 19 v2 M4.2 4.2 l1.4 1.4 M18.4 18.4 l1.4 1.4 M3 12 h2 M19 12 h2 M4.2 19.8 l1.4-1.4 M18.4 5.6 l1.4-1.4"
				/></svg
			>
		</button>
		<button type="button" aria-label="Dark theme" aria-pressed={isDark} onclick={() => theme.set('dark')}>
			<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
				><path d="M20 14.5 A8 8 0 1 1 9.5 4 A6.2 6.2 0 0 0 20 14.5 Z" /></svg
			>
		</button>
	</div>
</div>

<style>
	.toggle-group {
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}

	.pill-group button svg {
		display: block;
	}
</style>
