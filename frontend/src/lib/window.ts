// The installed app's own window controls (the window has no system frame;
// see src-tauri/tauri.conf.json "decorations": false). Loaded lazily so the
// web build never pulls in the Tauri API.
import { IS_APP } from './platform';

async function current() {
	const { getCurrentWindow } = await import('@tauri-apps/api/window');
	return getCurrentWindow();
}

export async function closeWindow() {
	if (IS_APP) (await current()).close();
}

export async function minimizeWindow() {
	if (IS_APP) (await current()).minimize();
}

export async function toggleMaximizeWindow() {
	if (IS_APP) (await current()).toggleMaximize();
}
