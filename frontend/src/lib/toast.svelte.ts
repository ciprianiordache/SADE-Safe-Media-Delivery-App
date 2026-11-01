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

	show(message: string, opts: { kind?: ToastKind; href?: string; action?: string; ms?: number } = {}) {
		const id = this.nextId++;
		this.items = [...this.items, { id, kind: opts.kind ?? 'success', message, href: opts.href, action: opts.action }];
		setTimeout(() => this.dismiss(id), opts.ms ?? 5000);
	}

	dismiss(id: number) {
		this.items = this.items.filter((t) => t.id !== id);
	}
}

export const toasts = new ToastStore();
