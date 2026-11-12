import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// Two builds of the same SvelteKit app:
// - the web build (default): served by the Go binary from frontend/build,
//   next to the static marketing page (frontend/static/index.html at "/");
// - the app build (`vite build --mode app`, .env.app sets VITE_SADE_APP=1):
//   the installed app's UI, packaged by Tauri (src-tauri), with no marketing
//   page - "/" goes straight to the app (src/routes/+layout.ts).
export default defineConfig(({ mode }) => {
	const app = mode === 'app';
	return {
		plugins: [
			sveltekit({
				compilerOptions: {
					// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
					runes: ({ filename }) =>
						filename.split(/[/\\]/).includes('node_modules') ? undefined : true
				},

				// Static SPA build: the Go binary serves frontend/build directly
				// (internals/app/static.go), falling back to 200.html for client-side
				// routes. index.html is the static marketing page from
				// frontend/static, served at "/". See src/routes/+layout.ts (ssr = false).
				// The app build is its own shell (index.html) over static-app/.
				files: { assets: app ? 'static-app' : 'static' },
				adapter: adapter({
					pages: app ? 'build-app' : 'build',
					assets: app ? 'build-app' : 'build',
					fallback: app ? 'index.html' : '200.html',
					precompress: false,
					strict: true
				})
			})
		],
		// Tauri's dev server expectations: a fixed port, fail if taken.
		server: app ? { port: 5174, strictPort: true } : undefined,
		clearScreen: false
	};
});
