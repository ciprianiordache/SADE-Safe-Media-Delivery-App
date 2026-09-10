<script lang="ts">
	import type { JobStatus } from '$lib/types';

	let { status }: { status: JobStatus } = $props();
</script>

<span class="status-dot status-dot-{status}"></span>

<style>
	.status-dot {
		width: 8px;
		height: 8px;
		border-radius: 999px;
		display: inline-block;
	}

	.status-dot-pending {
		background: var(--pending);
	}

	.status-dot-processing {
		background: var(--warning);
	}

	.status-dot-done {
		background: var(--success);
	}

	.status-dot-failed {
		background: var(--danger);
	}
</style>
