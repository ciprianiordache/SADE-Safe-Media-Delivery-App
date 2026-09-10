<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { theme } from '$lib/theme.svelte';
	import { i18n } from '$lib/i18n.svelte';
	import { onMount } from 'svelte';

	let { children } = $props();

	onMount(() => {
		theme.init();
		i18n.init();
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>{i18n.t('app.name')}</title>
</svelte:head>

{@render children()}
