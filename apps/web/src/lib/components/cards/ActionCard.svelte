<script lang="ts">
	import { onMount } from 'svelte';
	import { env } from '$env/dynamic/public';

	// Svelte 5 component properties
	let { task, onExecuteSuccess = () => {} } = $props<{
		task: any;
		onExecuteSuccess?: (taskId: string) => void;
	}>();

	// Gateway calls go through the SvelteKit server-side proxy (/proxy/...) to avoid
	// cross-domain cookie issues in production. PUBLIC_GATEWAY_URL is still needed
	// for legacy reference but no longer used for direct fetch calls here.

	let isExecuting = $state(false);
	let isSuccess = $state(false);
	let errorMessage = $state('');
	let timeRemainingText = $state('');
	let urgencyColor = $state('blue'); // 'red' | 'yellow' | 'blue'

	// Local editable values
	let draftBody = $state('');
	let bookingLocation = $state('Google Meet');
	let billAmount = $state('');

	// Parse Action Card Payload
	let cardPayload = $derived.by(() => {
		if (!task.actionCard?.payloadJson) return null;
		try {
			return JSON.parse(task.actionCard.payloadJson);
		} catch (e) {
			return null;
		}
	});

	// Compute urgency and deadline remaining
	function updateDeadline() {
		const now = Date.now();
		const diff = task.dueAt - now;

		if (diff <= 0) {
			timeRemainingText = 'Overdue';
			urgencyColor = 'red';
			return;
		}

		const hours = diff / (1000 * 60 * 60);
		if (hours < 2) {
			urgencyColor = 'red';
			const mins = Math.round(diff / (1000 * 60));
			timeRemainingText = `Due in ${mins}m`;
		} else if (hours < 6) {
			urgencyColor = 'yellow';
			timeRemainingText = `Due in ${Math.round(hours * 10) / 10}h`;
		} else {
			urgencyColor = 'blue';
			if (hours < 24) {
				timeRemainingText = `Due in ${Math.round(hours)}h`;
			} else {
				const days = Math.round(hours / 24);
				timeRemainingText = `Due in ${days} ${days === 1 ? 'day' : 'days'}`;
			}
		}
	}

	onMount(() => {
		updateDeadline();
		const interval = setInterval(updateDeadline, 60000);

		// Initialize local editable states from payload
		if (task.actionCard?.actionType === 'GMAIL_DRAFT') {
			draftBody = cardPayload?.body || task.actionCard?.payloadJson || '';
		} else if (task.actionCard?.actionType === 'CALENDAR_BOOKING') {
			bookingLocation = cardPayload?.location || 'Google Meet';
		} else if (task.actionCard?.actionType === 'BILL_PAY') {
			billAmount = cardPayload?.amount ? `$${cardPayload.amount}` : '$15.00';
		}

		return () => clearInterval(interval);
	});

	async function executeAction() {
		if (isExecuting || isSuccess) return;
		isExecuting = true;
		errorMessage = '';

		try {
			// Route through the SvelteKit server-side proxy so the session cookie
			// (on *.vercel.app) is never required on go-gateway's domain.
			const res = await fetch(`/proxy/v1/tasks/${task._id}/execute`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				}
			});

			if (!res.ok) {
				const txt = await res.text();
				throw new Error(txt || `Execution failed with status ${res.status}`);
			}

			isSuccess = true;
			setTimeout(() => {
				onExecuteSuccess(task._id);
			}, 1000);
		} catch (err: any) {
			slogError(err);
			errorMessage = err.message || 'Execution failed';
		} finally {
			isExecuting = false;
		}
	}

	function slogError(err: any) {
		const PUBLIC_ENV = env.PUBLIC_ENV || 'development';
		if (PUBLIC_ENV === 'development') {
			console.error('[ActionCard Error]:', err);
		}
	}
</script>

<div
	class="relative overflow-hidden rounded-2xl border border-white/5 bg-brand-card p-6 shadow-xl transition-all duration-300 hover:border-white/10 hover:shadow-2xl flex flex-col justify-between min-h-[300px] {isSuccess ? 'scale-95 opacity-50' : ''}"
>
	<!-- Decorative subtle glow -->
	<div class="absolute -top-10 -right-10 h-32 w-32 rounded-full pointer-events-none blur-[40px]
		{urgencyColor === 'red' ? 'bg-brand-urgent/10' : urgencyColor === 'yellow' ? 'bg-brand-warn/10' : 'bg-brand-ambient/10'}"
	></div>

	<div>
		<!-- Context Header -->
		<div class="flex items-center justify-between gap-2 mb-4">
			<!-- Urgency Delta Badge -->
			<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold tracking-wide shadow-sm
				{urgencyColor === 'red' ? 'bg-brand-urgent/10 text-brand-urgent border border-brand-urgent/20' : 
				 urgencyColor === 'yellow' ? 'bg-brand-warn/10 text-brand-warn border border-brand-warn/20' : 
				 'bg-brand-ambient/10 text-brand-ambient border border-brand-ambient/20'}"
			>
				{timeRemainingText}
			</span>

			<!-- Friction Reduction Estimator -->
			{#if task.actionCard?.savesMinutes}
				<span class="inline-flex items-center rounded-full bg-brand-success/15 border border-brand-success/20 px-2.5 py-0.5 text-xs font-medium text-brand-success">
					⚡ Saves {task.actionCard.savesMinutes}m
				</span>
			{/if}
		</div>

		<!-- Title -->
		<h3 class="text-lg font-bold tracking-tight text-brand-textMain font-heading mb-1 pr-6 truncate">
			{task.title}
		</h3>
		<p class="text-xs text-brand-textMuted font-sans mb-4">
			Source: <span class="font-medium text-slate-300">{task.source}</span>
		</p>

		<!-- Draft Asset Preview Panel (Editable) -->
		<div class="rounded-xl bg-[#090911] border border-white/[0.03] p-4 mb-5">
			{#if task.actionCard?.actionType === 'GMAIL_DRAFT'}
				<div class="flex flex-col gap-2">
					<div class="flex items-center justify-between text-[11px] text-brand-textMuted border-b border-white/5 pb-2">
						<span>Gmail Draft Response</span>
						<span class="text-brand-accent">Editable Preview</span>
					</div>
					<div class="text-[11px] flex gap-1"><span class="text-brand-textMuted">To:</span> <span class="text-brand-textMain font-mono truncate">{cardPayload?.to || 'client@acme.com'}</span></div>
					<textarea
						bind:value={draftBody}
						class="w-full h-24 mt-2 bg-transparent text-xs text-brand-textMain border-0 focus:ring-0 focus:outline-none resize-none font-sans leading-relaxed"
						placeholder="Compose email draft..."
					></textarea>
				</div>
			{:else if task.actionCard?.actionType === 'CALENDAR_BOOKING'}
				<div class="flex flex-col gap-2">
					<div class="flex items-center justify-between text-[11px] text-brand-textMuted border-b border-white/5 pb-2">
						<span>Calendar Reserve Block</span>
						<span class="text-brand-accent">Editable Location</span>
					</div>
					<div class="flex flex-col gap-1 mt-1 text-xs">
						<div class="flex justify-between"><span class="text-brand-textMuted">Slot:</span> <span class="text-brand-textMain font-medium">{cardPayload?.timeSlot || '3:15 PM - 3:45 PM'}</span></div>
						<div class="flex justify-between mt-1"><span class="text-brand-textMuted">Date:</span> <span class="text-brand-textMain font-medium">{cardPayload?.date || 'Today'}</span></div>
					</div>
					<input
						type="text"
						bind:value={bookingLocation}
						class="w-full mt-2 bg-transparent text-xs text-brand-textMain border-b border-white/5 pb-1 focus:border-brand-accent focus:ring-0 focus:outline-none font-sans"
						placeholder="Location / Link..."
					/>
				</div>
			{:else if task.actionCard?.actionType === 'BILL_PAY'}
				<div class="flex flex-col gap-2">
					<div class="flex items-center justify-between text-[11px] text-brand-textMuted border-b border-white/5 pb-2">
						<span>Secure Bill Checkout</span>
						<span class="text-brand-accent">Amount Confirmation</span>
					</div>
					<div class="flex flex-col gap-1 mt-1 text-xs">
						<div class="flex justify-between"><span class="text-brand-textMuted">Payee:</span> <span class="text-brand-textMain font-medium">{cardPayload?.payee || 'Utility Corp'}</span></div>
						<div class="flex justify-between mt-1"><span class="text-brand-textMuted">Due Date:</span> <span class="text-brand-textMain font-medium">{cardPayload?.dueDate || 'Tomorrow'}</span></div>
					</div>
					<input
						type="text"
						bind:value={billAmount}
						class="w-full mt-2 bg-transparent text-xs text-brand-textMain border-b border-white/5 pb-1 focus:border-brand-accent focus:ring-0 focus:outline-none font-sans font-mono"
						placeholder="Amount..."
					/>
				</div>
			{:else}
				<div class="text-xs text-brand-textMuted italic py-4 text-center">
					No automated action payload compiled.
				</div>
			{/if}
		</div>
	</div>

	<!-- Bottom Action Block -->
	<div>
		{#if errorMessage}
			<div class="text-xs text-brand-urgent mb-3 px-1">
				⚠️ {errorMessage}
			</div>
		{/if}

		<button
			id={`execute-btn-${task._id}`}
			onclick={executeAction}
			disabled={isExecuting || isSuccess}
			class="relative flex w-full items-center justify-center gap-2 rounded-xl py-3 px-4 text-sm font-semibold tracking-wide transition-all duration-300 cursor-pointer
				{isSuccess ? 'bg-brand-success text-white' : 
				 isExecuting ? 'bg-brand-accent/50 text-white/50 cursor-wait' :
				 urgencyColor === 'red' ? 'bg-brand-urgent hover:bg-brand-urgent/90 text-white shadow-lg shadow-brand-urgent/20 animate-pulse' : 
				 'bg-brand-accent hover:bg-brand-accentDark text-white shadow-lg shadow-brand-accent/15'}
				active:scale-[0.98] disabled:pointer-events-none"
		>
			{#if isSuccess}
				<svg class="h-5 w-5 animate-bounce" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
				</svg>
				<span>Approved & Sent!</span>
			{:else if isExecuting}
				<svg class="animate-spin h-5 w-5 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
					<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
					<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
				</svg>
				<span>Processing Resolution...</span>
			{:else if task.actionCard?.actionType === 'GMAIL_DRAFT'}
				<span>1-Tap Send Response</span>
			{:else if task.actionCard?.actionType === 'CALENDAR_BOOKING'}
				<span>1-Tap Confirm Slot</span>
			{:else if task.actionCard?.actionType === 'BILL_PAY'}
				<span>1-Tap Pay Bill</span>
			{:else}
				<span>Mark Task Completed</span>
			{/if}
		</button>
	</div>
</div>
