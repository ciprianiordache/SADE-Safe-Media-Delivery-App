<script lang="ts">
	import { i18n } from '$lib/i18n.svelte';
	import { api, ApiError } from '$lib/api';
	import { isAllowedFile, mediaTypeOf } from '$lib/types';
	import type { Job, MediaType, Usage, WatermarkKind } from '$lib/types';
	import { formatBytes, fullDate, relativeTime } from '$lib/format';
	import { toasts } from '$lib/toast.svelte';
	import { brand } from '$lib/stores.svelte';
	import { deliveryState, initials, stateClass, stateKey, tone } from '$lib/delivery';
	import MediaIcon from '$lib/components/MediaIcon.svelte';
	import UsageMeter from '$lib/components/UsageMeter.svelte';
	import PasskeyBanner from '$lib/components/PasskeyBanner.svelte';
	import BrandBanner from '$lib/components/BrandBanner.svelte';
	import LogoPicker from '$lib/components/LogoPicker.svelte';
		import JobPanel from '$lib/components/JobPanel.svelte';
	import WindowControls from '$lib/components/WindowControls.svelte';
	import AccountMenu from '$lib/components/AccountMenu.svelte';
		import { IS_APP, desktopOS } from '$lib/platform';

	const MOD = desktopOS() === 'mac' ? '⌘' : 'Ctrl+';
	import logo from '$lib/assets/logo.png';
	import { renderWatermarkText, withRecipient } from '$lib/format';
	import { suggestEmail } from '$lib/email';
	import { page } from '$app/state';
		import { goto, replaceState } from '$app/navigation';
	import { tick } from 'svelte';
	import { onDestroy, onMount } from 'svelte';

	// --- data -----------------------------------------------------------
	let jobs = $state<Job[]>([]);
	let jobsLoading = $state(true);
	let usage = $state<Usage | null>(null);

	async function loadUsage() {
		try {
			usage = await api.usage();
		} catch {
			// The meter is informational; the dashboard works without it.
		}
	}

	// Tells the operator when a job they are waiting on finishes, so they
	// don't have to keep watching the list.
	function announceFinished(prev: Job[], next: Job[]) {
		const before = new Map(prev.map((j) => [j.id, j.status]));
		for (const j of next) {
			const was = before.get(j.id);
			if (was !== 'pending' && was !== 'processing') continue;
			if (j.status === 'done') {
				toasts.show(i18n.t('dashboard.readyToast', { email: j.recipientEmail }), {
					href: `/app/jobs/${j.id}`,
					action: i18n.t('dashboard.seeJob')
				});
			} else if (j.status === 'failed') {
				toasts.show(i18n.t('dashboard.failedToast', { email: j.recipientEmail }), {
					kind: 'error',
					href: `/app/jobs/${j.id}`,
					action: i18n.t('dashboard.seeJob'),
					ms: 8000
				});
			}
		}
	}

	// A failed load is its own state: never shown as an empty account (which
	// would also open the upload form), and an already-loaded list stays, with
	// when it was last fresh.
	let loadError = $state(false);
	let lastLoaded = $state<Date | null>(null);
	let retrying = $state(false);

	async function loadJobs() {
		try {
			const next = await api.listJobs(0, 200);
			if (!jobsLoading) announceFinished(jobs, next);
			jobs = next;
			loadError = false;
			lastLoaded = new Date();
		} catch {
			// A 401 is handled by the layout (back to sign-in); anything else
			// means the server could not be reached or failed.
			loadError = true;
		} finally {
			jobsLoading = false;
		}
	}

	async function retryLoad() {
		retrying = true;
		await loadJobs();
		retrying = false;
	}

	let lastLoadedText = $derived(
		lastLoaded ? lastLoaded.toLocaleTimeString(i18n.locale === 'ro' ? 'ro-RO' : 'en-GB', { hour: '2-digit', minute: '2-digit' }) : ''
	);

	// --- the split view --------------------------------------------------
		// From 1024px the window is the marketing page's picture: the list on the
	// left and the open delivery on the right. "Fișier nou" turns the right
	// pane into the upload form. Narrower, a row opens the job page and the
	// form sits under the list.
		let wide = $state(false);
	let overlay = $state(false);
	let paneMode = $state<'job' | 'upload'>('job');
	let selectedId = $state(page.url.searchParams.get('job') ?? '');
	let selectedJob = $derived(jobs.find((j) => j.id === selectedId));
	// A link to /app?job=<id> while already here is a real navigation that
	// keeps this page: follow it (shallow replaceState never changes page.url).
	$effect(() => {
		const q = page.url.searchParams.get('job');
		if (q) {
			selectedId = q;
			paneMode = 'job';
		}
	});
		let showUpload = $derived(paneMode === 'upload' || (!jobsLoading && !loadError && jobs.length === 0));
	// The web opens the newest delivery by default; the installed app keeps
	// the list full width until a row (or "Fișier nou") opens the pane.
	let paneOpen = $derived(!IS_APP || showUpload || !!selectedJob);

	function closePane() {
		selectedId = '';
		paneMode = 'job';
		syncUrl();
	}

	function syncUrl() {
				const url = new URL(location.href);
		if (wide && paneMode === 'job' && selectedId) url.searchParams.set('job', selectedId);
		else url.searchParams.delete('job');
				// page.url does not follow a shallow replaceState: compare with the
		// address bar itself, or a closed pane stays in the URL (and F5 reopens it).
		if (url.search !== location.search) replaceState(url, page.state);
	}

	function select(e: MouseEvent, id: string) {
		// Narrow screens and modified clicks follow the link to the job page.
		if (!wide || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
		e.preventDefault();
		selectedId = id;
		paneMode = 'job';
		syncUrl();
	}

	function openUpload() {
		if (wide) {
			paneMode = 'upload';
			syncUrl();
			tick().then(() => uploadCard?.querySelector<HTMLElement>('.dropzone, input')?.focus());
		} else {
			jumpToUpload();
		}
	}

	function closeUpload() {
		paneMode = 'job';
		syncUrl();
	}

		let searchInput = $state<HTMLInputElement | undefined>();
	let rowsEl = $state<HTMLUListElement | undefined>();

	// Wide: Up/Down on a row open the previous/next delivery and keep focus
	// on it, as in a mail client. Narrow: rows are plain links.

	function onRowsKey(e: KeyboardEvent) {
		if (!wide || !['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(e.key)) return;
		const i = pageJobs.findIndex((j) => j.id === selectedId);
		const at = e.key === 'Home' ? 0 : e.key === 'End' ? pageJobs.length - 1 : e.key === 'ArrowDown' ? Math.min(pageJobs.length - 1, i + 1) : Math.max(0, i - 1);
		const next = pageJobs[at];
		if (!next) return;
		e.preventDefault();
		selectedId = next.id;
		paneMode = 'job';
		syncUrl();
		tick().then(() => rowsEl?.querySelector<HTMLElement>(`a[data-id="${next.id}"]`)?.focus());
	}

	// "/" jumps to the search box, as the mockup's window shows.
		function onKey(e: KeyboardEvent) {
		const t = e.target as HTMLElement | null;
		const typing = !!t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable);
		// Escape closes the app's open pane (the account menu handles its own).
				if (e.key === 'Escape' && ctx) {
			closeCtx(true);
			return;
		}
		if (e.key === 'Escape' && IS_APP && paneOpen && !typing && !document.querySelector('.menu')) {
			closePane();
			return;
		}
		// Ctrl/Cmd+F finds, as in every desktop program.
		if (IS_APP && (e.key === 'f' || e.key === 'F') && (e.ctrlKey || e.metaKey) && !e.altKey && searchInput) {
			e.preventDefault();
			searchInput.focus();
			searchInput.select();
			return;
		}
		if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return;
		if (typing) return;
		if (!searchInput) return;
		e.preventDefault();
		searchInput.focus();
	}

		// Right-click (or the menu key / Shift+F10) on a row: the actions a
	// desktop list offers there. App only; the web keeps the browser's menu.
	let ctx = $state<{ id: string; x: number; y: number } | null>(null);
	let ctxEl = $state<HTMLDivElement | undefined>();

	function openCtx(e: MouseEvent | KeyboardEvent, id: string) {
		if (!IS_APP) return;
		if (e instanceof KeyboardEvent) {
			if (e.key !== 'ContextMenu' && !(e.key === 'F10' && e.shiftKey)) return;
		}
		e.preventDefault();
		let x: number, y: number;
		if (e instanceof MouseEvent && e.clientX) {
			x = e.clientX;
			y = e.clientY;
		} else {
			const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
			x = r.left + 48;
			y = r.bottom - 6;
		}
		// Keep it inside the window.
		ctx = { id, x: Math.min(x, innerWidth - 260), y: Math.min(y, innerHeight - 150) };
		tick().then(() => ctxEl?.querySelector<HTMLElement>('button')?.focus());
	}

		// Enter on the open delivery goes on to its page; other keys may open
	// the row's menu.
	function onRowKey(e: KeyboardEvent, id: string) {
		if (e.key === 'Enter' && IS_APP && wide && id === selectedId && paneMode === 'job' && !e.ctrlKey && !e.metaKey) {
			e.preventDefault();
			goto(`/app/jobs/${id}`);
			return;
		}
		openCtx(e, id);
	}

	let ctxArmed = $state(false);
	let ctxJob = $derived(ctx ? jobs.find((j) => j.id === ctx!.id) : undefined);

	async function ctxResend() {
		if (!ctxJob) return;
		const j = ctxJob;
		closeCtx(true);
		try {
			await api.resend(j.id);
			toasts.show(i18n.t('delivery.resent', { email: j.recipientEmail }));
		} catch (err) {
			toasts.show(err instanceof ApiError && err.status === 429 ? i18n.t('delivery.resendLimit') : i18n.t('common.error'), { kind: 'error' });
		}
	}

	async function ctxLinks(enabled: boolean) {
		if (!ctxJob) return;
		// Switching off asks once more, inside the menu.
		if (!enabled && !ctxArmed) {
			ctxArmed = true;
			return;
		}
		const j = ctxJob;
		closeCtx(true);
		try {
			await api.setLinks(j.id, enabled);
			j.linksDisabled = !enabled;
			toasts.show(i18n.t(enabled ? 'delivery.enabledToast' : 'delivery.disabledToast'));
		} catch {
			toasts.show(i18n.t('common.error'), { kind: 'error' });
		}
	}

	function closeCtx(refocus = false) {
		ctxArmed = false;
		const id = ctx?.id;
		ctx = null;
		if (refocus && id) rowsEl?.querySelector<HTMLElement>(`a[data-id="${id}"]`)?.focus();
	}

	function ctxOpen() {
		if (!ctx) return;
		selectedId = ctx.id;
		paneMode = 'job';
		syncUrl();
		closeCtx(true);
	}

	async function ctxCopy() {
		if (!ctx) return;
		const id = ctx.id;
		closeCtx(true);
		try {
			const d = await api.delivery(id);
			if (!d.previewUrl) throw new Error('no link');
			await navigator.clipboard.writeText(d.previewUrl);
			toasts.show(i18n.t('delivery.copied'));
		} catch {
			toasts.show(i18n.t('delivery.copyFailed'), { kind: 'error' });
		}
	}

	function ctxPage() {
		if (!ctx) return;
		const id = ctx.id;
		closeCtx();
		goto(`/app/jobs/${id}`);
	}

	function onCtxKey(e: KeyboardEvent) {
		const items = [...(ctxEl?.querySelectorAll<HTMLElement>('button') ?? [])];
		const i = items.indexOf(document.activeElement as HTMLElement);
		if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			e.preventDefault();
			items[(i + (e.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length]?.focus();
		} else if (e.key === 'Tab') {
			closeCtx(true);
		}
	}

	let pollHandle: ReturnType<typeof setInterval> | undefined;
	onMount(() => {
				// The app is a window from 720px up (its minimum): always the shell,
		// the pane beside the list from 1024px, over it below that.
		const mq = matchMedia(IS_APP ? '(min-width: 720px)' : '(min-width: 1024px)');
		const roomy = matchMedia('(min-width: 1024px)');
		wide = mq.matches;
		overlay = IS_APP && !roomy.matches;
		const onMq = (e: MediaQueryListEvent) => (wide = e.matches);
		const onRoomy = (e: MediaQueryListEvent) => (overlay = IS_APP && !e.matches);
		mq.addEventListener('change', onMq);
		roomy.addEventListener('change', onRoomy);
		loadJobs();
		loadUsage();
		try {
			const k = localStorage.getItem(KIND_KEY);
			if (k === 'logo' || k === 'text' || k === 'both') watermarkKind = k;
		} catch {
			// no storage - keep the default
		}
		let ticks = 0;
		pollHandle = setInterval(() => {
			if (document.hidden) return;
			ticks++;
			// Fast while a file is being watermarked; otherwise every ~30s, so
			// "opened" and "paid" show up without a reload, and a server that
			// came back is noticed.
			if (jobs.some((j) => j.status === 'pending' || j.status === 'processing') || loadError || ticks % 8 === 0) loadJobs();
		}, 4000);
				return () => {
			mq.removeEventListener('change', onMq);
			roomy.removeEventListener('change', onRoomy);
		};
	});
	onDestroy(() => {
		if (pollHandle) clearInterval(pollHandle);
		uploadAbort?.abort();
	});

	// --- upload form ------------------------------------------------------
	const KIND_KEY = 'sade:watermarkKind';
	const KINDS: WatermarkKind[] = ['both', 'logo', 'text'];

	let file = $state<File | null>(null);
	let fileInput = $state<HTMLInputElement | undefined>();
	let fileError = $derived(file && !isAllowedFile(file.name) ? i18n.t('dashboard.fileUnsupported') : '');
	let fileKind = $derived(file ? mediaTypeOf(file.name) : null);
	let recipientEmail = $state('');
	let watermarkKind = $state<WatermarkKind>('both');
	let watermarkText = $state('');
	let submitting = $state(false);
	let progress = $state(0);
	let uploadError = $state('');
	let dragging = $state(false);
	let uploadAbort: AbortController | null = null;

	// Without their own logo an operator's files get text only (the server
	// enforces the same) - keep the form honest about it.
	// Unknown brand (not loaded, or the request failed): assume no logo, so
	// the form never offers what the server will not do.
	let hasLogo = $derived(brand.value?.hasLogo ?? false);
	// What we know about the logo: 'unknown' while the brand loads (the kind
	// control is shown neutral, with no message), 'none' once the server
	// confirmed there is none, 'yes', or 'error'.
	let logoState = $derived<'unknown' | 'none' | 'yes' | 'error'>(
		brand.status === 'ready' ? (hasLogo ? 'yes' : 'none') : brand.status === 'error' ? 'error' : 'unknown'
	);
	let logoPanel = $state(false); // inline LogoPicker (keeps the chosen file and recipient)

	// The text that would be burned in if the field is left empty.
	let defaultText = $derived(
		brand.value
			? renderWatermarkText(brand.value.textTemplate, brand.value.companyName, recipientEmail.trim() || i18n.t('brand.sampleRecipient'))
			: i18n.t('dashboard.watermarkTextPlaceholder')
	);

	// "Fișier nou" in the header (?upload=1): open the form, then drop the
	// flag so a reload or Back does not do it again. Waits for the media
	// query so it lands in the right place.
	let uploadCard = $state<HTMLElement | null>(null);
	let mounted = $state(false);
	onMount(() => (mounted = true));
	$effect(() => {
		if (!mounted || page.url.searchParams.get('upload') !== '1') return;
		tick().then(() => {
			const url = new URL(page.url);
			url.searchParams.delete('upload');
			replaceState(url, page.state);
			openUpload();
		});
	});

	// Wide screens open the newest delivery when nothing is chosen yet.
	$effect(() => {
				if (!IS_APP && wide && paneMode === 'job' && !selectedJob && sortedJobs.length > 0) selectedId = sortedJobs[0].id;
	});

	// What is shown and sent. Derived, not written back into watermarkKind,
	// so the operator's own choice returns once the brand has loaded.
	let kind = $derived<WatermarkKind>(hasLogo ? watermarkKind : 'text');

	function pickKind(k: WatermarkKind) {
		watermarkKind = k;
		try {
			localStorage.setItem(KIND_KEY, k);
		} catch {
			// best-effort
		}
	}

	// Recipient typo guard. The address is burned into the file and mailed at
	// once, so a likely typo stops the first submit (one more click sends anyway).
	let recipientTouched = $state(false);
	let typoAckFor = $state('');
	let typoBlocked = $state(false);
	let emailSuggestion = $derived(suggestEmail(recipientEmail));
	let showSuggestion = $derived(!!emailSuggestion && (recipientTouched || typoBlocked) && typoAckFor !== recipientEmail);
	let recipientInput = $state<HTMLInputElement | undefined>();

	function useSuggestion() {
		if (emailSuggestion) recipientEmail = emailSuggestion;
		typoBlocked = false;
		recipientInput?.focus();
	}

	function keepAddress() {
		typoAckFor = recipientEmail;
		typoBlocked = false;
	}

	// The summary above the send button: what goes where, before it is irreversible.
	let recipientLooksValid = $derived(/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(recipientEmail.trim()));
	// Mirrors the worker: own text keeps the recipient (appended when it
	// lacks it), and "Logo" still burns the recipient's address under it.
	let recipientShown = $derived(recipientEmail.trim() || i18n.t('brand.sampleRecipient'));
	let burnedText = $derived(
		kind === 'logo'
			? i18n.t('dashboard.summaryLogo', { email: recipientShown })
			: watermarkText.trim()
				? withRecipient(watermarkText, recipientShown)
				: defaultText
	);
	let recipientAppended = $derived(
		!!watermarkText.trim() && !watermarkText.toLowerCase().includes(recipientShown.toLowerCase())
	);

	function jumpToUpload() {
		uploadCard?.scrollIntoView({ block: 'start', behavior: 'smooth' });
		uploadCard?.querySelector<HTMLElement>('.dropzone, input')?.focus({ preventScroll: true });
	}

	function setFile(f: File | null) {
		file = f;
		uploadError = '';
	}

	function clearFile() {
		setFile(null);
		if (fileInput) fileInput.value = '';
	}

	function onFileChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		setFile(input.files?.[0] ?? null);
	}

	// dragenter/dragleave fire for every child element, so count them to know
	// when the pointer has really left the zone.
	let dragDepth = 0;
	function onDragEnter(e: DragEvent) {
		if (!e.dataTransfer?.types.includes('Files')) return;
		e.preventDefault();
		dragDepth++;
		dragging = true;
	}
	function onDragLeave() {
		dragDepth = Math.max(0, dragDepth - 1);
		if (dragDepth === 0) dragging = false;
	}
	function onDragOver(e: DragEvent) {
		if (!e.dataTransfer?.types.includes('Files')) return;
		e.preventDefault();
		e.dataTransfer.dropEffect = 'copy';
	}
	function onDrop(e: DragEvent) {
		e.preventDefault();
		dragDepth = 0;
		dragging = false;
		const f = e.dataTransfer?.files?.[0];
		if (f && !submitting) setFile(f);
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!file || fileError) return;
		if (emailSuggestion && typoAckFor !== recipientEmail) {
			typoBlocked = true;
			recipientInput?.focus();
			return;
		}
		uploadError = '';
		submitting = true;
		progress = 0;
		uploadAbort = new AbortController();
		const email = recipientEmail;
		try {
			const job = await api.createJob(
				{
					file,
					recipientEmail,
					watermarkKind: kind,
					watermarkText: (kind !== 'logo' && watermarkText.trim()) || undefined
				},
				{ onProgress: (p) => (progress = p), signal: uploadAbort.signal }
			);
			// The create response carries no file name yet; keep the one we sent.
			const added: Job = { ...job, fileName: job.fileName ?? file.name, sizeBytes: job.sizeBytes ?? file.size };
			// A poll that ran during the upload may already have fetched this job.
			jobs = [added, ...jobs.filter((j) => j.id !== job.id)];
			loadUsage();
			clearFile();
			recipientEmail = '';
			recipientTouched = false;
			typoAckFor = '';
			watermarkText = '';
			// Wide: the new delivery opens beside the list.
			if (wide) {
				selectedId = job.id;
				paneMode = 'job';
				syncUrl();
			}
			toasts.show(i18n.t('dashboard.uploadedToast', { email }), {
				href: `/app/jobs/${job.id}`,
				action: i18n.t('dashboard.seeJob')
			});
		} catch (err) {
			if (err instanceof DOMException && err.name === 'AbortError') {
				toasts.show(i18n.t('dashboard.uploadCanceled'), { kind: 'info' });
			} else {
				uploadError = uploadErrorMessage(err);
			}
		} finally {
			submitting = false;
			uploadAbort = null;
		}
	}

	// Server errors arrive in English; show the common ones in the UI language.
	function uploadErrorMessage(err: unknown): string {
		if (!(err instanceof ApiError)) return i18n.t('dashboard.uploadError');
		switch (err.status) {
			case 429:
				return i18n.t('dashboard.errorDailyLimit');
			case 413:
				return err.message.includes('quota') ? i18n.t('dashboard.errorStorageQuota') : i18n.t('dashboard.errorTooLarge');
			case 415:
				return i18n.t('dashboard.errorUnsupported');
			case 400:
				return i18n.t('dashboard.errorInvalid');
			default:
				return i18n.t('dashboard.uploadError');
		}
	}

	// --- list controls ----------------------------------------------------
	// The sidebar of the marketing page's list: where the deliveries stand,
	// then the kind of file.
	type Filter = 'all' | 'working' | 'opened' | 'paid' | 'off' | 'failed';
	type TypeFilter = 'all' | MediaType;
	type Sort = 'newest' | 'oldest' | 'status';
	const PAGE_SIZE = 15;
	const FILTERS: Filter[] = ['all', 'working', 'opened', 'paid', 'off', 'failed'];
	const TYPES: MediaType[] = ['video', 'image', 'audio'];

	let search = $state('');
	let filter = $state<Filter>('all');
	let typeFilter = $state<TypeFilter>('all');
	let sort = $state<Sort>('newest');
	let currentPage = $state(1);

	function inFilter(j: Job, f: Filter): boolean {
		switch (f) {
			case 'all':
				return true;
			case 'working':
				return j.status === 'pending' || j.status === 'processing';
			case 'opened':
				return !!j.openedAt;
			case 'paid':
				return j.paid;
			case 'off':
				return j.linksDisabled;
			case 'failed':
				return j.status === 'failed';
		}
	}

	let searchedJobs = $derived.by(() => {
		const q = search.trim().toLowerCase();
		if (!q) return jobs;
		return jobs.filter((j) => j.recipientEmail.toLowerCase().includes(q) || (j.fileName ?? '').toLowerCase().includes(q));
	});

	let typedJobs = $derived(typeFilter === 'all' ? searchedJobs : searchedJobs.filter((j) => j.mediaType === typeFilter));

	let counts = $derived.by(() => {
		const c = Object.fromEntries(FILTERS.map((f) => [f, 0])) as Record<Filter, number>;
		for (const j of typedJobs) for (const f of FILTERS) if (inFilter(j, f)) c[f]++;
		return c;
	});

	let typeCounts = $derived.by(() => {
		const c: Record<MediaType, number> = { video: 0, image: 0, audio: 0 };
		for (const j of searchedJobs) c[j.mediaType]++;
		return c;
	});

	// An empty "Eșuate" is noise: only filters with something in them (or the chosen one).
	let visibleFilters = $derived(FILTERS.filter((f) => f === 'all' || f === filter || counts[f] > 0));
	let visibleTypes = $derived(TYPES.filter((t) => t === typeFilter || typeCounts[t] > 0));

	const statusOrder: Record<Job['status'], number> = { processing: 0, pending: 1, failed: 2, done: 3 };

	let sortedJobs = $derived(
		typedJobs
			.filter((j) => inFilter(j, filter))
			.slice()
			.sort((a, b) => {
				if (sort === 'status') return statusOrder[a.status] - statusOrder[b.status];
				const diff = new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime();
				return sort === 'oldest' ? diff : -diff;
			})
	);

	let totalPages = $derived(Math.max(1, Math.ceil(sortedJobs.length / PAGE_SIZE)));
	let pageJobs = $derived(sortedJobs.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE));
	// The rows are one Tab stop (the open row, else the first): Tab moves past
	// the list, the arrows, Home and End move inside it.
	let focusRowId = $derived(pageJobs.some((j) => j.id === selectedId) ? selectedId : (pageJobs[0]?.id ?? ''));

	$effect(() => {
		// Narrowing the result set can strand currentPage past the last page.
		void search;
		void filter;
		void typeFilter;
		currentPage = 1;
	});

	function pickType(t: MediaType) {
		typeFilter = typeFilter === t ? 'all' : t;
	}

	function subLine(j: Job): string {
		if (j.fileName) return j.sizeBytes ? `${j.fileName} · ${formatBytes(j.sizeBytes)}` : j.fileName;
		return `${i18n.t(`media.${j.mediaType}`)} · ${i18n.t(`dashboard.watermarkKind.${j.watermarkKind}`)}`;
	}

	function stateTitle(j: Job): string | undefined {
		if (j.paid) return i18n.t('signal.paidTitle');
		if (j.linksDisabled) return i18n.t('signal.linksOffTitle');
		if (j.openedAt) return i18n.t('signal.openedTitle', { date: fullDate(j.openedAt, i18n.locale) });
		if (j.status === 'done') return i18n.t('signal.notOpenedTitle');
		return undefined;
	}
</script>

<svelte:head>
	<title>{IS_APP && selectedJob ? `${selectedJob.fileName ?? selectedJob.recipientEmail} · ` : ''}{i18n.t('dashboard.title')} · SADE</title>
</svelte:head>

<svelte:window onkeydown={onKey} onpointerdown={(e) => ctx && !ctxEl?.contains(e.target as Node) && closeCtx()} onblur={() => closeCtx()} onresize={() => closeCtx()} />

{#if ctx}
	<div class="ctx" role="menu" tabindex="-1" bind:this={ctxEl} style:left="{ctx.x}px" style:top="{ctx.y}px" onkeydown={onCtxKey}>
		<button type="button" role="menuitem" onclick={ctxOpen}>{i18n.t('dashboard.ctxOpen')}</button>
		<button type="button" role="menuitem" onclick={ctxCopy}>{i18n.t('dashboard.ctxCopy')}</button>
				<button type="button" role="menuitem" onclick={ctxPage}>{i18n.t('dashboard.ctxPage')}</button>
		{#if ctxJob && ctxJob.status === 'done'}
			<hr />
			{#if !ctxJob.linksDisabled}
				<button type="button" role="menuitem" onclick={ctxResend}>{i18n.t('dashboard.ctxResend')}</button>
			{/if}
			{#if ctxJob.linksDisabled}
				<button type="button" role="menuitem" onclick={() => ctxLinks(true)}>{i18n.t('dashboard.ctxLinksOn')}</button>
			{:else if !ctxJob.paid}
				<button type="button" role="menuitem" class="danger" onclick={() => ctxLinks(false)}>
					{ctxArmed ? i18n.t('dashboard.ctxLinksOffConfirm') : i18n.t('dashboard.ctxLinksOff')}
				</button>
			{/if}
		{/if}
	</div>
{/if}

{#snippet uploadForm()}
	<form class="upload-form" onsubmit={submit}>
		<div class="field">
			<label for="file-pick" class="sr-only">{i18n.t('dashboard.fileLabel')}</label>
			{#if file}
				<div class="file-card" class:has-error={!!fileError}>
					<span class="file-icon">
						{#if fileKind}<MediaIcon type={fileKind} size={18} />{/if}
					</span>
					<span class="file-meta">
						<span class="file-name" title={file.name}>{file.name}</span>
						<span class="muted mono file-size">
							{formatBytes(file.size)}{fileError ? ` · ${fileError}` : ''}
						</span>
					</span>
					{#if !submitting}
						<button type="button" class="link-btn" onclick={() => fileInput?.click()}>
							{i18n.t('dashboard.fileChange')}
						</button>
						<button type="button" class="icon-btn" aria-label={i18n.t('dashboard.fileRemove')} onclick={clearFile}>
							<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M6 6 L18 18 M18 6 L6 18" /></svg>
						</button>
					{/if}
				</div>
			{:else}
				<button
					id="file-pick"
					type="button"
					class="dropzone"
					class:dragging
					onclick={() => fileInput?.click()}
					ondragenter={onDragEnter}
					ondragleave={onDragLeave}
					ondragover={onDragOver}
					ondrop={onDrop}
				>
					<span class="drop-icon" aria-hidden="true">
						<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 15 V4 M8 8 L12 4 L16 8" /><path d="M5 15 v4 h14 v-4" /></svg>
					</span>
					{#if dragging}
						<span class="drop-text">{i18n.t('dashboard.dropRelease')}</span>
					{:else}
						<span class="drop-text">
							{i18n.t('dashboard.dropText')}
							<span class="accent">{i18n.t('dashboard.dropBrowse')}</span>
						</span>
					{/if}
					<span class="muted types">{i18n.t('dashboard.allowedTypes')}</span>
				</button>
			{/if}
			<input type="file" accept="video/*,audio/*,image/*" bind:this={fileInput} onchange={onFileChange} style="display:none" />
			{#if fileError}
				<span class="error-text small">{i18n.t('dashboard.fileChooseSupported')}</span>
			{/if}
		</div>

		<div class="field">
			<label for="recipient">{i18n.t('dashboard.recipientLabel')}</label>
			<input
				id="recipient"
				type="email"
				required
				autocomplete="email"
				placeholder={i18n.t('dashboard.recipientPlaceholder')}
				bind:value={recipientEmail}
				bind:this={recipientInput}
				oninput={() => {
					recipientTouched = false;
					typoBlocked = false;
				}}
				onblur={() => (recipientTouched = true)}
				aria-describedby={showSuggestion ? 'typo-note' : undefined}
				disabled={submitting}
			/>
			{#if showSuggestion && emailSuggestion}
				<div class="typo" class:blocked={typoBlocked} id="typo-note" role={typoBlocked ? 'alert' : 'status'}>
					<span>
						{#if typoBlocked}{i18n.t('dashboard.typoBlocked')}{/if}
						{i18n.t('dashboard.typoSuggest', { email: emailSuggestion })}
					</span>
					<span class="typo-actions">
						<button type="button" class="link-btn" onclick={useSuggestion}>{i18n.t('dashboard.typoUse')}</button>
						<button type="button" class="link-btn quiet" onclick={keepAddress}>{i18n.t('dashboard.typoKeep')}</button>
					</span>
				</div>
			{/if}
		</div>

		<div class="field">
			<span class="label" id="kind-label">{i18n.t('dashboard.watermarkLabel')}</span>
			<div class="segmented" class:unknown={logoState === 'unknown'} role="group" aria-labelledby="kind-label" aria-busy={logoState === 'unknown'}>
				{#each KINDS as k (k)}
					<button
						type="button"
						aria-pressed={logoState === 'unknown' ? false : kind === k}
						onclick={() => pickKind(k)}
						disabled={submitting || logoState === 'unknown' || (!hasLogo && k !== 'text')}
					>
						{i18n.t(`dashboard.watermarkKind.${k}`)}
					</button>
				{/each}
			</div>
			{#if logoState === 'none' && !logoPanel}
				<span class="muted small">
					{i18n.t('dashboard.logoNeeded')}
					<button type="button" class="inline-link" onclick={() => (logoPanel = true)}>{i18n.t('dashboard.logoAdd')}</button>
				</span>
			{:else if logoState === 'error'}
				<span class="muted small" role="alert">
					{i18n.t('dashboard.brandLoadError')}
					<button type="button" class="inline-link" onclick={() => brand.load()}>{i18n.t('common.retry')}</button>
				</span>
			{/if}
			{#if logoPanel && brand.value}
				<div class="logo-panel">
					<LogoPicker brand={brand.value} onchange={(b) => brand.set(b)} />
					<button type="button" class="inline-link" onclick={() => (logoPanel = false)}>{i18n.t('dashboard.logoClose')}</button>
				</div>
			{/if}
		</div>

		{#if kind !== 'logo'}
			<div class="field">
				<label for="text">{i18n.t('dashboard.watermarkTextLabel')} <span class="optional">{i18n.t('dashboard.optional')}</span></label>
				<input id="text" type="text" maxlength="200" placeholder={defaultText} bind:value={watermarkText} disabled={submitting} aria-describedby="text-hint" />
				<span class="muted small" id="text-hint">
					{#if recipientAppended}
						{i18n.t('dashboard.recipientAppended')} <span class="mono">· {recipientShown}</span>
					{:else}
						{i18n.t('dashboard.watermarkHint')}
					{/if}
				</span>
			</div>
		{/if}

		{#if uploadError}
			<p class="error-banner" role="alert">{uploadError}</p>
		{/if}

		{#if submitting}
			<div class="progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.round(progress * 100)}>
				<div class="progress-head">
					<span>{i18n.t('dashboard.submitting')}</span>
					<span class="mono">{Math.round(progress * 100)}%</span>
				</div>
				<div class="track"><div class="fill" style="width:{progress * 100}%"></div></div>
				<button type="button" class="btn btn-ghost btn-block" onclick={() => uploadAbort?.abort()}>
					{i18n.t('dashboard.cancelUpload')}
				</button>
			</div>
		{:else}
			{#if file && !fileError && recipientLooksValid}
				<dl class="summary">
					<div>
						<dt>{i18n.t('dashboard.summaryTo')}</dt>
						<dd class="mono">{recipientEmail.trim()}</dd>
					</div>
					<div>
						<dt>{i18n.t('dashboard.summaryText')}</dt>
						<dd>{burnedText}</dd>
					</div>
				</dl>
			{/if}
			{#if !file || fileError}
				<p class="muted small submit-why" id="submit-why">{i18n.t(file ? 'dashboard.submitNeedsValidFile' : 'dashboard.submitNeedsFile')}</p>
			{/if}
			<button class="btn btn-block" type="submit" disabled={!file || !!fileError} aria-describedby={!file || fileError ? 'submit-why' : undefined}>
				{i18n.t('dashboard.submit')}
			</button>
		{/if}
	</form>
	{#if usage && !usage.unlimited}
		<div class="usage-foot">
			<UsageMeter {usage} />
		</div>
	{/if}
{/snippet}

<div class="container page" class:wide-page={wide} class:app-page={IS_APP}>
	{#if !IS_APP}
		<div class="page-head" class:compact={wide}>
			<div>
				<h1>{i18n.t('dashboard.title')}</h1>
				<p class="lead">{i18n.t('dashboard.subtitle')}</p>
			</div>
		</div>
	{/if}
	

		{#if IS_APP}<h1 class="sr-only">{i18n.t('dashboard.title')}</h1>{/if}
	<div class="layout" class:wide class:list-first={jobs.length > 0}>
		<!-- the delivery list: the marketing page's window -->
				<section class="win deliveries" aria-labelledby="list-title">
			<div class="win-bar" data-tauri-drag-region>
				<WindowControls />
				<span class="crumb" id="list-title" data-tauri-drag-region>
					<img src={logo} alt="" width="16" height="16" />
										<span class="crumb-root">{i18n.t('dashboard.title')} <i aria-hidden="true">/</i></span>
					<b>{overlay && paneOpen ? (showUpload ? i18n.t('dashboard.newJob') : (selectedJob?.fileName ?? selectedJob?.recipientEmail ?? '')) : typeFilter === 'all' ? i18n.t(`dashboard.filter.${filter}`) : i18n.t(`dashboard.type.${typeFilter}`)}</b>
				</span>
								<span class="sp" data-tauri-drag-region></span>
				{#if loadError && jobs.length > 0}
					<span class="offline" role="status">
						<i aria-hidden="true"></i>
						{i18n.t('dashboard.offline', { time: lastLoadedText })}
						<button type="button" class="link-u" onclick={retryLoad} disabled={retrying}>{i18n.t('common.retry')}</button>
					</span>
				{/if}
				{#if jobs.length > 0}
					<label class="search">
						<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" aria-hidden="true"><circle cx="7" cy="7" r="4.5" /><path d="m10.5 10.5 3 3" /></svg>
						<span class="sr-only">{i18n.t('dashboard.searchShort')}</span>
						<input type="search" placeholder={wide && !IS_APP ? i18n.t('dashboard.searchShort') : i18n.t('dashboard.searchTiny')} bind:value={search} bind:this={searchInput} aria-keyshortcuts={IS_APP ? (desktopOS() === 'mac' ? 'Meta+F' : 'Control+F') : '/'} />
						<kbd aria-hidden="true">{IS_APP ? `${MOD}F` : '/'}</kbd>
					</label>
					<label class="sort">
						<span class="sr-only">{i18n.t('dashboard.sortLabel')}</span>
						<select bind:value={sort}>
							<option value="newest">{i18n.t('dashboard.sort.newest')}</option>
							<option value="oldest">{i18n.t('dashboard.sort.oldest')}</option>
														<option value="status">{i18n.t('dashboard.sort.status')}</option>
						</select>
					</label>
				{/if}
								{#if IS_APP}
					<AccountMenu compact alwaysNav />
				{/if}
				<WindowControls side="right" />
			</div>

									<div class="win-main" class:pane-open={paneOpen} class:overlay>
								<div class="list-pane" inert={overlay && paneOpen}>
					<div class="list-grid">
										{#if jobs.length > 0 || IS_APP}
						<nav class="side" aria-label={i18n.t('dashboard.filtersLabel')}>
							{#if IS_APP}
								<button type="button" class="btn btn-sm new-btn" onclick={openUpload} title={`${MOD}N`} aria-keyshortcuts={desktopOS() === 'mac' ? 'Meta+N' : 'Control+N'}>
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" aria-hidden="true"><path d="M12 5 V19 M5 12 H19" /></svg>
									{i18n.t('dashboard.jumpUpload')}
								</button>
							{/if}
							{#each visibleFilters as f (f)}
								<button type="button" class="side-item" aria-pressed={filter === f} onclick={() => (filter = f)}>
									{#if f === 'all'}
										<svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" aria-hidden="true"><path d="M2.5 9h3l1 1.5h3l1-1.5h3" /><path d="M2.5 9 4 3.5h8L13.5 9v3.5h-11z" /></svg>
									{:else if f === 'working'}
										<svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" aria-hidden="true"><circle cx="8" cy="8" r="5.5" /><path d="M8 5v3l2 1.5" /></svg>
									{:else if f === 'opened'}
										<svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true"><circle cx="8" cy="8" r="2.5" /><path d="M1.5 8s2.4-4.5 6.5-4.5S14.5 8 14.5 8s-2.4 4.5-6.5 4.5S1.5 8 1.5 8z" /></svg>
									{:else if f === 'paid'}
										<svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m3.5 8.4 2.8 2.8 6.2-6.4" /></svg>
									{:else if f === 'off'}
										<svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true"><circle cx="8" cy="8" r="5.5" /><path d="M4.2 11.8l7.6-7.6" /></svg>
									{:else}
										<svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" aria-hidden="true"><path d="M8 2.5 14 13H2z" /><path d="M8 6.5v3M8 11.2v.1" /></svg>
									{/if}
									<span>{i18n.t(`dashboard.filter.${f}`)}</span>
									<span class="count">{counts[f]}</span>
								</button>
							{/each}
							{#if visibleTypes.length > 1 || typeFilter !== 'all'}
								<span class="side-label">{i18n.t('dashboard.type')}</span>
								{#each visibleTypes as t (t)}
									<button type="button" class="side-item" aria-pressed={typeFilter === t} onclick={() => pickType(t)}>
										<span class="hash" aria-hidden="true">#</span>
										<span>{i18n.t(`dashboard.type.${t}`)}</span>
										<span class="count">{typeCounts[t]}</span>
									</button>
																{/each}
							{/if}
							{#if IS_APP}
								<a class="side-item side-foot" href="/app/settings" title={`${MOD},`}>
									<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" /></svg>
									<span>{i18n.t('nav.settings')}</span>
								</a>
							{/if}
						</nav>
					{/if}

					<div class="list">
						{#if jobsLoading}
							<div class="sk-list" aria-hidden="true">
								{#each [0, 1, 2, 3] as i (i)}<span class="sk sk-line"></span>{/each}
							</div>
						{:else if loadError && jobs.length === 0}
							<div class="empty load-error" role="alert">
								<h2>{i18n.t('dashboard.loadErrorTitle')}</h2>
								<p class="muted">{i18n.t('dashboard.loadErrorBody')}</p>
								<button type="button" class="btn btn-ink btn-sm" onclick={retryLoad} disabled={retrying}>
									{retrying ? i18n.t('dashboard.retrying') : i18n.t('common.retry')}
								</button>
							</div>
						{:else if jobs.length === 0}
							<div class="empty">
								<span class="empty-files" aria-hidden="true">
									<span><MediaIcon type="video" size={18} /></span>
									<span><MediaIcon type="image" size={18} /></span>
									<span><MediaIcon type="audio" size={18} /></span>
								</span>
								<h2>{i18n.t('dashboard.emptyTitle')}</h2>
								<p class="muted">{i18n.t('dashboard.emptyBody')}</p>
							</div>
						{:else}
							<div class="cols" aria-hidden="true">
								<span>{i18n.t('dashboard.col.client')}</span><span>{i18n.t('dashboard.col.file')}</span><span>{i18n.t('dashboard.col.status')}</span><span>{i18n.t('dashboard.col.when')}</span>
							</div>
							{#if sortedJobs.length === 0}
								<div class="no-matches">
									<p class="muted">{i18n.t('dashboard.noMatches')}</p>
									<button
										type="button"
										class="btn btn-line btn-sm"
										onclick={() => {
											search = '';
											filter = 'all';
											typeFilter = 'all';
										}}>{i18n.t('dashboard.clearFilters')}</button
									>
								</div>
							{:else}
																<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
								<ul class="rows" bind:this={rowsEl} onkeydown={onRowsKey} aria-keyshortcuts="ArrowUp ArrowDown Home End" aria-label={i18n.t('dashboard.listLabel')}>
									{#each pageJobs as job (job.id)}
										{@const st = deliveryState(job)}
										{@const sel = wide && !showUpload && job.id === selectedId}
										<li>
											<a class="row" class:is-sel={sel} class:is-ctx={ctx?.id === job.id} data-id={job.id} href={`/app/jobs/${job.id}`} onclick={(e) => select(e, job.id)} oncontextmenu={(e) => openCtx(e, job.id)} onkeydown={(e) => onRowKey(e, job.id)} tabindex={wide ? (job.id === focusRowId ? 0 : -1) : undefined} aria-current={sel ? 'true' : undefined} aria-haspopup={IS_APP ? 'menu' : undefined}>
												<span class="avatar" data-tone={tone(job.recipientEmail)} aria-hidden="true">{initials(job.recipientEmail)}</span>
												<span class="who">{job.recipientEmail}</span>
												<span class="sub">{subLine(job)}</span>
												<span class="state"><span class={stateClass(st)} title={stateTitle(job)}>{i18n.t(stateKey(st))}</span></span>
												<span class="age mono" title={fullDate(job.createdAt, i18n.locale)}>{relativeTime(job.createdAt, i18n.locale)}</span>
											</a>
										</li>
									{/each}
								</ul>
							{/if}

							{#if totalPages > 1}
								<div class="pagination">
									<span class="muted mono">
										{(currentPage - 1) * PAGE_SIZE + 1}–{Math.min(currentPage * PAGE_SIZE, sortedJobs.length)}
										{i18n.t('dashboard.of')}
										{sortedJobs.length}
									</span>
									<div class="pagination-controls">
										<button type="button" class="btn btn-line btn-sm" disabled={currentPage <= 1} onclick={() => currentPage--}>
											{i18n.t('dashboard.prev')}
										</button>
										<span class="muted mono">{currentPage} / {totalPages}</span>
										<button type="button" class="btn btn-line btn-sm" disabled={currentPage >= totalPages} onclick={() => currentPage++}>
											{i18n.t('dashboard.next')}
										</button>
									</div>
								</div>
							{/if}
						{/if}
					</div>
					</div>
				</div>

								{#if wide && paneOpen}
					<div class="open-pane">
						{#if showUpload}
							<section class="upload-pane" bind:this={uploadCard} aria-labelledby="new-title">
								<div class="pane-head">
									<h2 id="new-title">{i18n.t('dashboard.newJob')}</h2>
																		{#if jobs.length > 0}
										<button type="button" class="close-pane" aria-label={i18n.t('common.closePane')} title={i18n.t('common.closePane')} onclick={closeUpload}>
											<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" aria-hidden="true"><path d="M6 6 L18 18 M18 6 L6 18" /></svg>
										</button>
									{/if}
								</div>
								{@render uploadForm()}
							</section>
						{:else if selectedJob}
														<JobPanel jobId={selectedJob.id} status={selectedJob.status} onclose={IS_APP ? closePane : undefined} />
						{:else if jobsLoading}
							<div class="pane-loading" aria-hidden="true">
								<span class="sk" style="height:1.4rem;width:55%"></span>
								<span class="sk" style="height:280px"></span>
							</div>
						{/if}
					</div>
				{/if}
			</div>
		</section>

				

		<!-- narrow screens: the form under the list -->
		{#if !wide}
			<section class="win new-job" bind:this={uploadCard} aria-labelledby="new-title-n">
				<div class="win-bar">
					<b id="new-title-n">{i18n.t('dashboard.newJob')}</b>
				</div>
								{@render uploadForm()}
			</section>
		{/if}

				<!-- secondary nudges (passkey, brand) after the work, on every screen -->
		<div class="nudges" class:hidden={IS_APP && wide}>
			<PasskeyBanner />
			<BrandBanner />
		</div>
	</div>
</div>

<style>
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		margin: -1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}

	.layout {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 380px;
		gap: 24px;
		align-items: start;
	}

	.layout.wide {
		grid-template-columns: minmax(0, 1fr);
	}

	@media (max-width: 1023px) {
		.layout {
			grid-template-columns: minmax(0, 1fr);
		}
		/* An account with no files sees the form first. */
		.layout:not(.list-first) .deliveries {
			order: 1;
		}
	}

	@media (min-width: 1024px) {
		.new-job {
			position: sticky;
			top: 96px;
		}
	}

	/* ---- the window --------------------------------------------------- */
	.crumb {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
	}

	.crumb img {
		flex: none;
	}

	.crumb i {
		font-style: normal;
	}

	.crumb b {
		overflow: hidden;
		text-overflow: ellipsis;
	}

	/* The server stopped answering: the list stays, saying how old it is. */
	.offline {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		flex: none;
		font-size: 0.82rem;
		color: var(--text-muted);
		white-space: nowrap;
	}

	.offline i {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--warning);
	}

	.offline button {
		appearance: none;
		border: none;
		background: none;
		padding: 0;
		font: inherit;
		color: var(--text);
		text-decoration: underline;
		text-underline-offset: 3px;
		cursor: pointer;
	}

	.submit-why {
		margin: 0;
		font-size: 0.85rem;
	}

	.load-error .btn {
		margin-top: 6px;
	}

	.search {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: 8px;
		flex: 0 1 280px;
		min-width: 0;
		height: 32px;
		margin: 0;
		padding: 0 4px 0 10px;
		border-radius: 7px;
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line-2);
		color: var(--text-muted);
		font-weight: 400;
		cursor: text;
	}

	/* Drawn 32px inside the 44px bar, tapped 44px. */
	.search::after {
		content: '';
		position: absolute;
		inset: -6px 0;
	}

	.search:focus-within {
		box-shadow:
			inset 0 0 0 1px var(--accent-bright),
			0 0 0 3px color-mix(in oklab, var(--accent-bright) 18%, transparent);
	}

	/* Above the hit strip, so a click inside the text still places the caret. */
	.search input {
		position: relative;
		z-index: 1;
		flex: 1;
		min-width: 0;
		min-height: 30px;
		padding: 0;
		border: none;
		background: none;
		box-shadow: none;
		font-size: 0.85rem;
	}

	.search input:focus,
	.search input:focus-visible {
		outline: none;
		box-shadow: none;
	}

	kbd {
		display: inline-grid;
		place-items: center;
		min-width: 20px;
		height: 20px;
		padding: 0 4px;
		border-radius: 5px;
		background: var(--fill-2);
		font: 500 0.75rem var(--font-sans);
		color: var(--text-muted);
	}

	.search:focus-within kbd {
		display: none;
	}

	.sort {
		margin: 0;
		flex: none;
	}

	.deliveries {
		container: win / inline-size;
	}

		@container win (max-width: 680px) {
		.sort {
			display: none;
		}
	}

	.crumb-root {
		display: inline-flex;
		align-items: center;
		gap: 8px;
	}

	/* A phone keeps the filter's name in the bar, not the "Livrări /" root. */
	@container win (max-width: 480px) {
		.crumb-root {
			display: none;
		}
		.crumb {
			flex: none;
		}
		.search {
			flex: 1 1 auto;
		}
	}


	.sort select {
		width: auto;
		min-height: 32px;
		padding: 0 28px 0 10px;
		border-radius: 7px;
		font-size: 0.85rem;
		font-weight: 500;
		color: var(--text);
	}

	:global(html.is-app) .sort select {
		padding: 0 10px;
		min-height: 32px;
		border-radius: 7px;
		font-size: 0.85rem;
		font-weight: 500;
		color: var(--text);
	}

	@media (max-width: 559px) {
		.sort,
		kbd {
			display: none;
		}
	}

	/* Wide: one window, the list and the open delivery side by side, each
	   scrolling on its own, as the marketing page's picture. */
	.win-main {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
	}

		/* Wide: the head shrinks to one line and the window fills what is left
	   of the screen, so only its panes scroll. */
	.wide-page {
		padding-top: 24px;
		padding-bottom: 32px;
	}

	.page-head.compact {
		display: flex;
		align-items: baseline;
		margin-bottom: 18px;
	}

	.page-head.compact > div {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 4px 20px;
	}

	.page-head.compact h1 {
		font-size: 2rem;
		letter-spacing: -0.03em;
	}

	.page-head.compact .lead {
		margin: 0;
		font-size: 0.98rem;
	}

	.wide .deliveries {
		display: flex;
		flex-direction: column;
		/* 72 header + 24 top + ~40 head + 18 gap + 24 air */
		height: max(560px, calc(100svh - 178px));
	}

		.nudges {
		display: flex;
		flex-direction: column;
		order: 2;
	}

	.nudges :global(.banner) {
		margin-bottom: 0;
		margin-top: 0;
	}

	.nudges:empty {
		display: none;
	}

		.wide .win-main {
		flex: 1;
		min-height: 0;
		grid-template-columns: minmax(0, 1fr);
	}

		.wide .win-main.pane-open {
		grid-template-columns: minmax(0, 5fr) minmax(0, 7fr);
	}

	/* A narrower app window: the pane slides over the list instead. */
	.win-main {
		position: relative;
	}

	.wide .win-main.pane-open.overlay {
		grid-template-columns: minmax(0, 1fr);
	}

	.overlay .open-pane {
		position: absolute;
		inset: 0;
		z-index: 5;
		background: var(--surface);
		border-left: none;
		animation: pane-in 0.18s var(--ease);
	}

	@keyframes pane-in {
		from {
			opacity: 0;
			transform: translateX(16px);
		}
	}

	/* ---- the installed app: the list window is the whole window -------- */
	.app-page {
		max-width: none;
		padding: 0;
	}

	.app-page .layout.wide .deliveries {
		height: 100vh;
		border-radius: 0;
		box-shadow: none;
	}

	.app-page .layout.wide {
		gap: 0;
	}

		.app-page .win-bar {
		user-select: none;
		padding-right: 8px;
		--bar-pad-right: 8px;
	}

	.nudges.hidden {
		display: none;
	}

	/* A narrow app window: the list and the form stacked, with breathing room. */
	.app-page .layout:not(.wide) {
		padding: 10px;
		gap: 10px;
	}

	.new-btn {
		justify-content: flex-start;
		width: 100%;
		margin-bottom: 10px;
		padding-left: 12px;
	}

	.side-foot {
		margin-top: auto;
	}

	

		.wide .open-pane {
		min-height: 0;
		overflow-y: auto;
		overscroll-behavior: none;
	}

	/* The sidebar ("Fișier nou", filters, Setări) stays put; only the rows
	   scroll beside it. */
	.wide .list-pane {
		min-height: 0;
		overflow: hidden;
	}

	.wide .list {
		min-height: 0;
		overflow-y: auto;
		overscroll-behavior: none;
	}

	.wide .side {
		overflow-y: auto;
	}

	.open-pane {
		border-left: 1px solid var(--line);
	}

	.pane-loading {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 22px;
	}

	.upload-pane {
		display: flex;
		flex-direction: column;
	}

	.pane-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 20px 22px 0;
	}

	.pane-head h2 {
		margin: 0;
		font-size: 1.25rem;
		font-weight: 500;
		letter-spacing: -0.02em;
	}

		/* The form uses the pane: the drop zone across, address and watermark
	   side by side once there is room. */
	.upload-pane {
		container: upload / inline-size;
	}

	@container upload (min-width: 640px) {
		.upload-pane .upload-form {
			display: grid;
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
			column-gap: 20px;
		}
		.upload-pane .upload-form > :global(*) {
			grid-column: 1 / -1;
		}
		.upload-pane .upload-form > :global(.field:nth-child(2)) {
			grid-column: 1;
		}
		.upload-pane .upload-form > :global(.field:nth-child(3)) {
			grid-column: 2;
		}
		.upload-pane .upload-form > :global(.btn) {
			justify-self: end;
			width: auto;
			min-width: 240px;
		}
	}

	.close-pane {
		appearance: none;
		position: relative;
		width: 36px;
		height: 36px;
		display: grid;
		place-items: center;
		border: none;
		border-radius: 50%;
		background: transparent;
		color: var(--text-muted);
		cursor: pointer;
	}

	.close-pane::after {
		content: '';
		position: absolute;
		inset: -4px;
	}

	.close-pane:hover {
		background: var(--fill-2);
		color: var(--text);
	}

	/* The list pane: filters beside the rows; it narrows by its own width. */
	/* The pane is the size container; the grid inside it reacts to it. */
	.list-pane {
		container: list / inline-size;
	}

	.list-grid {
		display: grid;
		grid-template-columns: 172px minmax(0, 1fr);
		align-content: start;
	}

	.list-grid:has(> .list:only-child) {
		grid-template-columns: minmax(0, 1fr);
	}

		.wide .list-grid {
		grid-template-columns: 160px minmax(0, 1fr);
		grid-template-rows: minmax(0, 1fr);
		height: 100%;
	}

	.side {
		display: flex;
		flex-direction: column;
		gap: 1px;
		padding: 10px 8px;
		border-right: 1px solid var(--line);
		align-self: stretch;
	}

	.hash {
		width: 15px;
		text-align: center;
		color: var(--text-faint);
	}

	.list {
		min-width: 0;
		display: flex;
		flex-direction: column;
	}

		/* Columns sized to what they hold, so the state and the time sit next to
	   the file instead of at the far edge of a wide window. */
	.cols,
	.row {
		display: grid;
		grid-template-columns: 28px minmax(140px, 280px) minmax(0, 460px) 120px 64px;
		justify-content: start;
		align-items: center;
		gap: 0 14px;
		padding: 0 18px;
	}

	.cols {
		height: 38px;
		border-bottom: 1px solid var(--line);
				font-size: 0.82rem;
		color: var(--text-muted);
	}

	.cols span:first-child {
		grid-column: 1 / 3;
	}

	.cols span:last-child {
		text-align: right;
	}

	.row {
		min-height: 56px;
		font-size: 0.9rem;
		color: var(--text);
		transition: background 0.18s var(--ease);
	}

	.row:hover {
		color: var(--text);
		background: color-mix(in oklab, var(--text) 3%, var(--surface));
	}

	/* The open row, as the mockup marks it. */
	.row.is-sel,
	.row.is-ctx {
		background: color-mix(in oklab, var(--text) 6%, var(--surface));
	}

	/* On the dark surface 6% is near nothing: the selection needs more. */
	:global(:root[data-theme='dark']) .row.is-sel {
		background: color-mix(in oklab, var(--text) 13%, var(--surface));
	}

	@media (prefers-color-scheme: dark) {
		:global(:root:not([data-theme='light'])) .row.is-sel {
			background: color-mix(in oklab, var(--text) 13%, var(--surface));
		}
	}

	/* The row's own menu, drawn like the account menu. */
	.ctx {
		position: fixed;
		z-index: 60;
		min-width: 240px;
		padding: 6px;
		display: flex;
		flex-direction: column;
		border: 1px solid var(--line);
		border-radius: 10px;
		background: var(--surface);
		box-shadow: 0 10px 30px -8px rgb(0 0 0 / 0.22), 0 2px 6px rgb(0 0 0 / 0.08);
		animation: ctx-in 0.12s var(--ease);
	}

	@keyframes ctx-in {
		from {
			opacity: 0;
			transform: translateY(-3px);
		}
	}

	.ctx button {
		appearance: none;
		min-height: 34px;
		padding: 0 10px;
		border: none;
		border-radius: 6px;
		background: none;
		font: inherit;
		font-size: 0.9rem;
		color: var(--text);
		text-align: left;
		cursor: pointer;
	}

		.ctx button:hover,
	.ctx button:focus-visible {
		background: var(--fill);
		outline: none;
	}

	.ctx button.danger {
		color: var(--danger);
	}

	.ctx hr {
		margin: 5px 4px;
		border: none;
		border-top: 1px solid var(--line);
	}

	.row:focus-visible {
		outline-offset: -2px;
	}

	.rows > li:last-child .row {
		border-bottom: 1px solid var(--line);
	}

	.who {
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.sub {
		color: var(--text-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.age {
		color: var(--text-muted);
		text-align: right;
		font-size: 0.85rem;
	}

	/* Narrow list: rows stack name over file. */
	@container list (max-width: 720px) {
		.cols {
			display: none;
		}
		.row {
			grid-template-columns: 32px minmax(0, 1fr) auto;
			grid-template-rows: auto auto;
			row-gap: 2px;
			padding: 10px 14px;
			min-height: 64px;
		}
		.row .avatar {
			grid-row: 1 / 3;
			width: 32px;
			height: 32px;
		}
		.who {
			grid-column: 2;
			grid-row: 1;
		}
		.sub {
			grid-column: 2;
			grid-row: 2;
			font-size: 0.85rem;
		}
				.state {
			grid-column: 3;
			grid-row: 1;
			justify-self: end;
		}
		/* The time stays: comparing deliveries is when it matters. */
		.age {
			grid-column: 3;
			grid-row: 2;
			font-size: 0.8rem;
		}
	}

	/* Very narrow: the filters become a row of chips above the list. */
	@container list (max-width: 479px) {
				.list-grid,
		.wide .list-grid {
			grid-template-columns: minmax(0, 1fr);
			grid-template-rows: auto minmax(0, 1fr);
			align-content: start;
		}
		.side {
			flex-direction: row;
			flex-wrap: nowrap;
			align-items: center;
			overflow-x: auto;
			gap: 4px;
			padding: 8px 10px;
			border-right: none;
			border-bottom: 1px solid var(--line);
			scrollbar-width: none;
		}
		/* A chip row that scrolls sideways clips anything drawn outside a
		   chip, so the chip itself is the full 44px target. */
		.side .side-item {
			width: auto;
			flex: none;
			min-height: 44px;
		}
		.side .side-item .count {
			margin-left: 2px;
		}
		.side > .side-label {
			display: none;
		}
	}

	.no-matches {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 12px;
		padding: 40px 16px;
		text-align: center;
	}

	.no-matches p {
		margin: 0;
	}

	.sk-list {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 18px;
	}

	.sk-line {
		height: 44px;
	}

	.empty {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 10px;
		padding: 56px 24px;
		text-align: center;
	}

	.empty h2 {
		margin: 8px 0 0;
		font-size: 1.35rem;
	}

	.empty p {
		margin: 0;
		max-width: 38ch;
	}

	/* The three file tiles of the marketing page's "Lucrează cu video, audio și imagini". */
	.empty-files {
		display: flex;
		gap: 10px;
	}

	.empty-files span {
		width: 52px;
		height: 52px;
		display: grid;
		place-items: center;
		border-radius: 12px;
		background: var(--surface);
		color: var(--accent-ink);
		box-shadow: var(--shadow-card);
	}

	.empty-files span:nth-child(2) {
		transform: translateY(-6px);
	}

	.pagination {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 12px 18px;
		flex-wrap: wrap;
		font-size: 0.85rem;
	}

	.pagination-controls {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	/* ---- the upload form ---------------------------------------------- */
	.upload-form {
		display: flex;
		flex-direction: column;
		padding: 20px;
	}

	.upload-pane .upload-form {
		padding: 18px 22px 22px;
	}

	.new-job .win-bar {
		font-size: 0.9rem;
	}

	.usage-foot {
		padding: 16px 20px 18px;
		border-top: 1px solid var(--line);
		background: var(--fill);
	}

	.upload-pane .usage-foot {
		margin-top: auto;
		padding: 16px 22px 18px;
	}

	.label {
		display: block;
		font-size: 0.85rem;
		font-weight: 500;
		margin-bottom: 0.4rem;
		color: var(--text-muted);
	}

	.optional {
		font-weight: 400;
		color: var(--text-muted);
	}

	.segmented.unknown button {
		color: var(--text-muted);
	}

	.drop-icon {
		width: 36px;
		height: 36px;
		display: grid;
		place-items: center;
		border-radius: 50%;
		background: var(--surface);
		color: var(--text-muted);
		box-shadow: var(--shadow-sm);
		margin-bottom: 2px;
	}

	.dropzone.dragging .drop-icon,
	.dropzone:hover .drop-icon {
		color: var(--accent-ink);
	}

	.drop-text {
		font-size: 0.92rem;
	}

	.dropzone .types {
		font-size: 0.78rem;
	}

	.inline-link {
		appearance: none;
		background: none;
		border: none;
		padding: 0;
		font: inherit;
		font-weight: 500;
		color: var(--accent-ink);
		text-decoration: underline;
		text-underline-offset: 3px;
		cursor: pointer;
		position: relative;
	}

	/* Inline in a sentence, so the visible text stays small; the tap area does not. */
	.inline-link::after {
		content: '';
		position: absolute;
		inset: -12px -6px;
	}

	.typo {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		margin-top: 0.45rem;
		padding: 0.55rem 0.7rem 0.2rem;
		border-radius: var(--radius-sm);
		background: var(--warning-tint);
		font-size: 0.85rem;
		line-height: 1.45;
		color: var(--text);
	}

	.typo.blocked {
		background: var(--danger-tint);
	}

	.typo-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0 0.5rem;
		margin-left: -0.6rem;
	}

	.link-btn.quiet {
		color: var(--text-muted);
	}

	/* What goes where, as hairline rows (the marketing page's template table). */
	.summary {
		margin: 0 0 14px;
		font-size: 0.85rem;
		line-height: 1.4;
	}

	.summary div {
		display: grid;
		grid-template-columns: 92px minmax(0, 1fr);
		gap: 10px;
		padding: 9px 0;
		border-top: 1px solid var(--line);
	}

	.summary div:last-child {
		border-bottom: 1px solid var(--line);
	}

	.summary dt {
		color: var(--text-muted);
	}

	.summary dd {
		margin: 0;
		overflow-wrap: anywhere;
	}

	.summary div > dd:only-child {
		grid-column: 1 / -1;
	}

	.logo-panel {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 0.5rem;
		margin-top: 0.6rem;
		padding: 0.8rem;
		border-radius: var(--radius-sm);
		background: var(--fill);
	}

	.file-card {
		display: flex;
		align-items: center;
		gap: 0.65rem;
		padding: 10px 6px 10px 10px;
		border-radius: var(--radius);
		background: var(--surface);
		box-shadow: var(--shadow-sm);
	}

	.file-card.has-error {
		background: var(--danger-tint);
	}

	.file-icon {
		width: 40px;
		height: 40px;
		flex-shrink: 0;
		border-radius: 10px;
		background: var(--surface);
		color: var(--accent-ink);
		box-shadow: var(--shadow-sm);
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.file-meta {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}

	.file-name {
		font-size: 0.9rem;
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.file-size {
		font-size: 0.8rem;
	}

	.link-btn {
		appearance: none;
		background: none;
		border: none;
		color: var(--accent-ink);
		font: inherit;
		font-size: 0.85rem;
		font-weight: 500;
		cursor: pointer;
		min-height: 44px;
		padding: 0 0.6rem;
		border-radius: var(--radius-sm);
	}

	.icon-btn {
		appearance: none;
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		width: 44px;
		height: 44px;
		flex: none;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border-radius: 50%;
	}

	.icon-btn:hover {
		color: var(--text);
		background: var(--fill-2);
	}

	.accent {
		color: var(--accent-ink);
		font-weight: 500;
	}

	.small {
		font-size: 0.8rem;
		display: block;
		margin-top: 0.35rem;
	}

	.error-banner {
		margin: 0 0 1rem;
	}

	.progress {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.progress-head {
		display: flex;
		justify-content: space-between;
		font-size: 0.85rem;
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
		transition: width 0.2s ease;
	}
</style>
