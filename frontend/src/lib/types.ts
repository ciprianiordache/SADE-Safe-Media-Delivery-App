// Mirrors the JSON DTOs in internals/app/{user,job,asset}/model.go. Keep in
// sync by hand - there is no codegen step.

export type Role = 'operator' | 'admin';

export interface User {
	id: string;
	email: string;
	role: Role;
	createdAt: string;
	updatedAt: string;
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
}

export interface NewJobInput {
	file: File;
	recipientEmail: string;
	watermarkKind: WatermarkKind;
	watermarkText?: string;
}

// Mirrors internals/app/payment.statusResponse / checkoutResponse.
export interface PaymentStatus {
	enabled: boolean; // false when the server has no Stripe key configured
	paid: boolean;
	originalUrl?: string; // set only when paid - a signed GET /o/{token} link
	amountCents: number;
	currency: string;
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
