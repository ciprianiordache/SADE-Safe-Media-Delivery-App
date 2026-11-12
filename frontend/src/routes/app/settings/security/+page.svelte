<script lang="ts">
	import { goto } from '$app/navigation';
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { auth } from '$lib/stores.svelte';
	import { toasts } from '$lib/toast.svelte';
	import { createPasskey, deviceName, isCancel, passkeysSupported } from '$lib/webauthn';
	import { ago, deviceLabel, fullDate } from '$lib/format';
	import type { ActivityEntry, PasskeyInfo, SessionInfo } from '$lib/types';
	import { onMount } from 'svelte';
	import SettingsTabs from '$lib/components/SettingsTabs.svelte';
	import { IS_APP } from '$lib/platform';

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

		// Signing this very session out is leaving the account: ask once.
	let selfArmed = $state(false);

	async function revoke(s: SessionInfo) {
		if (s.current && !selfArmed) {
			selfArmed = true;
			return;
		}
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

	// Sessions signed in from the installed app say "SADE app on ...". This
	// app's own session may predate that marker; it is the app all the same.
	function sessionLabel(s: SessionInfo): string {
		const d = IS_APP && s.current ? s.device.replace(/^.*? on /, 'SADE app on ') : s.device;
		return deviceLabel(d, i18n.locale);
	}

	// Sessions from the same kind of device (the same browser on the same
	// system) fold into one row: seven "Chrome pe Windows" rows say one thing.
	// This device always stays a row of its own.
	type Group = { key: string; label: string; items: SessionInfo[] };
	let groups = $derived.by(() => {
		const out: Group[] = [];
		const byLabel = new Map<string, Group>();
		for (const s of sessions) {
			const label = sessionLabel(s);
			if (s.current) {
				out.unshift({ key: s.id, label, items: [s] });
				continue;
			}
			let g = byLabel.get(label);
			if (!g) {
				g = { key: 'g:' + label, label, items: [] };
				byLabel.set(label, g);
				out.push(g);
			}
			g.items.push(s);
		}
		return out;
	});
	let expanded = $state<Record<string, boolean>>({});
	let revokingGroup = $state('');

	function latest(items: SessionInfo[]): string {
		return items.reduce((a, b) => (a.lastSeenAt > b.lastSeenAt ? a : b)).lastSeenAt;
	}

	async function revokeGroup(g: Group) {
		revokingGroup = g.key;
		try {
			for (const s of g.items) await api.revokeSession(s.id);
			const gone = new Set(g.items.map((s) => s.id));
			sessions = sessions.filter((x) => !gone.has(x.id));
			toasts.show(i18n.t('settings.sessionsRevoked', { n: g.items.length }));
		} catch {
			toasts.show(i18n.t('common.error'), { kind: 'error' });
		} finally {
			revokingGroup = '';
		}
	}
</script>

<svelte:head>
	<title>{i18n.t('settings.tabSecurity')} · SADE</title>
</svelte:head>

<div class="container page">
		<SettingsTabs lead={i18n.t('settings.securityLead')} />

	{#if loading}
		<div class="set-row" aria-busy="true" aria-label={i18n.t('common.loading')}>
			<span class="sk" style="height:5rem"></span>
			<span class="sk" style="height:220px"></span>
		</div>
	{:else}
		<!-- passkeys: a heading and its text on the left, the panel on the right -->
		<!-- Passkeys are tied to the server's domain, so the app cannot create
		     or use them; they are managed from SADE in the browser. -->
		{#if !IS_APP}
		<section class="set-row" id="passkeys" aria-labelledby="pk-title">
			<div class="set-head">
				<h2 id="pk-title">{i18n.t('settings.passkeysTitle')}</h2>
				<p class="muted intro">{i18n.t('settings.passkeysIntro')}</p>
				{#if passkeysUsable}
					<button class="btn btn-ink" type="button" onclick={addPasskey} disabled={adding}>
						{adding ? i18n.t('settings.passkeyAdding') : i18n.t('settings.passkeyAdd')}
					</button>
				{/if}
			</div>
			<div class="win set-body">


			{#if !serverPasskeys}
				<p class="notice">{i18n.t('settings.passkeysUnavailable')}</p>
			{:else if !browserPasskeys}
				<p class="notice">{i18n.t('settings.passkeysUnsupported')}</p>
			{/if}

			{#if passkeys.length === 0}
				<p class="muted empty">{i18n.t('settings.passkeysEmpty')}</p>
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
								<span class="muted small">
									{i18n.t('settings.passkeyMeta', {
										created: fullDate(pk.createdAt, i18n.locale),
										used:
											pk.lastUsedAt && !pk.lastUsedAt.startsWith('0001')
												? i18n.t('settings.passkeyUsed', { when: ago(pk.lastUsedAt, i18n.locale) })
												: i18n.t('settings.passkeyNeverUsed')
									})}
								</span>
							</span>
													<button
							type="button"
							class="btn btn-line btn-sm"
							class:danger={armedDelete === pk.id}
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
					></button>
				</div>
			{/if}
			</div>
		</section>
		{/if}

		<!-- devices -->
		<section class="set-row" aria-labelledby="dev-title">
			<div class="set-head">
				<h2 id="dev-title">{i18n.t('settings.sessionsTitle')}</h2>
				<p class="muted intro">{i18n.t('settings.sessionsIntro')}</p>
			</div>
			<div class="win set-body">
			<ul class="rows">
								{#each groups as g (g.key)}
				{#if g.items.length > 1}
					<li class="row group">
						<span class="row-icon" aria-hidden="true">
							<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="12" rx="1.5" /><path d="M8 20 h8 M12 16 v4" /></svg>
						</span>
						<span class="row-main">
							<span class="bold">{g.label} <span class="muted count">· {i18n.t('settings.sessionsCount', { n: g.items.length })}</span></span>
							<span class="muted small">{i18n.t('settings.sessionsLastSeen', { when: ago(latest(g.items), i18n.locale) })}</span>
						</span>
						<button type="button" class="btn btn-ghost btn-sm" aria-expanded={!!expanded[g.key]} onclick={() => (expanded[g.key] = !expanded[g.key])}>
							{expanded[g.key] ? i18n.t('settings.sessionsHide') : i18n.t('settings.sessionsShow')}
						</button>
						<button type="button" class="btn btn-line btn-sm" disabled={revokingGroup === g.key} onclick={() => revokeGroup(g)}>
							{i18n.t('settings.sessionsRevokeAll')}
						</button>
					</li>
				{/if}
				{#each g.items.length > 1 && !expanded[g.key] ? [] : g.items as s (s.id)}
					<li class="row" class:nested={g.items.length > 1}>
						<span class="row-icon" aria-hidden="true">
							<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"
								><rect x="3" y="4" width="18" height="12" rx="1.5" /><path d="M8 20 h8 M12 16 v4" /></svg
							>
						</span>
						<span class="row-main">
							<span class="bold">
								{sessionLabel(s)}
								{#if s.current}<span class="chip chip-ok">{i18n.t('settings.sessionCurrent')}</span>{/if}
							</span>
							<span class="muted small" title={fullDate(s.lastSeenAt, i18n.locale)}>
								{i18n.t('settings.sessionMeta', { when: ago(s.lastSeenAt, i18n.locale), ip: s.ip || '—' })}
							</span>
						</span>
																		{#if s.current && selfArmed}
							<button type="button" class="btn btn-ghost btn-sm" onclick={() => (selfArmed = false)}>{i18n.t('common.cancel')}</button>
						{/if}
						<button
							type="button"
							class="btn btn-line btn-sm"
							class:self-armed={s.current && selfArmed}
							title={s.current ? i18n.t('settings.sessionSignOutSelfHint') : undefined}
							onclick={() => revoke(s)}
						>
							{s.current && selfArmed ? i18n.t('settings.sessionSignOutSelf') : i18n.t('settings.sessionRevoke')}
						</button>
					</li>
				{/each}
				{/each}
			</ul>
			</div>
		</section>

		<!-- activity -->
		<section class="set-row" aria-labelledby="act-title">
			<div class="set-head">
				<h2 id="act-title">{i18n.t('settings.activityTitle')}</h2>
				<p class="muted intro">{i18n.t('settings.activityIntro')}</p>
			</div>
			<div class="win set-body">
			{#if activity.length === 0}
				<p class="muted empty">{i18n.t('settings.activityEmpty')}</p>
			{:else}
				<ul class="rows compact">
					{#each activity as a, i (i)}
						<li class="row">
							<span class="row-main">
								<span>{deviceLabel(a.device, i18n.locale)} <span class="muted">· {i18n.t(`settings.method.${a.method}`)}</span></span>
								<span class="muted small">{fullDate(a.createdAt, i18n.locale)} · IP {a.ip || '—'}</span>
							</span>
						</li>
					{/each}
								</ul>
			{/if}
			</div>
		</section>

		{#if IS_APP}
			<!-- In the app passkeys can only be pointed to (they belong to the
			     server's domain), so this note comes last, not first. -->
			<section class="set-row" aria-labelledby="pk-title">
				<div class="set-head">
					<h2 id="pk-title">{i18n.t('settings.passkeysTitle')}</h2>
					<p class="muted intro">{i18n.t('settings.passkeysAppNote')}</p>
				</div>
			</section>
		{/if}
	{/if}
</div>

<style>
	/* The marketing page's split: the heading and its text left, the panel right. */
	.set-row {
		display: grid;
		grid-template-columns: minmax(0, 4fr) minmax(0, 8fr);
		gap: 16px 48px;
		align-items: start;
		padding: 32px 0;
		border-top: 1px solid var(--line);
	}

	.set-row:first-of-type {
		border-top: none;
		padding-top: 8px;
	}

		@media (max-width: 860px) {
		.set-row {
			grid-template-columns: minmax(0, 1fr);
			padding: 24px 0;
		}
	}

	/* The app: one column, the heading over its group, as a settings window. */
	:global(html.is-app) .set-row {
		grid-template-columns: minmax(0, 1fr);
		max-width: 760px;
		gap: 12px;
		padding: 24px 0;
	}

	:global(html.is-app) .set-head {
		gap: 4px;
	}

	:global(html.is-app) h2 {
		font-size: 1.125rem;
	}

	.set-head {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 10px;
	}

	h2 {
		margin: 0;
		font-size: 1.35rem;
		font-weight: 500;
	}

		.self-armed {
		background: var(--danger);
		color: #fff;
		border-color: transparent;
	}

	.intro {
		margin: 0;
		font-size: 0.95rem;
		line-height: 1.55;
		text-wrap: pretty;
	}

	.set-head .btn {
		margin-top: 6px;
	}

	.set-body {
		padding: 4px 20px;
	}

	.notice {
		margin: 16px 0 0;
		padding: 0.7rem 0.85rem;
		border-radius: var(--radius-sm);
		background: var(--warning-tint);
		color: var(--warning);
		font-size: 0.88rem;
		line-height: 1.5;
	}

	.empty {
		margin: 0;
		padding: 18px 0;
		font-size: 0.92rem;
	}

	.row {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 14px 0;
	}

	.rows.compact .row {
		padding: 10px 0;
	}

	.row-icon {
		width: 36px;
		height: 36px;
		flex-shrink: 0;
		border-radius: 50%;
		background: var(--fill-2);
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
		gap: 2px;
		font-size: 0.92rem;
	}

	.bold {
		font-weight: 500;
	}

	.small {
		font-size: 0.85rem;
		line-height: 1.45;
		margin: 0;
	}

	.chip {
		height: 22px;
		margin-left: 6px;
		vertical-align: 1px;
	}

		.count {
		font-weight: 400;
	}

	.row.group .btn + .btn {
		margin-left: -4px;
	}

	.row.nested {
		padding-left: 48px;
	}

	.require {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 14px 0;
		border-top: 1px solid var(--line);
	}

	.require p {
		margin-top: 2px;
	}
</style>
