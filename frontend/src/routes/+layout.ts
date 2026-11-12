import { redirect } from '@sveltejs/kit';
import { IS_APP } from '$lib/platform';
import { lastPath } from '$lib/apptitle.svelte';

// adapter-static builds a single fallback shell (index.html); every route
// renders client-side, matching how internals/app/static.go serves it.
export const ssr = false;

// The installed app has no marketing page: "/" opens the app - or, after a
// reload (which Tauri serves from the root), the page that was open.
export function load({ url }) {
	if (IS_APP && url.pathname === '/') redirect(307, lastPath() || '/app');
}
