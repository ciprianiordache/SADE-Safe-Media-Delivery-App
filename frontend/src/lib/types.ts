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
