export type Locale = 'ro' | 'en';

const STORAGE_KEY = 'sade:locale';

const dict = {
	ro: {
		'app.name': 'SADE',
		'app.tagline': 'Livrare sigură de fișiere media, cu filigran',
		'nav.dashboard': 'Panou',
		'nav.signOut': 'Deconectare',
		'landing.title': 'Trimite fișiere media în siguranță',
		'landing.subtitle':
			'Încarci un fișier video, audio sau imagine, aplicăm automat un filigran, iar destinatarul primește un link securizat către previzualizare.',
		'landing.cta': 'Intră în cont',
		'landing.ctaApp': 'Mergi la panou',
		'login.title': 'Autentificare',
		'login.subtitle': 'Introdu adresa de email și îți trimitem un link de autentificare.',
		'login.emailLabel': 'Email',
		'login.emailPlaceholder': 'nume@exemplu.com',
		'login.submit': 'Trimite link-ul',
		'login.submitting': 'Se trimite…',
		'login.sent': 'Verifică-ți inboxul — ți-am trimis un link de autentificare.',
		'login.errorInvalidLink': 'Acel link este invalid sau a expirat. Încearcă din nou.',
		'login.errorGeneric': 'Nu am putut trimite link-ul. Încearcă din nou.',
		'dashboard.title': 'Fișiere',
		'dashboard.subtitle': 'Încarci un fișier, îl marcăm cu filigran, destinatarul primește un link.',
		'dashboard.autoRefresh': 'Actualizare automată cât timp rulează fișierele',
		'dashboard.newJob': 'Fișier nou',
		'dashboard.recipientLabel': 'Email destinatar',
		'dashboard.fileLabel': 'Fișier media',
		'dashboard.dropText': 'Trage un fișier sau',
		'dashboard.dropBrowse': 'alege',
		'dashboard.allowedTypes': 'mp4 · mov · mp3 · wav · jpg · png',
		'dashboard.watermarkLabel': 'Filigran',
		'dashboard.watermarkKind.logo': 'Logo',
		'dashboard.watermarkKind.text': 'Text',
		'dashboard.watermarkKind.both': 'Logo + text',
		'dashboard.watermarkTextLabel': 'Text filigran',
		'dashboard.optional': '· opțional',
		'dashboard.watermarkTextPlaceholder': 'Ex: Confidențial — nu redistribui',
		'dashboard.watermarkHint': 'Lasă gol pentru valoarea implicită — destinatar + dată.',
		'dashboard.submit': 'Încarcă și pune în coadă',
		'dashboard.submitting': 'Se încarcă…',
		'dashboard.uploadError': 'Încărcarea a eșuat.',
		'dashboard.fileUnsupported': 'tip nesuportat',
		'dashboard.fileChooseSupported': 'Alege un fișier suportat ca să continui.',
		'dashboard.searchPlaceholder': 'Caută după destinatar',
		'dashboard.filter.all': 'Toate',
		'dashboard.filter.pending': 'În așteptare',
		'dashboard.filter.processing': 'Se procesează',
		'dashboard.filter.done': 'Finalizate',
		'dashboard.filter.failed': 'Eșuate',
		'dashboard.sortLabel': 'Sortare',
		'dashboard.sort.newest': 'Cele mai noi',
		'dashboard.sort.oldest': 'Cele mai vechi',
		'dashboard.sort.status': 'Status',
		'dashboard.densityComfortable': 'Confortabil',
		'dashboard.densityCompact': 'Compact',
		'dashboard.viewList': 'Listă',
		'dashboard.viewGrid': 'Grilă',
		'dashboard.rowsLabel': 'Rânduri',
		'dashboard.prev': 'Înapoi',
		'dashboard.next': 'Înainte',
		'dashboard.of': 'din',
		'dashboard.jobsEmpty': 'Nu ai încărcat încă niciun fișier.',
		'dashboard.emptyTitle': 'Niciun fișier încă',
		'dashboard.emptyBody':
			'Completează formularul din stânga și încarcă un fișier — îl marcăm cu filigran și trimitem destinatarului un link.',
		'dashboard.noMatches': 'Niciun fișier nu corespunde filtrelor curente.',
		'dashboard.col.file': 'Destinatar',
		'dashboard.col.status': 'Status',
		'dashboard.col.type': 'Tip',
		'dashboard.col.created': 'Creat',
		'status.pending': 'În așteptare',
		'status.processing': 'Se procesează',
		'status.done': 'Finalizat',
		'status.failed': 'Eșuat',
		'job.title': 'Detalii fișier',
		'job.status': 'Status',
		'job.mediaType': 'Tip media',
		'job.recipient': 'Destinatar',
		'job.watermarkKind': 'Tip filigran',
		'job.watermarkText': 'Text filigran',
		'job.attempts': 'Încercări',
		'job.error': 'Eroare',
		'job.createdAt': 'Creat la',
		'job.updatedAt': 'Actualizat la',
		'job.assets': 'Fișiere asociate',
		'job.assetsEmpty': 'Încă nu există fișiere procesate.',
		'job.assetsNote':
			'Destinatarul primește pe email link-urile de previzualizare și descărcare odată ce procesarea s-a încheiat.',
		'job.back': 'Înapoi la panou',
		'job.notFound': 'Fișierul nu a fost găsit.',
		'job.stepQueued': 'În coadă',
		'job.stepWatermarking': 'Se aplică filigranul',
		'job.stepDelivered': 'Livrat',
		'job.sectionPreview': 'Previzualizare',
		'job.tabPreview': 'Cu filigran',
		'job.tabOriginal': 'Original',
		'job.download': 'Descarcă',
		'job.previewNotReady': 'Fișierul procesat nu e disponibil încă.',
		'preview.title': 'Previzualizare',
		'preview.download': 'Descarcă originalul cu filigran',
		'preview.unsupported': 'Nu putem afișa acest fișier direct în browser.',
		'preview.openDirect': 'Deschide previzualizarea',
		'preview.unlockCta': 'Deblochează originalul',
		'preview.unlockDisclaimer':
			'Această previzualizare are un filigran vizibil și audibil. Deblocarea îl elimină și îți trimite fișierul original curat.',
		'preview.unlocked': 'Original deblocat',
		'preview.downloadOriginal': 'Descarcă originalul',
		'preview.checkingPayment': 'Se verifică plata…',
		'preview.unlockError': 'Nu am putut porni plata. Încearcă din nou.',
		'preview.keepsWorking': 'Link-ul rămâne valabil — revino oricând ca să descarci din nou.',
		'common.loading': 'Se încarcă…',
		'common.signOut': 'Deconectare'
	},
	en: {
		'app.name': 'SADE',
		'app.tagline': 'Safe media delivery, watermarked',
		'nav.dashboard': 'Dashboard',
		'nav.signOut': 'Sign out',
		'landing.title': 'Send media files safely',
		'landing.subtitle':
			'Upload a video, audio, or image file, we watermark it automatically, and the recipient gets a secure link to the preview.',
		'landing.cta': 'Sign in',
		'landing.ctaApp': 'Go to dashboard',
		'login.title': 'Sign in',
		'login.subtitle': "Enter your email and we'll send you a sign-in link.",
		'login.emailLabel': 'Email',
		'login.emailPlaceholder': 'name@example.com',
		'login.submit': 'Send link',
		'login.submitting': 'Sending…',
		'login.sent': "Check your inbox — we've sent you a sign-in link.",
		'login.errorInvalidLink': 'That link is invalid or has expired. Please try again.',
		'login.errorGeneric': 'Could not send the link. Please try again.',
		'dashboard.title': 'Jobs',
		'dashboard.subtitle': 'Upload a file, we watermark it, the recipient gets a link.',
		'dashboard.autoRefresh': 'Auto-refreshing while jobs run',
		'dashboard.newJob': 'New job',
		'dashboard.recipientLabel': 'Recipient email',
		'dashboard.fileLabel': 'Media file',
		'dashboard.dropText': 'Drop a file or',
		'dashboard.dropBrowse': 'browse',
		'dashboard.allowedTypes': 'mp4 · mov · mp3 · wav · jpg · png',
		'dashboard.watermarkLabel': 'Watermark',
		'dashboard.watermarkKind.logo': 'Logo',
		'dashboard.watermarkKind.text': 'Text',
		'dashboard.watermarkKind.both': 'Logo + text',
		'dashboard.watermarkTextLabel': 'Overlay text',
		'dashboard.optional': '· optional',
		'dashboard.watermarkTextPlaceholder': 'e.g. Confidential — do not redistribute',
		'dashboard.watermarkHint': 'Leave blank for the default — recipient + date.',
		'dashboard.submit': 'Upload & queue',
		'dashboard.submitting': 'Uploading…',
		'dashboard.uploadError': 'Upload failed.',
		'dashboard.fileUnsupported': 'unsupported type',
		'dashboard.fileChooseSupported': 'Choose a supported file to continue.',
		'dashboard.searchPlaceholder': 'Search by recipient',
		'dashboard.filter.all': 'All',
		'dashboard.filter.pending': 'Pending',
		'dashboard.filter.processing': 'Processing',
		'dashboard.filter.done': 'Done',
		'dashboard.filter.failed': 'Failed',
		'dashboard.sortLabel': 'Sort',
		'dashboard.sort.newest': 'Newest',
		'dashboard.sort.oldest': 'Oldest',
		'dashboard.sort.status': 'Status',
		'dashboard.densityComfortable': 'Comfortable',
		'dashboard.densityCompact': 'Compact',
		'dashboard.viewList': 'List',
		'dashboard.viewGrid': 'Grid',
		'dashboard.rowsLabel': 'Rows',
		'dashboard.prev': 'Prev',
		'dashboard.next': 'Next',
		'dashboard.of': 'of',
		'dashboard.jobsEmpty': "You haven't uploaded any files yet.",
		'dashboard.emptyTitle': 'No jobs yet',
		'dashboard.emptyBody':
			"Fill in the form on the left and upload a file — we'll watermark it and email your recipient a link.",
		'dashboard.noMatches': 'No jobs match the current filters.',
		'dashboard.col.file': 'Recipient',
		'dashboard.col.status': 'Status',
		'dashboard.col.type': 'Type',
		'dashboard.col.created': 'Created',
		'status.pending': 'Pending',
		'status.processing': 'Processing',
		'status.done': 'Done',
		'status.failed': 'Failed',
		'job.title': 'File details',
		'job.status': 'Status',
		'job.mediaType': 'Media type',
		'job.recipient': 'Recipient',
		'job.watermarkKind': 'Watermark type',
		'job.watermarkText': 'Watermark text',
		'job.attempts': 'Attempts',
		'job.error': 'Error',
		'job.createdAt': 'Created',
		'job.updatedAt': 'Updated',
		'job.assets': 'Files',
		'job.assetsEmpty': 'No processed files yet.',
		'job.assetsNote':
			'The recipient gets the preview and download links by email once processing finishes.',
		'job.back': 'Back to dashboard',
		'job.notFound': 'File not found.',
		'job.stepQueued': 'Queued',
		'job.stepWatermarking': 'Watermarking',
		'job.stepDelivered': 'Delivered',
		'job.sectionPreview': 'Preview',
		'job.tabPreview': 'Watermarked',
		'job.tabOriginal': 'Original',
		'job.download': 'Download',
		'job.previewNotReady': "The processed file isn't ready yet.",
		'preview.title': 'Preview',
		'preview.download': 'Download the watermarked file',
		'preview.unsupported': "We can't display this file directly in the browser.",
		'preview.openDirect': 'Open the preview',
		'preview.unlockCta': 'Unlock the original',
		'preview.unlockDisclaimer':
			'This preview carries a visible and audible watermark. Unlocking removes it and sends you the clean original.',
		'preview.unlocked': 'Original unlocked',
		'preview.downloadOriginal': 'Download the original',
		'preview.checkingPayment': 'Checking payment…',
		'preview.unlockError': 'Could not start checkout. Please try again.',
		'preview.keepsWorking': 'The link keeps working — come back anytime to download again.',
		'common.loading': 'Loading…',
		'common.signOut': 'Sign out'
	}
} as const satisfies Record<Locale, Record<string, string>>;

export type MessageKey = keyof (typeof dict)['ro'];

function readStored(): Locale {
	try {
		const v = localStorage.getItem(STORAGE_KEY);
		if (v === 'ro' || v === 'en') return v;
	} catch {
		// localStorage unavailable
	}
	return 'ro';
}

class I18nStore {
	locale = $state<Locale>('ro');

	init() {
		this.locale = readStored();
	}

	set(locale: Locale) {
		this.locale = locale;
		try {
			localStorage.setItem(STORAGE_KEY, locale);
		} catch {
			// best-effort persistence only
		}
	}

	t(key: MessageKey): string {
		return dict[this.locale][key] ?? key;
	}
}

export const i18n = new I18nStore();
