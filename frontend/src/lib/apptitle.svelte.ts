// What the installed app's title bar names after "‹ Livrări" (a page sets it,
// e.g. the file page with the file's name; empty = the default label).
class AppTitle {
	value = $state('');
}

export const appTitle = new AppTitle();

// The app's last route, so a reload (F5) comes back to it: Tauri serves only
// real files and sends any other path to the root.
const LAST_KEY = 'sade:lastPath';

export function rememberPath(path: string) {
	try {
		if (path.startsWith('/app')) sessionStorage.setItem(LAST_KEY, path);
	} catch {
		// no storage
	}
}

export function lastPath(): string {
	try {
		return sessionStorage.getItem(LAST_KEY) || '';
	} catch {
		return '';
	}
}
