// Small app-wide notifications ("Uploaded", "Ready"). One store, rendered
// once by Toasts.svelte in the root layout.
export type ToastKind = 'success' | 'error' | 'info';

export interface Toast {
	id: number;
	kind: ToastKind;
	message: string;
	href?: string;
	action?: string;
}

class ToastStore {
	items = $state<Toast[]>([]);
	private nextId = 1;
	private timers = new Map<number, { handle: ReturnType<typeof setTimeout>; ms: number }>();

	show(message: string, opts: { kind?: ToastKind; href?: string; action?: string; ms?: number } = {}) {
		const id = this.nextId++;
		this.items = [...this.items, { id, kind: opts.kind ?? 'success', message, href: opts.href, action: opts.action }];
		// A toast with a link needs time to be read and reached (WCAG 2.2.1).
		this.arm(id, opts.ms ?? (opts.href ? 12000 : 5000));
	}

	private arm(id: number, ms: number) {
		this.timers.set(id, { handle: setTimeout(() => this.dismiss(id), ms), ms });
	}

	/** Stops the countdown while the pointer or keyboard focus is on the toast. */
	pause(id: number) {
		const t = this.timers.get(id);
		if (t) clearTimeout(t.handle);
	}

	resume(id: number) {
		const t = this.timers.get(id);
		if (t) this.arm(id, Math.max(t.ms, 4000));
	}

	dismiss(id: number) {
		const t = this.timers.get(id);
		if (t) clearTimeout(t.handle);
		this.timers.delete(id);
		this.items = this.items.filter((t) => t.id !== id);
	}
}

export const toasts = new ToastStore();
