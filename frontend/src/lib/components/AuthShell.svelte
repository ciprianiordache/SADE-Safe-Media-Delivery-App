<script lang="ts">
	// The sign-in pages (login, confirm, revoke) laid out like the marketing
	// page's split sections: what SADE's sign-in is on the left, the form as a
	// white card on a warm panel on the right. On a phone the card comes first.
	import type { Snippet } from 'svelte';
	import { i18n } from '$lib/i18n.svelte';
		import MarkBar from './MarkBar.svelte';
	import { IS_APP } from '$lib/platform';
	import logo from '$lib/assets/logo.png';

	let { children }: { children: Snippet } = $props();
</script>

<MarkBar />

{#if IS_APP}
	<main class="app-auth">
		<img class="app-icon" src={logo} alt="" width="56" height="56" />
		<div class="card form-card">
			{@render children()}
		</div>
	</main>
{:else}
<main class="container shell">
	<div class="copy">
		<p class="big" aria-hidden="true">{i18n.t('auth.heading')}</p>
		<ul class="points">
			<li>
				<span class="pt-title">{i18n.t('auth.point1Title')}</span>
				<span class="muted">{i18n.t('auth.point1Body')}</span>
			</li>
			<li>
				<span class="pt-title">{i18n.t('auth.point2Title')}</span>
				<span class="muted">{i18n.t('auth.point2Body')}</span>
			</li>
		</ul>
	</div>
	<div class="stage">
		<div class="card form-card">
			{@render children()}
		</div>
		</div>
</main>
{/if}

<style>
		/* Below the 44px title bar, the sign-in scrolls on its own (the window
	   itself never does); "safe" keeps the top reachable when it overflows. */
	.app-auth {
		height: calc(100vh - 44px);
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: safe center;
		gap: 18px;
		padding: 24px 16px 48px;
	}

	.app-icon {
		display: block;
		filter: drop-shadow(0 6px 14px rgb(232 83 30 / 0.28));
	}

	.app-auth .form-card {
		max-width: 380px;
		padding: 24px;
	}

	.shell {
		display: grid;
		grid-template-columns: minmax(0, 5fr) minmax(0, 6fr);
		gap: 48px;
		align-items: start;
		padding-top: 56px;
		padding-bottom: 96px;
	}

	@media (max-width: 899px) {
		.shell {
			grid-template-columns: minmax(0, 1fr);
			gap: 40px;
			padding-top: 24px;
			padding-bottom: 64px;
		}
		.stage {
			order: -1;
		}
	}

	.copy {
		padding-top: 16px;
	}

	/* The page's statement: the marketing page's section heading size. It is
	   decorative (the card holds the real <h1>), so screen readers skip it. */
	.big {
		margin: 0;
		font-size: clamp(2.2rem, 1.3rem + 2.8vw, 3.5rem);
		font-weight: 500;
		line-height: 1.04;
		letter-spacing: -0.035em;
		max-width: 14ch;
	}

	.points {
		list-style: none;
		margin: 40px 0 0;
		padding: 0;
		max-width: 46ch;
	}

	.points li {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 16px 0;
		border-top: 1px solid var(--line);
		font-size: 0.95rem;
		line-height: 1.5;
	}

	.points li:last-child {
		border-bottom: 1px solid var(--line);
	}

	.pt-title {
		font-weight: 500;
		font-size: 1.02rem;
	}

	/* The warm panel the marketing page sets its product cards on. */
	.stage {
		display: flex;
		justify-content: center;
		align-items: flex-start;
		padding: 56px 24px;
		min-height: 520px;
		border-radius: var(--radius);
		background: color-mix(in oklab, var(--surface-2) 70%, var(--bg));
		box-shadow: inset 0 0 0 1px var(--line);
	}

	@media (max-width: 899px) {
		.stage {
			min-height: 0;
			padding: 24px 16px;
		}
	}

	@media (max-width: 479px) {
		.stage {
			padding: 0;
			background: none;
			box-shadow: none;
		}
	}

	.form-card {
		width: 100%;
		max-width: 420px;
		padding: 28px;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}

	@media (max-width: 479px) {
		.form-card {
			padding: 22px 18px;
		}
	}
</style>
