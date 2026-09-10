<script lang="ts">
	// Public, login-free page for the link SADE emails a recipient. The token
	// itself is the whole authorization (internals/app/share), so this page
	// makes no /api call - it just points a player at the Go-served stream.
	// We don't know the media kind up front, so we try <video>, then
	// <audio>, then <img>, and fall back to a plain link.
	import { page } from '$app/state';
	import { i18n } from '$lib/i18n.svelte';
	import { API_BASE } from '$lib/api';

	const token = page.params.token as string;
	const previewUrl = `${API_BASE}/p/${encodeURIComponent(token)}`;
	const downloadUrl = `${API_BASE}/d/${encodeURIComponent(token)}`;

	type Stage = 'video' | 'audio' | 'image' | 'none';
	let stage = $state<Stage>('video');

	function next() {
		if (stage === 'video') stage = 'audio';
		else if (stage === 'audio') stage = 'image';
		else stage = 'none';
	}
</script>

<div class="container">
	<h1>{i18n.t('preview.title')}</h1>

	<div class="card media">
		{#if stage === 'video'}
			<!-- svelte-ignore a11y_media_has_caption -->
			<video controls src={previewUrl} onerror={next}>
				<track kind="captions" />
			</video>
		{:else if stage === 'audio'}
			<audio controls src={previewUrl} onerror={next}></audio>
		{:else if stage === 'image'}
			<img src={previewUrl} alt="" onerror={next} />
		{:else}
			<p class="muted">{i18n.t('preview.unsupported')}</p>
			<a class="btn btn-ghost" href={previewUrl} target="_blank" rel="noreferrer">
				{i18n.t('preview.openDirect')}
			</a>
		{/if}
	</div>

	<a class="btn" href={downloadUrl}>{i18n.t('preview.download')}</a>
</div>

<style>
	.media {
		margin: 1.5rem 0;
	}

	.media video,
	.media img {
		max-width: 100%;
		border-radius: var(--radius);
	}

	.media audio {
		width: 100%;
	}
</style>
