// Mirrors the JSON DTOs in internals/app/{user,job,asset}/model.go. Keep in
// sync by hand - there is no codegen step.

export type Role = 'operator' | 'admin';

export interface User {
	id: string;
	email: string;
	role: Role;
	createdAt: string;
	updatedAt: string;
	requirePasskey: boolean;
}

// --- sign-in & account security (internals/app/auth) ----------------------

// What a finished email step returns: signed in, or a passkey still needed.
export type SignInResult =
	| { status: 'signed_in' }
	| { status: 'passkey_required'; passkey: PasskeyChallenge };

export interface PasskeyChallenge {
	ceremonyId: string;
	options: Record<string, unknown>;
}

export interface SessionInfo {
	id: string;
	device: string;
	ip: string;
	createdAt: string;
	lastSeenAt: string;
	current: boolean;
}

export interface ActivityEntry {
	method: 'link' | 'code' | 'passkey';
	device: string;
	ip: string;
	createdAt: string;
}

export interface PasskeyInfo {
	id: string;
	name: string;
	createdAt: string;
	lastUsedAt: string;
}

export type JobStatus = 'pending' | 'processing' | 'done' | 'failed';
export type MediaType = 'video' | 'audio' | 'image';
export type WatermarkKind = 'logo' | 'text' | 'both';
export type AssetKind = 'original' | 'preview';

export interface Asset {
	id: string;
	jobId: string;
	kind: AssetKind;
	filename: string;
	mime: string;
	sizeBytes: number;
	checksum?: string;
	createdAt: string;
}

export interface Job {
	id: string;
	status: JobStatus;
	mediaType: MediaType;
	recipientEmail: string;
	watermarkKind: WatermarkKind;
	watermarkText?: string;
	attempts: number;
	error?: string;
	createdAt: string;
	updatedAt: string;
	// Populated only by GET /api/jobs/{id}; the list endpoint leaves it out.
	assets?: Asset[];
	linksDisabled: boolean;
	// Delivery signals: when the recipient last opened the preview, and
	// whether they paid for the original.
	openedAt?: string;
	paid: boolean;
}

// Mirrors internals/app/share.Delivery - GET /api/jobs/{id}/delivery.
export type DeliveryEventKind =
	| 'sent'
	| 'resent'
	| 'viewed'
	| 'downloaded'
	| 'paid'
	| 'original_downloaded'
	| 'links_disabled'
	| 'links_enabled';

export interface DeliveryEvent {
	kind: DeliveryEventKind;
	detail?: string;
	createdAt: string;
}

export interface Delivery {
	enabled: boolean;
	paid: boolean;
	ready: boolean;
	previewUrl?: string;
	expiresAt?: string;
	events: DeliveryEvent[];
}

export interface NewJobInput {
	file: File;
	recipientEmail: string;
	watermarkKind: WatermarkKind;
	watermarkText?: string;
}

// Mirrors internals/app/job.Usage - the caller's consumption against their
// quotas. A zero limit means that quota is off; unlimited is set for admins.
export interface Usage {
	jobsLast24h: number;
	jobsPerDay: number;
	storageBytes: number;
	storageLimitBytes: number;
	unlimited: boolean;
}

// Mirrors internals/app/payment.statusResponse.
export interface PaymentStatus {
	enabled: boolean; // false when the server has no Stripe key configured
	paid: boolean;
	originalUrl?: string; // set only when paid - a signed GET /o/{token} link
	amountCents: number;
	currency: string;
	publishableKey?: string; // set whenever enabled - mounts Stripe.js client-side
}

// Mirrors config.UploadConfig's default Allowed* extensions
// (internals/app/job checks the real config server-side; this is just a
// fast client-side check so a bad file never reaches the upload button).
export const ALLOWED_EXTENSIONS = [
	'.mp4',
	'.mov',
	'.mkv',
	'.webm',
	'.avi',
	'.mp3',
	'.wav',
	'.m4a',
	'.aac',
	'.flac',
	'.ogg',
	'.jpg',
	'.jpeg',
	'.png',
	'.webp',
	'.tiff'
];

export function isAllowedFile(filename: string): boolean {
	const dot = filename.lastIndexOf('.');
	if (dot < 0) return false;
	return ALLOWED_EXTENSIONS.includes(filename.slice(dot).toLowerCase());
}

// Client-side guess of the media kind from the extension, for the upload
// form's file icon. The server decides for real (ffprobe).
export function mediaTypeOf(filename: string): MediaType | null {
	const ext = filename.slice(filename.lastIndexOf('.')).toLowerCase();
	if (['.mp4', '.mov', '.mkv', '.webm', '.avi'].includes(ext)) return 'video';
	if (['.mp3', '.wav', '.m4a', '.aac', '.flac', '.ogg'].includes(ext)) return 'audio';
	if (['.jpg', '.jpeg', '.png', '.webp', '.tiff'].includes(ext)) return 'image';
	return null;
}
