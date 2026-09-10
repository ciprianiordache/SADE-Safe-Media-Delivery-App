<script lang="ts">
	// Public, login-free page for the link SADE emails a recipient. The token
	// itself is the whole authorization (internals/app/share), so this page
	// makes no /api call - it just points a player at the Go-served stream.
	// We don't know the media kind up front, so we try <video>, then
	// <audio>, then <img>, and fall back to a plain link.
	import { page } from '$app/state';
	import { i18n } from '$lib/i18n.svelte';
	import { API_BASE } from '$lib/api';
	import MarkBar from '$lib/components/MarkBar.svelte';

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

<div class="shell">
	<div class="top">
		<MarkBar size={20} />
	</div>

	<div class="content">
		<h1>{i18n.t('preview.title')}</h1>

		<div class="media">
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

		<a class="btn" href={downloadUrl} style="width: 100%">{i18n.t('preview.download')}</a>
	</div>

	<div class="footer faint">DELIVERED SECURELY BY SADE</div>
</div>

<style>
	.shell {
		min-height: 100vh;
		max-width: 480px;
		margin: 0 auto;
		display: flex;
		flex-direction: column;
	}

	.top {
		padding: 1.35rem 1.25rem 0;
	}

	.content {
		flex: 1;
		display: flex;
		flex-direction: column;
		justify-content: center;
		gap: 1.1rem;
		padding: 1.25rem;
	}

	h1 {
		margin: 0;
		font-size: 1.2rem;
		font-weight: 700;
	}

	.media {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.75rem;
	}

	.media video,
	.media img {
		width: 100%;
		border-radius: var(--radius);
		display: block;
	}

	.media audio {
		width: 100%;
	}

	.footer {
		text-align: center;
		font-size: 0.68rem;
		letter-spacing: 0.04em;
		padding: 1rem 1.25rem 1.6rem;
	}
</style>
