<script lang="ts">
	import { goto } from '$app/navigation';
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { auth } from '$lib/stores.svelte';
	import { toasts } from '$lib/toast.svelte';
	import { createPasskey, deviceName, isCancel, passkeysSupported } from '$lib/webauthn';
	import { deviceLabel, fullDate, relativeTime } from '$lib/format';
	import type { ActivityEntry, PasskeyInfo, SessionInfo } from '$lib/types';
	import { onMount } from 'svelte';

	let serverPasskeys = $state(true);
	let browserPasskeys = $state(true);
	let passkeys = $state<PasskeyInfo[]>([]);
	let sessions = $state<SessionInfo[]>([]);
	let activity = $state<ActivityEntry[]>([]);
	let loading = $state(true);

	let adding = $state(false);
	let armedDelete = $state('');
	let requireBusy = $state(false);

	async function loadAll() {
		const [cfg, pk, ss, act] = await Promise.all([
			api.authConfig().catch(() => ({ passkeys: false })),
			api.passkeys().catch(() => []),
			api.sessions().catch(() => []),
			api.activity().catch(() => [])
		]);
		serverPasskeys = cfg.passkeys;
		passkeys = pk;
		sessions = ss;
		activity = act;
		loading = false;
	}

	onMount(() => {
		browserPasskeys = passkeysSupported();
		loadAll();
	});

	async function addPasskey() {
		adding = true;
		try {
			const ch = await api.passkeyRegisterBegin();
			const credential = await createPasskey(ch.options);
			const pk = await api.passkeyRegisterFinish(ch.ceremonyId, credential, deviceName());
			passkeys = [...passkeys, pk];
			toasts.show(i18n.t('settings.passkeyAdded'));
		} catch (err) {
			if (!isCancel(err)) toasts.show(i18n.t('settings.passkeyFailed'), { kind: 'error' });
		} finally {
			adding = false;
		}
	}

	async function removePasskey(id: string) {
		if (armedDelete !== id) {
			armedDelete = id;
			return;
		}
		armedDelete = '';
		try {
			await api.deletePasskey(id);
			passkeys = passkeys.filter((p) => p.id !== id);
			// Removing the last one also turns the requirement off server-side.
			if (passkeys.length === 0) await auth.refresh();
			toasts.show(i18n.t('settings.passkeyDeleted'));
		} catch {
			toasts.show(i18n.t('common.error'), { kind: 'error' });
		}
	}

	async function toggleRequire() {
		if (!auth.user) return;
		const next = !auth.user.requirePasskey;
		requireBusy = true;
		try {
			auth.user = await api.setRequirePasskey(next);
			toasts.show(i18n.t(next ? 'settings.requireOnToast' : 'settings.requireOffToast'));
		} catch (err) {
			toasts.show(
				err instanceof ApiError && err.status === 409 ? i18n.t('settings.requireNeedsPasskey') : i18n.t('common.error'),
				{ kind: 'error' }
			);
		} finally {
			requireBusy = false;
		}
	}

	async function revoke(s: SessionInfo) {
		try {
			await api.revokeSession(s.id);
			if (s.current) {
				auth.user = null;
				goto('/login');
				return;
			}
			sessions = sessions.filter((x) => x.id !== s.id);
			toasts.show(i18n.t('settings.sessionRevoked'));
		} catch {
			toasts.show(i18n.t('common.error'), { kind: 'error' });
		}
	}

	let passkeysUsable = $derived(serverPasskeys && browserPasskeys);
</script>

<svelte:head>
	<title>{i18n.t('settings.title')} · SADE</title>
</svelte:head>

<div class="container page">
	<a class="back" href="/app">
		<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
			><path d="M15 6 L9 12 L15 18" /></svg
		>
		{i18n.t('job.back')}
	</a>
	<div>
		<h1>{i18n.t('settings.title')}</h1>
		<p class="subtitle muted">{i18n.t('settings.subtitle')}</p>
	</div>

	{#if loading}
		<p class="muted">{i18n.t('common.loading')}</p>
	{:else}
		<!-- passkeys -->
		<section class="card" id="passkeys">
			<div class="section-head">
				<div>
					<h2>{i18n.t('settings.passkeysTitle')}</h2>
					<p class="muted intro">{i18n.t('settings.passkeysIntro')}</p>
				</div>
				{#if passkeysUsable}
					<button class="btn" type="button" onclick={addPasskey} disabled={adding}>
						{adding ? i18n.t('settings.passkeyAdding') : i18n.t('settings.passkeyAdd')}
					</button>
				{/if}
			</div>

			{#if !serverPasskeys}
				<p class="notice">{i18n.t('settings.passkeysUnavailable')}</p>
			{:else if !browserPasskeys}
				<p class="notice">{i18n.t('settings.passkeysUnsupported')}</p>
			{/if}

			{#if passkeys.length === 0}
				<p class="faint empty">{i18n.t('settings.passkeysEmpty')}</p>
			{:else}
				<ul class="rows">
					{#each passkeys as pk (pk.id)}
						<li class="row">
							<span class="row-icon" aria-hidden="true">
								<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"
									><circle cx="8" cy="15" r="4" /><path d="M10.8 12.2 L19 4 M16 7 l3 3 M14 9 l2 2" /></svg
								>
							</span>
							<span class="row-main">
								<span class="bold">{pk.name}</span>
								<span class="faint small">
									{i18n.t('settings.passkeyMeta', {
										created: fullDate(pk.createdAt, i18n.locale),
										used:
											pk.lastUsedAt && !pk.lastUsedAt.startsWith('0001')
												? i18n.t('settings.passkeyUsed', { when: relativeTime(pk.lastUsedAt, i18n.locale) })
												: i18n.t('settings.passkeyNeverUsed')
									})}
								</span>
							</span>
							<button
								type="button"
								class="btn btn-ghost small-btn"
								class:armed={armedDelete === pk.id}
								onclick={() => removePasskey(pk.id)}
								onblur={() => armedDelete === pk.id && setTimeout(() => (armedDelete = ''), 200)}
							>
								{armedDelete === pk.id ? i18n.t('settings.passkeyDeleteConfirm') : i18n.t('settings.passkeyDelete')}
							</button>
						</li>
					{/each}
				</ul>
			{/if}

			{#if serverPasskeys}
				<div class="require">
					<div>
						<span class="bold">{i18n.t('settings.requireTitle')}</span>
						<p class="muted small">{i18n.t('settings.requireBody')}</p>
					</div>
					<button
						type="button"
						class="switch"
						role="switch"
						aria-checked={auth.user?.requirePasskey ?? false}
						aria-label={i18n.t('settings.requireTitle')}
						disabled={requireBusy || (passkeys.length === 0 && !auth.user?.requirePasskey)}
						onclick={toggleRequire}
					>
						<span class="knob"></span>
					</button>
				</div>
			{/if}
		</section>

		<!-- devices -->
		<section class="card">
			<h2>{i18n.t('settings.sessionsTitle')}</h2>
			<p class="muted intro">{i18n.t('settings.sessionsIntro')}</p>
			<ul class="rows">
				{#each sessions as s (s.id)}
					<li class="row">
						<span class="row-icon" aria-hidden="true">
							<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"
								><rect x="3" y="4" width="18" height="12" rx="1.5" /><path d="M8 20 h8 M12 16 v4" /></svg
							>
						</span>
						<span class="row-main">
							<span class="bold">
								{deviceLabel(s.device, i18n.locale)}
								{#if s.current}<span class="pill">{i18n.t('settings.sessionCurrent')}</span>{/if}
							</span>
							<span class="faint small" title={fullDate(s.lastSeenAt, i18n.locale)}>
								{i18n.t('settings.sessionMeta', { when: relativeTime(s.lastSeenAt, i18n.locale), ip: s.ip || '—' })}
							</span>
						</span>
						<button type="button" class="btn btn-ghost small-btn" onclick={() => revoke(s)}>
							{i18n.t('settings.sessionRevoke')}
						</button>
					</li>
				{/each}
			</ul>
		</section>

		<!-- activity -->
		<section class="card">
			<h2>{i18n.t('settings.activityTitle')}</h2>
			{#if activity.length === 0}
				<p class="faint empty">{i18n.t('settings.activityEmpty')}</p>
			{:else}
				<ul class="rows compact">
					{#each activity as a, i (i)}
						<li class="row">
							<span class="row-main">
								<span>{deviceLabel(a.device, i18n.locale)} <span class="faint">· {i18n.t(`settings.method.${a.method}`)}</span></span>
								<span class="faint small">{fullDate(a.createdAt, i18n.locale)} · IP {a.ip || '—'}</span>
							</span>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}
</div>

<style>
	.page {
		padding: 1.5rem 1.75rem 3rem;
		max-width: 760px;
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
	}

	@media (max-width: 520px) {
		.page {
			padding: 1.25rem 1rem 3rem;
		}
	}

	.back {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		color: var(--text-muted);
		font-size: 0.85rem;
		align-self: flex-start;
	}

	h1 {
		margin: 0;
		font-size: 1.55rem;
		font-weight: 700;
	}

	.subtitle {
		margin: 0.2rem 0 0;
		font-size: 0.85rem;
	}

	h2 {
		margin: 0;
		font-size: 1.05rem;
		font-weight: 700;
	}

	.card {
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
	}

	.section-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}

	@media (max-width: 600px) {
		.section-head {
			flex-direction: column;
		}
	}

	.section-head .btn {
		white-space: nowrap;
	}

	.intro {
		margin: 0.3rem 0 0;
		font-size: 0.85rem;
		line-height: 1.5;
	}

	.notice {
		margin: 0;
		padding: 0.65rem 0.8rem;
		border-radius: var(--radius-sm);
		background: var(--warning-tint);
		color: var(--warning);
		font-size: 0.83rem;
		line-height: 1.5;
	}

	.empty {
		margin: 0;
		font-size: 0.85rem;
	}

	.rows {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
	}

	.row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.7rem 0;
		border-top: 1px solid var(--border);
	}

	.rows.compact .row {
		padding: 0.55rem 0;
	}

	.row-icon {
		width: 34px;
		height: 34px;
		flex-shrink: 0;
		border-radius: 8px;
		background: var(--bg);
		color: var(--text-muted);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.row-main {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		font-size: 0.88rem;
	}

	.bold {
		font-weight: 600;
	}

	.small {
		font-size: 0.78rem;
		line-height: 1.45;
		margin: 0;
	}

	.pill {
		display: inline-block;
		margin-left: 0.4rem;
		padding: 0.08rem 0.5rem;
		border-radius: 999px;
		background: var(--success-tint);
		color: var(--success);
		font-size: 0.7rem;
		font-weight: 600;
		vertical-align: middle;
	}

	.small-btn {
		padding: 0.4rem 0.75rem;
		font-size: 0.8rem;
		white-space: nowrap;
	}

	.small-btn.armed {
		border-color: var(--danger);
		color: var(--danger);
	}

	.require {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding-top: 0.9rem;
		border-top: 1px solid var(--border);
	}

	.require p {
		margin-top: 0.25rem;
	}

	.switch {
		appearance: none;
		flex-shrink: 0;
		width: 44px;
		height: 26px;
		border-radius: 999px;
		border: none;
		background: var(--border-strong);
		position: relative;
		cursor: pointer;
		transition: background 0.15s;
	}

	.switch[aria-checked='true'] {
		background: var(--accent);
	}

	.switch:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.knob {
		position: absolute;
		top: 3px;
		left: 3px;
		width: 20px;
		height: 20px;
		border-radius: 999px;
		background: #fff;
		box-shadow: 0 1px 3px rgb(0 0 0 / 0.25);
		transition: transform 0.15s;
	}

	.switch[aria-checked='true'] .knob {
		transform: translateX(18px);
	}
</style>
