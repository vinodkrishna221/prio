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

	// Support fallback/aliases for payload naming differences
	let payeeName = $derived(cardPayload?.payee || cardPayload?.vendor || 'Utility Corp');
	let dueDateValue = $derived(cardPayload?.dueDate || cardPayload?.due_date || 'Tomorrow');

	// Dynamic visual themes for each action card type
	let cardTheme = $derived.by(() => {
		const type = task.actionCard?.actionType;
		if (type === 'GMAIL_DRAFT') {
			return {
				primaryColor: '#6366f1',
				glowClass: 'bg-indigo-500/10',
				borderClass: 'hover:border-indigo-500/30 border-t-2 border-t-indigo-500/20',
				badgeBg: 'bg-indigo-500/10 text-indigo-400 border-indigo-500/20',
				buttonClass: 'bg-indigo-600 hover:bg-indigo-700 text-white shadow-lg shadow-indigo-600/25',
				icon: 'email',
				label: 'Gmail Reply Draft'
			};
		} else if (type === 'CALENDAR_BOOKING') {
			return {
				primaryColor: '#10b981',
				glowClass: 'bg-emerald-500/10',
				borderClass: 'hover:border-emerald-500/30 border-t-2 border-t-emerald-500/20',
				badgeBg: 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20',
				buttonClass: 'bg-emerald-600 hover:bg-emerald-700 text-white shadow-lg shadow-emerald-600/25',
				icon: 'calendar',
				label: 'Calendar Slot'
			};
		} else if (type === 'BILL_PAY') {
			return {
				primaryColor: '#f59e0b',
				glowClass: 'bg-amber-500/10',
				borderClass: 'hover:border-amber-500/30 border-t-2 border-t-amber-500/20',
				badgeBg: 'bg-amber-500/10 text-amber-400 border-amber-500/20',
				buttonClass: 'bg-amber-600 hover:bg-amber-700 text-white shadow-lg shadow-amber-600/25',
				icon: 'bill',
				label: 'Bill Payment'
			};
		}
		// Fallback
		return {
			primaryColor: '#3b82f6',
			glowClass: 'bg-brand-ambient/10',
			borderClass: 'hover:border-white/10',
			badgeBg: 'bg-brand-ambient/10 text-brand-ambient border border-brand-ambient/20',
			buttonClass: 'bg-brand-accent hover:bg-brand-accentDark text-white shadow-lg shadow-brand-accent/15',
			icon: 'default',
			label: 'System Action'
		};
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
			const amt = cardPayload?.amount;
			if (amt !== undefined) {
				billAmount = String(amt).startsWith('$') ? String(amt) : `$${amt}`;
			} else {
				billAmount = '$15.00';
			}
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
	class="relative overflow-hidden rounded-2xl border border-white/5 bg-brand-card p-6 shadow-xl transition-all duration-300 hover:shadow-2xl flex flex-col justify-between min-h-[300px] {cardTheme.borderClass} {isSuccess ? 'scale-95 opacity-50' : ''}"
>
	<!-- Decorative subtle glow -->
	<div class="absolute -top-10 -right-10 h-32 w-32 rounded-full pointer-events-none blur-[40px]
		{cardTheme.glowClass}"
	></div>

	<div>
		<!-- Context Header -->
		<div class="flex items-center justify-between gap-2 mb-4">
			<div class="flex items-center gap-1.5">
				<!-- Action Type Badge -->
				<span class="inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold tracking-wide border {cardTheme.badgeBg}">
					{#if cardTheme.icon === 'email'}
						<svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
							<path stroke-linecap="round" stroke-linejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 0 1-2.25 2.25h-15a2.25 2.25 0 0 1-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0 0 19.5 4.5h-15a2.25 2.25 0 0 0-2.25 2.25m19.5 0v.243a2.25 2.25 0 0 1-1.07 1.916l-7.5 4.615a2.25 2.25 0 0 1-2.36 0L3.32 8.91a2.25 2.25 0 0 1-1.07-1.916V6.75" />
						</svg>
					{:else if cardTheme.icon === 'calendar'}
						<svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
							<path stroke-linecap="round" stroke-linejoin="round" d="M6.75 3v2.25M17.25 3v2.25M3 18.75V7.5a2.25 2.25 0 0 1 2.25-2.25h13.5A2.25 2.25 0 0 1 21 7.5v11.25m-18 0A2.25 2.25 0 0 0 5.25 21h13.5A2.25 2.25 0 0 0 21 18.75m-18 0v-7.5A2.25 2.25 0 0 1 5.25 9h13.5A2.25 2.25 0 0 1 21 11.25v7.5m-9-6h.008v.008H12v-.008ZM12 15h.008v.008H12V15Zm0 2.25h.008v.008H12v-.008ZM9.75 15h.008v.008H9.75V15Zm0 2.25h.008v.008H9.75v-.008ZM7.5 15h.008v.008H7.5V15Zm0 2.25h.008v.008H7.5v-.008Zm6.75-4.5h.008v.008h-.008v-.008Zm0 2.25h.008v.008h-.008V15Zm0 2.25h.008v.008h-.008v-.008Zm2.25-4.5h.008v.008H16.5v-.008Zm0 2.25h.008v.008H16.5V15Z" />
						</svg>
					{:else if cardTheme.icon === 'bill'}
						<svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
							<path stroke-linecap="round" stroke-linejoin="round" d="M2.25 8.25h19.5M2.25 9h19.5m-16.5 5.25h6m-6 2.25h3m-3.75 3h15a2.25 2.25 0 0 0 2.25-2.25V6.75A2.25 2.25 0 0 0 19.5 4.5h-15a2.25 2.25 0 0 0-2.25 2.25v10.5A2.25 2.25 0 0 0 4.5 19.5Z" />
						</svg>
					{/if}
					{cardTheme.label}
				</span>

				<!-- Urgency Delta Badge -->
				<span class="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold tracking-wide border
					{urgencyColor === 'red' ? 'bg-brand-urgent/10 text-brand-urgent border-brand-urgent/20 animate-pulse font-bold' : 
					 urgencyColor === 'yellow' ? 'bg-brand-warn/10 text-brand-warn border-brand-warn/20' : 
					 'bg-brand-ambient/10 text-brand-ambient border-brand-ambient/20'}"
				>
					{timeRemainingText}
				</span>
			</div>

			<!-- Friction Reduction Estimator -->
			{#if task.actionCard?.savesMinutes}
				<span class="inline-flex items-center rounded-full bg-brand-success/15 border border-brand-success/20 px-2 py-0.5 text-[10px] font-medium text-brand-success">
					⚡ Saves {task.actionCard.savesMinutes}m
				</span>
			{/if}
		</div>

		<!-- Title -->
		<h3 class="text-base font-bold tracking-tight text-brand-textMain font-heading mb-1 pr-6 truncate">
			{task.title}
		</h3>
		<p class="text-[10px] text-brand-textMuted font-sans mb-3">
			Source: <span class="font-medium text-slate-400">{task.source}</span>
		</p>

		<!-- Draft Asset Preview Panel (Editable) -->
		<div class="rounded-xl bg-[#090911] border border-white/[0.03] p-4 mb-5">
			{#if task.actionCard?.actionType === 'GMAIL_DRAFT'}
				<div class="flex flex-col gap-2">
					<!-- Mock Email Header Controls -->
					<div class="flex items-center justify-between text-[11px] text-brand-textMuted border-b border-white/5 pb-2">
						<div class="flex items-center gap-1.5">
							<span class="w-2 h-2 rounded-full bg-red-500/65"></span>
							<span class="w-2 h-2 rounded-full bg-yellow-500/65"></span>
							<span class="w-2 h-2 rounded-full bg-green-500/65"></span>
							<span class="ml-2 font-mono text-[9px] text-slate-500 truncate max-w-[125px]">{cardPayload?.subject || 'Reply Draft'}</span>
						</div>
						<span class="text-indigo-400 font-medium">Draft Editor</span>
					</div>
					<div class="text-[10px] flex flex-col gap-1 border-b border-white/5 py-1.5">
						<div class="flex gap-1.5"><span class="text-brand-textMuted w-7">To:</span> <span class="text-brand-textMain font-mono truncate">{cardPayload?.to || 'recipient@example.com'}</span></div>
						{#if cardPayload?.subject}
							<div class="flex gap-1.5"><span class="text-brand-textMuted w-7">Subj:</span> <span class="text-slate-300 font-medium truncate">{cardPayload.subject}</span></div>
						{/if}
					</div>
					<textarea
						bind:value={draftBody}
						class="w-full h-24 mt-1 bg-transparent text-xs text-brand-textMain border-0 focus:ring-0 focus:outline-none resize-none font-sans leading-relaxed placeholder-slate-700"
						placeholder="Compose email draft..."
					></textarea>
					<!-- Mock Email Editor toolbar -->
					<div class="flex items-center justify-between border-t border-white/5 pt-2 mt-1 select-none">
						<div class="flex gap-2.5 text-slate-600">
							<span class="font-bold text-xs cursor-default hover:text-slate-400">B</span>
							<span class="italic text-xs cursor-default hover:text-slate-400">I</span>
							<span class="underline text-xs cursor-default hover:text-slate-400">U</span>
							<span class="text-xs cursor-default hover:text-slate-400">
								<svg class="w-3.5 h-3.5 inline-block" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
									<path stroke-linecap="round" stroke-linejoin="round" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
								</svg>
							</span>
						</div>
						<span class="text-[9px] font-mono text-slate-600">Auto-saved</span>
					</div>
				</div>
			{:else if task.actionCard?.actionType === 'CALENDAR_BOOKING'}
				<div class="flex flex-col gap-3">
					<div class="flex items-center justify-between text-[11px] text-brand-textMuted border-b border-white/5 pb-2">
						<span class="font-medium text-emerald-400">Calendar Reserve Block</span>
						<span>Tentative Event</span>
					</div>
					
					<!-- Meeting Title -->
					<div class="text-xs font-bold text-brand-textMain truncate">
						{cardPayload?.title || task.title || 'Architecture Sync'}
					</div>

					<!-- Time Slot & Date Details in a Grid -->
					<div class="grid grid-cols-2 gap-3 text-xs bg-emerald-500/5 border border-emerald-500/10 rounded-xl p-2.5">
						<div class="flex flex-col">
							<span class="text-[9px] text-slate-500 font-mono">DATE</span>
							<span class="text-brand-textMain font-medium mt-0.5">{cardPayload?.date || 'Today'}</span>
						</div>
						<div class="flex flex-col border-l border-white/5 pl-3">
							<span class="text-[9px] text-slate-500 font-mono">TIME (SLOT)</span>
							<span class="text-brand-textMain font-medium mt-0.5 truncate">{cardPayload?.timeSlot || '3:15 PM - 3:45 PM'}</span>
						</div>
					</div>

					<!-- Attendee Initials-Bubbles -->
					{#if cardPayload?.attendees && cardPayload.attendees.length > 0}
						<div class="flex flex-col gap-1">
							<span class="text-[9px] text-slate-500 font-mono">ATTENDEES ({cardPayload.attendees.length})</span>
							<div class="flex -space-x-1.5 overflow-hidden">
								{#each cardPayload.attendees as attendee}
									<div 
										class="inline-flex items-center justify-center h-6 w-6 rounded-full bg-emerald-500/20 text-[9px] text-emerald-400 font-bold border border-[#090911] select-none"
										title={attendee}
									>
										{attendee.split('@')[0].substring(0, 2).toUpperCase()}
									</div>
								{/each}
							</div>
						</div>
					{/if}

					<!-- Editable Location Input -->
					<div class="flex flex-col gap-1 mt-1">
						<label for={`loc-input-${task._id}`} class="text-[9px] text-slate-500 font-mono">LOCATION / MEETING LINK</label>
						<div class="relative flex items-center">
							<svg class="absolute left-2.5 h-3.5 w-3.5 text-slate-500 pointer-events-none" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M15 10.5a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
								<path stroke-linecap="round" stroke-linejoin="round" d="M19.5 10.5c0 7.142-7.5 11.25-7.5 11.25S4.5 17.642 4.5 10.5a7.5 7.5 0 1 1 15 0Z" />
							</svg>
							<input
								id={`loc-input-${task._id}`}
								type="text"
								bind:value={bookingLocation}
								class="w-full pl-8 pr-3 py-1.5 bg-white/5 border border-white/5 rounded-lg text-xs text-brand-textMain focus:border-emerald-500/50 focus:ring-0 focus:outline-none font-sans"
								placeholder="Google Meet / Room link..."
							/>
						</div>
					</div>
				</div>
			{:else if task.actionCard?.actionType === 'BILL_PAY'}
				<div class="flex flex-col gap-3">
					<div class="flex items-center justify-between text-[11px] text-brand-textMuted border-b border-white/5 pb-2">
						<span class="font-medium text-amber-400">Secure Bill Checkout</span>
						<span>Pending Payment</span>
					</div>

					<!-- Receipt Detail Box -->
					<div class="flex flex-col gap-2 bg-amber-500/5 border border-amber-500/10 rounded-xl p-3 relative overflow-hidden">
						<!-- Dotted divider -->
						<div class="absolute -left-1 -right-1 top-1/2 border-t border-dashed border-amber-500/20 pointer-events-none"></div>

						<div class="flex justify-between text-xs z-10">
							<span class="text-slate-500">PAYEE / MERCHANT</span>
							<span class="text-brand-textMain font-bold">{payeeName}</span>
						</div>
						<div class="flex justify-between text-xs mt-3.5 z-10">
							<span class="text-slate-500">DUE DATE</span>
							<span class="text-brand-textMain font-bold text-amber-400">{dueDateValue}</span>
						</div>
					</div>

					<!-- Editable Amount input with card symbol -->
					<div class="flex flex-col gap-1 mt-1">
						<label for={`amount-input-${task._id}`} class="text-[9px] text-slate-500 font-mono">CONFIRM PAYMENT AMOUNT</label>
						<div class="relative flex items-center">
							<svg class="absolute left-2.5 h-3.5 w-3.5 text-amber-500/80 pointer-events-none" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M2.25 8.25h19.5M2.25 9h19.5m-16.5 5.25h6m-6 2.25h3m-3.75 3h15a2.25 2.25 0 0 0 2.25-2.25V6.75A2.25 2.25 0 0 0 19.5 4.5h-15a2.25 2.25 0 0 0-2.25 2.25v10.5A2.25 2.25 0 0 0 4.5 19.5Z" />
							</svg>
							<input
								id={`amount-input-${task._id}`}
								type="text"
								bind:value={billAmount}
								class="w-full pl-8 pr-8 py-1.5 bg-white/5 border border-white/5 rounded-lg text-xs text-brand-textMain focus:border-amber-500/50 focus:ring-0 focus:outline-none font-mono"
								placeholder="Amount..."
							/>
							<!-- Lock Icon -->
							<span class="absolute right-2.5 flex items-center text-slate-600" title="Secure SSL connection">
								<svg class="w-3 h-3 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
									<path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 1 0-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 0 0 2.25-2.25v-6.75a2.25 2.25 0 0 0-2.25-2.25H6.75a2.25 2.25 0 0 0-2.25 2.25v6.75a2.25 2.25 0 0 0 2.25 2.25Z" />
								</svg>
							</span>
						</div>
					</div>
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
				{isSuccess ? 'bg-brand-success text-white shadow-lg shadow-brand-success/20' : 
				 isExecuting ? 'bg-brand-accent/50 text-white/50 cursor-wait' :
				 urgencyColor === 'red' ? 'bg-brand-urgent hover:bg-brand-urgent/90 text-white shadow-lg shadow-brand-urgent/20 animate-pulse' : 
				 cardTheme.buttonClass}
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
