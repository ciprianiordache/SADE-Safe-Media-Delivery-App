<script lang="ts">
	import { i18n } from '$lib/i18n.svelte';
	import type { JobStatus } from '$lib/types';

	let { status }: { status: JobStatus } = $props();
</script>

<span class="badge badge-{status}">{i18n.t(`status.${status}`)}</span>
