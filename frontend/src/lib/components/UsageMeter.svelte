<script lang="ts">
	// The operator's consumption against their quotas (GET /api/usage).
	// Admins are unlimited and see nothing here.
	import { i18n } from '$lib/i18n.svelte';
	import type { Usage } from '$lib/types';

	let { usage }: { usage: Usage | null } = $props();

	function pct(used: number, limit: number): number {
		return limit > 0 ? Math.min(100, Math.round((used / limit) * 100)) : 0;
	}

	function size(bytes: number): string {
		const g = bytes / 1024 ** 3;
		if (g >= 1) return `${g.toFixed(g >= 10 ? 0 : 1)} GB`;
				if (bytes > 0 && bytes < 1024 ** 2) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
		return `${Math.round(bytes / 1024 ** 2)} MB`;
	}
</script>

{#if usage && !usage.unlimited}
	<div class="usage" aria-label={i18n.t('usage.label')}>
		{#if usage.jobsPerDay > 0}
			{@const p = pct(usage.jobsLast24h, usage.jobsPerDay)}
			<div class="meter">
				<div class="meter-head">
					<span>{i18n.t('usage.jobs')}</span>
					<span class="mono">{usage.jobsLast24h} / {usage.jobsPerDay}</span>
				</div>
				<div class="track"><div class="fill" class:warn={p >= 80} style="width:{p}%"></div></div>
			</div>
		{/if}
		{#if usage.storageLimitBytes > 0}
			{@const p = pct(usage.storageBytes, usage.storageLimitBytes)}
			<div class="meter">
				<div class="meter-head">
					<span>{i18n.t('usage.storage')}</span>
					<span class="mono">{size(usage.storageBytes)} / {size(usage.storageLimitBytes)}</span>
				</div>
				<div class="track"><div class="fill" class:warn={p >= 80} style="width:{p}%"></div></div>
			</div>
		{/if}
	</div>
{/if}

<style>
	.usage {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 14px 24px;
		margin: 0;
	}

	.meter {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.meter-head {
		display: flex;
		justify-content: space-between;
		font-size: 0.82rem;
		color: var(--text-muted);
	}

	.track {
		height: 6px;
		border-radius: 999px;
		background: var(--fill-2);
		overflow: hidden;
	}

	.fill {
		height: 100%;
		border-radius: inherit;
		background: var(--accent-bright);
		transition: width 0.3s ease;
	}

	.fill.warn {
		background: var(--danger);
	}
</style>
