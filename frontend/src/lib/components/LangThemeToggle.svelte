<script lang="ts">
	import { theme } from '$lib/theme.svelte';
	import { i18n } from '$lib/i18n.svelte';

</script>

<div class="toggle-group">
	<div class="pill-group" role="group" aria-label={i18n.t('lang.label')}>
		<button type="button" aria-pressed={i18n.locale === 'ro'} onclick={() => i18n.set('ro')}>
			RO
		</button>
		<button type="button" aria-pressed={i18n.locale === 'en'} onclick={() => i18n.set('en')}>
			EN
		</button>
	</div>
	<div class="pill-group" role="group" aria-label={i18n.t('theme.label')}>
		<button type="button" aria-label={i18n.t('theme.system')} title={i18n.t('theme.system')} aria-pressed={theme.value === 'system'} onclick={() => theme.set('system')}>
			<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
				><rect x="3" y="4.5" width="18" height="12" rx="1.8" /><path d="M9 20h6M12 16.5V20" /></svg
			>
		</button>
		<button type="button" aria-label={i18n.t('theme.light')} title={i18n.t('theme.light')} aria-pressed={theme.value === 'light'} onclick={() => theme.set('light')}>
			<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
				><circle cx="12" cy="12" r="4.2" /><path
					d="M12 3 v2 M12 19 v2 M4.2 4.2 l1.4 1.4 M18.4 18.4 l1.4 1.4 M3 12 h2 M19 12 h2 M4.2 19.8 l1.4-1.4 M18.4 5.6 l1.4-1.4"
				/></svg
			>
		</button>
		<button type="button" aria-label={i18n.t('theme.dark')} title={i18n.t('theme.dark')} aria-pressed={theme.value === 'dark'} onclick={() => theme.set('dark')}>
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
