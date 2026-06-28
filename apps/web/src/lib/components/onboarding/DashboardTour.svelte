<script lang="ts">
	import { onMount, onDestroy } from 'svelte';

	// ─── Props ───────────────────────────────────────────────────────────────────
	let {
		userId,
		onComplete
	}: {
		userId: string;
		onComplete: () => void;
	} = $props();

	// ─── Tour Step Definitions ────────────────────────────────────────────────────
	const STEPS = [
		{
			selector: '#energy-slider',
			title: 'Circadian Energy Profiler',
			description:
				'Adjust your cognitive state score from 1 (Exceeded) to 10 (Peak) to auto-prioritize tasks. Low energy recommends low-effort admin drafts, while high energy unlocks complex strategic tasks.',
			icon: '⚡',
			accent: '#7c6fff'
		},
		{
			selector: '#friction-stat-card',
			title: 'Friction Reduction Index',
			description:
				'View real-time value metrics. Tracks the cumulative and active time saved by delegating emails, scheduling, and payments directly to the agent.',
			icon: '📊',
			accent: '#34d399'
		},
		{
			selector: '#actions-queue',
			title: '1-Tap Actions Queue',
			description:
				'Review and resolve incoming email drafts, calendar slots, and utility bill checkouts. A single click completes the entire backend automation.',
			icon: '🚀',
			accent: '#f59e0b'
		},
		{
			selector: '#calendar-view',
			title: 'Micro-Gap Focus Calendar',
			description:
				'See your daily micro-gaps. The system automatically creates "Ghost Blocks" to protect focus time and dissolves them if tasks are manually cleared.',
			icon: '📅',
			accent: '#38bdf8'
		}
	] as const;

	// ─── State ────────────────────────────────────────────────────────────────────
	let stepIndex = $state(0);
	let visible = $state(false);

	// Popover position — driven by a spring animation
	let popoverX = $state(0);
	let popoverY = $state(0);
	let popoverW = $state(360);
	let popoverOpacity = $state(0);

	// Spotlight hole geometry (in viewport coords)
	let spotX = $state(0);
	let spotY = $state(0);
	let spotW = $state(0);
	let spotH = $state(0);

	// Spring targets (raw positions before lerp)
	let targetX = 0;
	let targetY = 0;
	let targetSpotX = 0;
	let targetSpotY = 0;
	let targetSpotW = 0;
	let targetSpotH = 0;

	let rafId: number | null = null;
	let resizeObserver: ResizeObserver | null = null;

	// ─── Derived ──────────────────────────────────────────────────────────────────
	let currentStep = $derived(STEPS[stepIndex]);
	let isLastStep = $derived(stepIndex === STEPS.length - 1);
	let stepLabel = $derived(`Step ${stepIndex + 1} of ${STEPS.length}`);

	// Spotlight overlay shadow: a solid shadow carving a hole in the overlay
	let spotlightShadow = $derived(
		visible
			? `${spotX}px ${spotY}px 0 0 rgba(18,18,29,0.88), 0 0 0 9999px rgba(18,18,29,0.88)`
			: '0 0 0 9999px rgba(18,18,29,0)'
	);

	// ─── Positioning Engine ───────────────────────────────────────────────────────
	function computePositions(el: Element) {
		const rect = el.getBoundingClientRect();
		const PADDING = 12; // px padding around spotlight hole
		const POP_W = 360;
		const POP_H = 220; // estimated popover height
		const MARGIN = 16;

		// Spotlight hole (we draw a transparent div over the element)
		targetSpotX = rect.left - PADDING;
		targetSpotY = rect.top - PADDING;
		targetSpotW = rect.width + PADDING * 2;
		targetSpotH = rect.height + PADDING * 2;

		// Popover placement: prefer right, then left, then bottom
		const vw = window.innerWidth;
		const vh = window.innerHeight;

		let px: number;
		let py: number;

		const spaceRight = vw - rect.right - PADDING;
		const spaceLeft = rect.left - PADDING;
		const spaceBelow = vh - rect.bottom - PADDING;

		if (spaceRight >= POP_W + MARGIN) {
			// Place right
			px = rect.right + PADDING + MARGIN;
			py = Math.max(MARGIN, Math.min(rect.top, vh - POP_H - MARGIN));
		} else if (spaceLeft >= POP_W + MARGIN) {
			// Place left
			px = rect.left - PADDING - MARGIN - POP_W;
			py = Math.max(MARGIN, Math.min(rect.top, vh - POP_H - MARGIN));
		} else if (spaceBelow >= POP_H + MARGIN) {
			// Place below
			px = Math.max(MARGIN, Math.min(rect.left, vw - POP_W - MARGIN));
			py = rect.bottom + PADDING + MARGIN;
		} else {
			// Fallback: place above
			px = Math.max(MARGIN, Math.min(rect.left, vw - POP_W - MARGIN));
			py = rect.top - PADDING - MARGIN - POP_H;
		}

		targetX = px;
		targetY = py;
	}

	// ─── Spring Animation Loop ────────────────────────────────────────────────────
	function springLoop() {
		const STIFFNESS = 0.12;

		popoverX += (targetX - popoverX) * STIFFNESS;
		popoverY += (targetY - popoverY) * STIFFNESS;
		spotX += (targetSpotX - spotX) * STIFFNESS;
		spotY += (targetSpotY - spotY) * STIFFNESS;
		spotW += (targetSpotW - spotW) * STIFFNESS;
		spotH += (targetSpotH - spotH) * STIFFNESS;

		const deltaSum =
			Math.abs(targetX - popoverX) +
			Math.abs(targetY - popoverY) +
			Math.abs(targetSpotX - spotX) +
			Math.abs(targetSpotY - spotY) +
			Math.abs(targetSpotW - spotW) +
			Math.abs(targetSpotH - spotH);

		if (deltaSum > 0.1) {
			rafId = requestAnimationFrame(springLoop);
		} else {
			rafId = null;
		}
	}

	function startSpring() {
		if (rafId) cancelAnimationFrame(rafId);
		rafId = requestAnimationFrame(springLoop);
	}

	// ─── Step Navigation ──────────────────────────────────────────────────────────
	function updateStep(index: number) {
		const step = STEPS[index];
		const el = document.querySelector(step.selector);
		if (!el) return;

		// Scroll element into view smoothly so it's visible
		el.scrollIntoView({ behavior: 'smooth', block: 'center', inline: 'nearest' });

		// Give scroll time to settle then compute positions
		setTimeout(() => {
			const freshEl = document.querySelector(step.selector);
			if (!freshEl) return;
			computePositions(freshEl);
			popoverOpacity = 0;
			startSpring();
			setTimeout(() => {
				popoverOpacity = 1;
			}, 120);
		}, 300);
	}

	function next() {
		if (isLastStep) {
			finish();
		} else {
			stepIndex++;
			updateStep(stepIndex);
		}
	}

	function back() {
		if (stepIndex > 0) {
			stepIndex--;
			updateStep(stepIndex);
		}
	}

	function finish() {
		visible = false;
		onComplete();
	}

	function skip() {
		visible = false;
		onComplete();
	}

	// ─── Keyboard Handler ─────────────────────────────────────────────────────────
	function handleKeydown(e: KeyboardEvent) {
		if (!visible) return;
		if (e.key === 'Escape') skip();
		if (e.key === 'Enter' || e.key === 'ArrowRight') next();
		if (e.key === 'ArrowLeft') back();
	}

	// ─── Lifecycle ────────────────────────────────────────────────────────────────
	export function startTour() {
		stepIndex = 0;
		visible = true;

		// Wait for the backdrop to render, then compute step 0
		setTimeout(() => {
			const el = document.querySelector(STEPS[0].selector);
			if (!el) return;
			const rect = el.getBoundingClientRect();

			// Instantly snap to correct target before spring
			targetSpotX = rect.left - 12;
			targetSpotY = rect.top - 12;
			targetSpotW = rect.width + 24;
			targetSpotH = rect.height + 24;
			spotX = targetSpotX;
			spotY = targetSpotY;
			spotW = targetSpotW;
			spotH = targetSpotH;

			computePositions(el);
			popoverX = targetX;
			popoverY = targetY;

			popoverOpacity = 0;
			setTimeout(() => {
				popoverOpacity = 1;
			}, 80);
		}, 80);
	}

	onMount(() => {
		window.addEventListener('keydown', handleKeydown);

		// Recompute on resize
		resizeObserver = new ResizeObserver(() => {
			if (visible) updateStep(stepIndex);
		});
		resizeObserver.observe(document.body);
	});

	onDestroy(() => {
		if (typeof window !== 'undefined') {
			window.removeEventListener('keydown', handleKeydown);
			if (rafId) cancelAnimationFrame(rafId);
		}
		resizeObserver?.disconnect();
	});
</script>

{#if visible}
	<!-- ─── Full-Screen Backdrop ─────────────────────────────────────────────────── -->
	<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
	<div
		class="tour-overlay"
		onclick={skip}
		role="presentation"
		aria-hidden="true"
	>
		<!-- Spotlight hole — positioned element that's transparent, creating the "cutout" effect -->
		<div
			class="tour-spotlight"
			style="
				left: {spotX}px;
				top: {spotY}px;
				width: {spotW}px;
				height: {spotH}px;
			"
		></div>
	</div>

	<!-- ─── Popover ──────────────────────────────────────────────────────────────── -->
	<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
	<div
		class="tour-popover"
		style="
			left: {popoverX}px;
			top: {popoverY}px;
			opacity: {popoverOpacity};
			--accent: {currentStep.accent};
			width: {popoverW}px;
		"
		onclick={(e) => e.stopPropagation()}
		role="dialog"
		aria-modal="true"
		aria-label="Onboarding Tour Step {stepIndex + 1}"
	>
		<!-- Step Progress Bar -->
		<div class="tour-progress-bar">
			{#each STEPS as _, i}
				<div
					class="tour-progress-dot"
					class:active={i === stepIndex}
					class:done={i < stepIndex}
				></div>
			{/each}
		</div>

		<!-- Header -->
		<div class="tour-header">
			<span class="tour-icon" style="background: color-mix(in srgb, {currentStep.accent} 15%, transparent);">
				{currentStep.icon}
			</span>
			<div>
				<p class="tour-step-label">{stepLabel}</p>
				<h3 class="tour-title">{currentStep.title}</h3>
			</div>
		</div>

		<!-- Description -->
		<p class="tour-description">{currentStep.description}</p>

		<!-- ─── Actions ──────────────────────────────────────────────────────────────── -->
		<div class="tour-actions">
			<button class="tour-skip" onclick={skip}>
				Skip Tour
			</button>

			<div class="tour-nav">
				{#if stepIndex > 0}
					<button class="tour-back" onclick={back}>
						← Back
					</button>
				{/if}
				<button
					class="tour-next"
					style="background: {currentStep.accent}; box-shadow: 0 0 20px color-mix(in srgb, {currentStep.accent} 40%, transparent);"
					onclick={next}
				>
					{isLastStep ? '🎉 Finish' : 'Next →'}
				</button>
			</div>
		</div>

		<!-- Keyboard hint -->
		<p class="tour-hint">
			<kbd>←</kbd><kbd>→</kbd> navigate &nbsp;·&nbsp; <kbd>Enter</kbd> next &nbsp;·&nbsp; <kbd>Esc</kbd> skip
		</p>
	</div>
{/if}

<style>
	/* ─── Overlay ─────────────────────────────────────────────────────────────── */
	.tour-overlay {
		position: fixed;
		inset: 0;
		z-index: 9000;
		pointer-events: auto;
		cursor: pointer;
		/* The dark overlay — spotlight hole is cut by the .tour-spotlight element */
		background: transparent;
	}

	/* The spotlight "hole" is transparent but surrounded by the dark overlay via
	   a massive box-shadow that fills everything outside this element. */
	.tour-spotlight {
		position: fixed;
		pointer-events: none;
		border-radius: 14px;
		box-shadow:
			0 0 0 4px rgba(255, 255, 255, 0.08),
			0 0 0 9999px rgba(18, 18, 29, 0.88);
		transition: box-shadow 0.2s ease;
		z-index: 9001;
	}

	/* ─── Popover Card ────────────────────────────────────────────────────────── */
	.tour-popover {
		position: fixed;
		z-index: 9100;
		max-width: 380px;
		border-radius: 20px;
		padding: 24px;
		background: rgba(18, 18, 29, 0.96);
		border: 1px solid rgba(255, 255, 255, 0.1);
		backdrop-filter: blur(24px) saturate(160%);
		-webkit-backdrop-filter: blur(24px) saturate(160%);
		box-shadow:
			0 32px 64px rgba(0, 0, 0, 0.6),
			0 0 0 1px rgba(255, 255, 255, 0.04) inset;
		transition: opacity 0.18s ease;
		pointer-events: auto;
	}

	/* ─── Progress Dots ───────────────────────────────────────────────────────── */
	.tour-progress-bar {
		display: flex;
		gap: 6px;
		margin-bottom: 20px;
	}

	.tour-progress-dot {
		height: 3px;
		flex: 1;
		border-radius: 99px;
		background: rgba(255, 255, 255, 0.1);
		transition: background 0.3s ease;
	}

	.tour-progress-dot.active {
		background: var(--accent, #7c6fff);
	}

	.tour-progress-dot.done {
		background: rgba(255, 255, 255, 0.35);
	}

	/* ─── Header ──────────────────────────────────────────────────────────────── */
	.tour-header {
		display: flex;
		align-items: flex-start;
		gap: 14px;
		margin-bottom: 14px;
	}

	.tour-icon {
		flex-shrink: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 44px;
		height: 44px;
		border-radius: 12px;
		font-size: 22px;
		border: 1px solid rgba(255, 255, 255, 0.08);
	}

	.tour-step-label {
		font-size: 10px;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: rgba(255, 255, 255, 0.35);
		margin: 0 0 4px;
	}

	.tour-title {
		font-size: 16px;
		font-weight: 700;
		color: #ffffff;
		margin: 0;
		line-height: 1.25;
		letter-spacing: -0.01em;
	}

	/* ─── Description ─────────────────────────────────────────────────────────── */
	.tour-description {
		font-size: 13px;
		line-height: 1.65;
		color: rgba(255, 255, 255, 0.6);
		margin: 0 0 22px;
	}

	/* ─── Actions Row ─────────────────────────────────────────────────────────── */
	.tour-actions {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
	}

	.tour-skip {
		background: none;
		border: none;
		cursor: pointer;
		font-size: 12px;
		color: rgba(255, 255, 255, 0.3);
		padding: 0;
		transition: color 0.2s ease;
		text-decoration: underline;
		text-underline-offset: 3px;
		flex-shrink: 0;
	}

	.tour-skip:hover {
		color: rgba(255, 255, 255, 0.6);
	}

	.tour-nav {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.tour-back {
		background: rgba(255, 255, 255, 0.06);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: 10px;
		cursor: pointer;
		font-size: 12px;
		font-weight: 600;
		color: rgba(255, 255, 255, 0.55);
		padding: 8px 14px;
		transition:
			background 0.2s ease,
			color 0.2s ease;
	}

	.tour-back:hover {
		background: rgba(255, 255, 255, 0.1);
		color: rgba(255, 255, 255, 0.85);
	}

	.tour-next {
		border: none;
		border-radius: 10px;
		cursor: pointer;
		font-size: 13px;
		font-weight: 700;
		color: #fff;
		padding: 9px 20px;
		letter-spacing: 0.01em;
		transition:
			transform 0.15s ease,
			box-shadow 0.2s ease;
	}

	.tour-next:hover {
		transform: translateY(-1px);
	}

	.tour-next:active {
		transform: translateY(0);
	}

	/* ─── Keyboard Hint ───────────────────────────────────────────────────────── */
	.tour-hint {
		margin: 14px 0 0;
		font-size: 10px;
		color: rgba(255, 255, 255, 0.2);
		text-align: center;
		letter-spacing: 0.02em;
	}

	.tour-hint kbd {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 1px 5px;
		border-radius: 4px;
		background: rgba(255, 255, 255, 0.06);
		border: 1px solid rgba(255, 255, 255, 0.1);
		font-family: monospace;
		font-size: 9px;
		color: rgba(255, 255, 255, 0.35);
	}
</style>
