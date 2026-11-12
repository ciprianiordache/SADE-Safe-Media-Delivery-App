<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { theme } from '$lib/theme.svelte';
	import { i18n } from '$lib/i18n.svelte';
	import { onMount } from 'svelte';
	import { afterNavigate } from '$app/navigation';
	import Toasts from '$lib/components/Toasts.svelte';
	import { IS_APP } from '$lib/platform';
	import { rememberPath } from '$lib/apptitle.svelte';

	let { children } = $props();

	onMount(() => {
		theme.init();
		i18n.init();
		if (!IS_APP) return;
		// The installed app behaves like a program, not a page: its own
		// styling hooks (html.is-app), and no browser context menu (Back,
		// Reload, Inspect) - text fields keep theirs for copy/paste.
		document.documentElement.classList.add('is-app');
		const onContext = (e: MouseEvent) => {
			const t = e.target as HTMLElement | null;
			if (!t?.closest('input, textarea, [contenteditable="true"]')) e.preventDefault();
		};
		document.addEventListener('contextmenu', onContext);
		return () => document.removeEventListener('contextmenu', onContext);
	});

	// A reload of the app comes back to this page (see routes/+layout.ts).
	afterNavigate(({ to }) => {
		if (IS_APP && to?.url) rememberPath(to.url.pathname + to.url.search);
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>{i18n.t('app.name')}</title>
</svelte:head>

{@render children()}

<Toasts />
