export type Theme = 'light' | 'dark' | 'system';

const STORAGE_KEY = 'sade:theme';

function readStored(): Theme {
	try {
		const v = localStorage.getItem(STORAGE_KEY);
		if (v === 'light' || v === 'dark' || v === 'system') return v;
	} catch {
		// localStorage unavailable (private mode, SSR) - fall through
	}
	return 'system';
}

class ThemeStore {
	value = $state<Theme>('system');

	// Reads localStorage and paints the DOM; called once from the root
	// layout's onMount (browser-only, avoids a hydration mismatch).
	init() {
		this.value = readStored();
		this.apply();
	}

	set(theme: Theme) {
		this.value = theme;
		try {
			localStorage.setItem(STORAGE_KEY, theme);
		} catch {
			// best-effort persistence only
		}
		this.apply();
	}

	private apply() {
		if (typeof document === 'undefined') return;
		const root = document.documentElement;
		if (this.value === 'system') root.removeAttribute('data-theme');
		else root.setAttribute('data-theme', this.value);
	}
}

export const theme = new ThemeStore();
