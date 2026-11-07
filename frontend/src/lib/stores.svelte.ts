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
//
// status: 'idle' (never asked) -> 'loading' -> 'ready' | 'error'. A brand
// that is already 'ready' stays 'ready' while it is fetched again, so pages
// never flash back to a "don't know yet" state. Concurrent load() calls
// share one request; a request with no answer after BRAND_TIMEOUT_MS
// counts as an error (pages show "Reîncearcă" instead of waiting forever).
export type BrandStatus = 'idle' | 'loading' | 'ready' | 'error';
const BRAND_TIMEOUT_MS = 8000;

class BrandStore {
	value = $state<Brand | null>(null);
	status = $state<BrandStatus>('idle');
	private inflight: Promise<Brand | null> | null = null;

	load(): Promise<Brand | null> {
		if (this.inflight) return this.inflight;
		if (this.status !== 'ready') this.status = 'loading';
		const timeout = new Promise<never>((_, reject) =>
			setTimeout(() => reject(new Error('brand: timeout')), BRAND_TIMEOUT_MS)
		);
		this.inflight = Promise.race([api.brand(), timeout])
			.then((b) => {
				this.value = b;
				this.status = 'ready';
				return b;
			})
			.catch(() => {
				// A refresh that fails keeps the brand we already have.
				if (this.status !== 'ready') this.status = 'error';
				return this.status === 'ready' ? this.value : null;
			})
			.finally(() => {
				this.inflight = null;
			});
		return this.inflight;
	}

	set(b: Brand) {
		this.value = b;
		this.status = 'ready';
	}
}

export const brand = new BrandStore();
