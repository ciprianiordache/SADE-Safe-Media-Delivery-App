// Svelte 5 runes need a .svelte.ts file - plain .ts is never compiled by the
// Svelte preprocessor, so $state would be a runtime no-op there.
import { api } from './api';
import type { Brand, User } from './types';

class AuthStore {
	user = $state<User | null>(null);
	loading = $state(true);
	loaded = false;

	// Idempotent: safe to call from every /app route's guard without
	// refetching on each navigation.
	async ensureLoaded() {
		if (this.loaded) return;
		await this.refresh();
	}

	async refresh() {
		this.loading = true;
		try {
			this.user = await api.me();
		} finally {
			this.loading = false;
			this.loaded = true;
		}
	}

	async logout() {
		await api.logout();
		this.user = null;
	}

	async logoutEverywhere() {
		await api.logoutAll();
		this.user = null;
	}
}

export const auth = new AuthStore();

// The signed-in account's brand: read by the /app layout (first-run setup),
// the upload form (is there a logo?) and the settings page.
class BrandStore {
	value = $state<Brand | null>(null);

	async load(): Promise<Brand | null> {
		try {
			this.value = await api.brand();
		} catch {
			this.value = null;
		}
		return this.value;
	}

	set(b: Brand) {
		this.value = b;
	}
}

export const brand = new BrandStore();
