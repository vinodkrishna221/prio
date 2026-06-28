<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { env } from '$env/dynamic/public';
	import { useActiveTasks } from '$lib/convex/useActiveTasks';
	import { useActiveSchedules } from '$lib/convex/useActiveSchedules';
	import { useFrictionSaved } from '$lib/convex/useFrictionSaved';
	import { useCurrentUser } from '$lib/convex/useCurrentUser';
	import { client } from '$lib/convex/client';
	import { api } from '../../../../../convex/_generated/api';
	import ActionCard from '$lib/components/cards/ActionCard.svelte';
	import CalendarView from './CalendarView.svelte';
	import DashboardTour from '$lib/components/onboarding/DashboardTour.svelte';

	let { data } = $props<{ data: { user: { id: string }; sseToken?: string } }>();
	let userId = $derived(data.user?.id || '');
	let sseToken = $derived(data.sseToken || '');

	const gatewayUrl = env.PUBLIC_GATEWAY_URL || 'http://localhost:8080';

	// Subscriptions to Convex reactive stores via Svelte 5 $effect
	let tasksList = $state<any[]>([]);
	let schedulesList = $state<any[]>([]);
	let userProfile = $state<{ completedTour?: boolean } | null>(null);
	let frictionSaved = $state({ completed: 0, active: 0, total: 0 });

	$effect(() => {
		if (userId) {
			const tasksStore = useActiveTasks(userId);
			const schedulesStore = useActiveSchedules(userId);
			const frictionStore = useFrictionSaved(userId);
			const userStore = useCurrentUser(userId);
			
			const unsubTasks = tasksStore.subscribe(val => {
				tasksList = val;
			});
			const unsubSchedules = schedulesStore.subscribe(val => {
				schedulesList = val;
			});
			const unsubFriction = frictionStore.subscribe(val => {
				frictionSaved = val;
			});
			const unsubUser = userStore.subscribe(val => {
				userProfile = val;
			});

			return () => {
				unsubTasks();
				unsubSchedules();
				unsubFriction();
				unsubUser();
				tasksStore.destroy();
				schedulesStore.destroy();
				frictionStore.destroy();
				userStore.destroy();
			};
		}
	});

	// Local state
	let energyScore = $state(5);
	let isSyncing = $state(false);
	let syncMessage = $state('');
	let sseConnected = $state(false);

	// ─── Onboarding Tour ─────────────────────────────────────────────────────────
	let tourRef = $state<ReturnType<typeof DashboardTour> | null>(null);
	let tourStarted = $state(false);

	// Watch userProfile — once loaded, auto-start tour for first-time visitors
	$effect(() => {
		if (!tourStarted && userProfile !== null && !userProfile.completedTour) {
			tourStarted = true;
			// 800ms delay so the page fully paints before the spotlight appears
			setTimeout(() => {
				tourRef?.startTour();
			}, 800);
		}
	});

	// Called by the tour component when the user finishes or skips
	async function handleTourComplete() {
		try {
			await client.mutation(api.mutations.completeUserTour, { userId: userId as any });
		} catch (err) {
			slogError(err);
		}
	}

	interface Toast {
		id: string;
		title: string;
		message: string;
		type: 'triage' | 'due' | 'success' | 'error';
	}
	let toasts = $state<Toast[]>([]);

	let sseSource: EventSource | null = null;

	function addToast(title: string, message: string, type: Toast['type'] = 'success') {
		const id = Math.random().toString(36).substring(2, 9);
		toasts = [...toasts, { id, title, message, type }];
		setTimeout(() => {
			removeToast(id);
		}, 6000);
	}

	function removeToast(id: string) {
		toasts = toasts.filter((t) => t.id !== id);
	}

	// Adjust Energy Score locally & sync back to Go Gateway via SvelteKit proxy
	async function handleEnergyChange(newScore: number) {
		energyScore = newScore;
		try {
			const res = await fetch('/proxy/api/user/energy-state', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ score: newScore })
			});
			if (!res.ok) throw new Error('Failed to update energy state');
			addToast('Circadian rhythm updated', `Energy set to ${newScore}/10. Task ranking modified.`, 'success');
		} catch (err) {
			slogError(err);
			addToast('Sync error', 'Failed to update biometric state.', 'error');
		}
	}

	// Trigger manual Google Tasks/Gmail Watch Sync via SvelteKit proxy
	async function triggerSync() {
		if (isSyncing) return;
		isSyncing = true;
		syncMessage = 'Syncing Google Workspace...';
		try {
			// Trigger tasks sync through the server-side proxy
			const resTasks = await fetch('/proxy/tasks/sync', { method: 'POST' });
			if (!resTasks.ok) throw new Error('Tasks sync failed');

			// Trigger watch renew — may return a warning in dev (Pub/Sub topic not set up yet)
			const resWatch = await fetch('/proxy/v1/users/me/watch', { method: 'POST' });
			if (!resWatch.ok) throw new Error('Gmail watch renewal failed');

			addToast('Workspace Synced', 'Google Tasks and Gmail watches synchronized successfully.', 'success');
		} catch (err: any) {
			slogError(err);
			addToast('Sync Failed', err.message || 'Workspace sync failed', 'error');
		} finally {
			isSyncing = false;
			syncMessage = '';
		}
	}

	// Client-side Logout by removing session cookie
	function logout() {
		// Cookies are shared across localhost ports, so we delete it
		document.cookie = 'session_id=; path=/; expires=Thu, 01 Jan 1970 00:00:00 UTC;';
		window.location.href = '/login';
	}

	onMount(() => {
		// Establish SSE stream connection directly to Go Gateway.
		// In production the browser can't send the session cookie to a different domain,
		// so we use a short-lived signed token (generated server-side in +page.server.ts)
		// appended as ?token= instead. Falls back to cookie auth in local dev.
		const sseUrl = sseToken
			? `${gatewayUrl}/v1/events?token=${encodeURIComponent(sseToken)}`
			: `${gatewayUrl}/v1/events`;

		try {
			sseSource = new EventSource(sseUrl, { withCredentials: !sseToken });

			sseSource.onopen = () => {
				sseConnected = true;
			};

			sseSource.onerror = (e) => {
				sseConnected = false;
				slogError('SSE Connection Error: ' + JSON.stringify(e));
			};

			// Handle events streamed from Go Gateway
			sseSource.addEventListener('TASK_TRIAGED', (e: any) => {
				try {
					const payload = JSON.parse(e.data);
					addToast(
						'⚡ Task Triaged',
						`"${payload.title}" prioritized at score ${payload.priorityScore}%. Pre-compiled draft ready.`,
						'triage'
					);
				} catch (err) {
					slogError(err);
				}
			});

			sseSource.addEventListener('MICRO_TASK_DUE', (e: any) => {
				try {
					const payload = JSON.parse(e.data);
					addToast(
						'⏰ Task Slot Commencing',
						`Focus window ready for task: "${payload.taskTitle}". Click to execute.`,
						'due'
					);
				} catch (err) {
					slogError(err);
				}
			});
		} catch (err) {
			slogError(err);
		}

		// Initial load of current energy score from database if available.
		// For the MVP, we default it to 5.
	});

	onDestroy(() => {
		if (sseSource) sseSource.close();
	});

	function slogError(msg: any) {
		const PUBLIC_ENV = env.PUBLIC_ENV || 'development';
		if (PUBLIC_ENV === 'development') {
			console.error(msg);
		}
	}
</script>

<div class="relative min-h-screen bg-[#040408] text-brand-textMain">
	<!-- Background radial glows -->
	<div class="absolute top-0 left-1/4 h-[500px] w-[500px] rounded-full bg-brand-accent/5 blur-[150px] pointer-events-none"></div>
	<div class="absolute bottom-10 right-1/4 h-[500px] w-[500px] rounded-full bg-brand-ambient/5 blur-[150px] pointer-events-none"></div>

	<!-- Top Navigation Header -->
	<header class="sticky top-0 z-40 w-full border-b border-white/5 bg-brand-bg/85 backdrop-blur-md">
		<div class="mx-auto flex max-w-7xl items-center justify-between px-6 py-4">
			<div class="flex items-center gap-3">
				<span class="text-2xl">⚡</span>
				<div>
					<h1 class="text-lg font-bold tracking-tight text-white font-heading">
						Last-Minute Life Saver
					</h1>
					<div class="flex items-center gap-1.5 mt-0.5">
						<span class="h-1.5 w-1.5 rounded-full {sseConnected ? 'bg-brand-success animate-pulse' : 'bg-brand-urgent'}"></span>
						<span class="text-[10px] text-brand-textMuted tracking-wider uppercase font-medium">
							{sseConnected ? 'Connected' : 'Disconnected'}
						</span>
					</div>
				</div>
			</div>

			<div class="flex items-center gap-4">
				<button
					id="sync-workspace-btn"
					onclick={triggerSync}
					disabled={isSyncing}
					class="inline-flex items-center gap-2 rounded-xl bg-white/5 border border-white/5 hover:bg-white/10 text-xs font-semibold px-4 py-2.5 transition-all cursor-pointer disabled:opacity-50"
				>
					{#if isSyncing}
						<svg class="animate-spin h-3.5 w-3.5 text-white" fill="none" viewBox="0 0 24 24">
							<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
							<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
						</svg>
						<span>Syncing...</span>
					{:else}
						<span>🔄 Sync Workspace</span>
					{/if}
				</button>

				<button
					id="logout-btn"
					onclick={logout}
					class="text-xs font-medium text-brand-textMuted hover:text-white transition-colors cursor-pointer"
				>
					Sign Out
				</button>
			</div>
		</div>
	</header>

	<main class="mx-auto max-w-7xl px-6 py-8">
		<!-- Top Dashboard Summary Grid -->
		<div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
			<!-- Biometrics widget card -->
			<div class="rounded-2xl border border-white/5 bg-brand-card p-6 shadow-lg backdrop-blur-md md:col-span-2 flex flex-col justify-between">
				<div>
					<h3 class="text-sm font-semibold text-brand-textMuted uppercase tracking-wider mb-2">
						Circadian Energy Profiler
					</h3>
					<p class="text-xs text-brand-textMuted leading-relaxed max-w-xl">
						Adjust your cognitive state score. Low energy score prompts the scheduler to recommend low-effort admin tasks (e.g. paying utilities). High score unlocks complex, strategic drafts.
					</p>
				</div>
				<div class="flex items-center gap-6 mt-6">
					<div class="flex-1">
						<input
							id="energy-slider"
							type="range"
							min="1"
							max="10"
							bind:value={energyScore}
							onchange={(e) => handleEnergyChange(parseInt(e.currentTarget.value))}
							class="w-full h-1.5 bg-white/10 rounded-lg appearance-none cursor-pointer accent-brand-accent focus:outline-none"
						/>
						<div class="flex justify-between text-[10px] text-brand-textMuted font-mono mt-2">
							<span>1 Exceeded</span>
							<span>5 Mid</span>
							<span>10 Peak</span>
						</div>
					</div>
					<div class="flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-tr from-brand-accent/20 to-brand-ambient/20 border border-brand-accent/30 text-xl font-bold font-heading text-white">
						{energyScore}
					</div>
				</div>
			</div>

			<!-- Quick stats card —— id used by the onboarding tour spotlight -->
			<div id="friction-stat-card" class="rounded-2xl border border-white/5 bg-brand-card p-6 shadow-lg backdrop-blur-md flex flex-col justify-between">
				<div>
					<h3 class="text-sm font-semibold text-brand-textMuted uppercase tracking-wider mb-2">
						Friction Reduction Index
					</h3>
					<p class="text-xs text-brand-textMuted leading-relaxed">
						Estimated time saved by delegating compose tasks and gap reservations to the agent.
					</p>
				</div>
				<div class="mt-4">
					<div class="text-4xl font-extrabold text-transparent bg-clip-text bg-gradient-to-r from-brand-success via-emerald-400 to-teal-400 font-heading">
						⚡ {frictionSaved.total} min
					</div>
					<span class="text-[10px] text-brand-textMuted/70 font-mono mt-1 block">
						{frictionSaved.completed}m saved • {frictionSaved.active}m queued
					</span>
				</div>
			</div>
		</div>

		<!-- Main Split Area -->
		<div class="grid grid-cols-1 lg:grid-cols-12 gap-8 items-start">
			<!-- Action Card Queue (Left side) — id used by the onboarding tour spotlight -->
			<section id="actions-queue" class="lg:col-span-7 flex flex-col gap-6">
				<div class="flex items-center justify-between">
					<h2 class="text-xl font-bold tracking-tight text-white font-heading">
						1-Tap Actions Queue ({tasksList.length})
					</h2>
					<span class="text-xs text-brand-textMuted">Sorted by Priority Score</span>
				</div>

				{#if tasksList.length === 0}
					<div class="flex flex-col items-center justify-center py-20 text-center border border-dashed border-white/5 rounded-2xl bg-brand-card">
						<span class="text-4xl mb-4">🏆</span>
						<h3 class="text-base font-semibold text-brand-textMain">Inbox Zero Achieved</h3>
						<p class="text-xs text-brand-textMuted max-w-xs mt-1">
							All high-priority tasks have been resolved. Your calendar and Gmail draft boxes are completely clear.
						</p>
					</div>
				{:else}
					<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
						{#each tasksList as task (task._id)}
							<ActionCard {task} />
						{/each}
					</div>
				{/if}
			</section>

			<!-- Calendar View (Right side) — id used by the onboarding tour spotlight -->
			<section id="calendar-view" class="lg:col-span-5">
				<CalendarView schedules={schedulesList} />
			</section>
		</div>
	</main>

	<!-- ─── Onboarding Tour (renders as a fixed overlay) ────────────────────── -->
	<DashboardTour bind:this={tourRef} {userId} onComplete={handleTourComplete} />

	<!-- Reactive Glowing Toasts Container -->
	<div class="fixed bottom-6 right-6 z-50 flex flex-col gap-3 w-full max-w-sm px-4 sm:px-0">
		{#each toasts as toast (toast.id)}
			<div
				class="relative overflow-hidden rounded-xl border p-4 shadow-2xl backdrop-blur-md transition-all duration-300 flex justify-between gap-3 animate-slide-in
					{toast.type === 'triage' ? 'bg-[#0f0e26]/90 border-brand-accent/40 shadow-brand-accent/10' :
					 toast.type === 'due' ? 'bg-[#261f0e]/90 border-brand-warn/40 shadow-brand-warn/10 animate-bounce' :
					 toast.type === 'error' ? 'bg-red-950/90 border-brand-urgent/40 shadow-brand-urgent/10' :
					 'bg-brand-card border-brand-success/40 shadow-brand-success/10'}"
			>
				<div class="flex-1">
					<div class="flex items-center gap-1.5 text-xs font-bold text-white font-heading">
						<span>
							{toast.type === 'triage' ? '⚡ Agent Triage Event' :
							 toast.type === 'due' ? '⏰ Focus Block Commencing' :
							 toast.type === 'error' ? '⚠️ Sync Failure' :
							 '✅ Success'}
						</span>
					</div>
					<h4 class="text-xs font-semibold text-slate-100 mt-1">{toast.title}</h4>
					<p class="text-[11px] text-brand-textMuted mt-0.5 leading-relaxed">{toast.message}</p>
				</div>
				<button
					onclick={() => removeToast(toast.id)}
					class="text-brand-textMuted hover:text-white text-xs font-bold self-start cursor-pointer px-1"
				>
					×
				</button>
			</div>
		{/each}
	</div>
</div>

<style>
	@keyframes -global-slide-in {
		0% {
			transform: translateY(1rem);
			opacity: 0;
		}
		100% {
			transform: translateY(0);
			opacity: 1;
		}
	}
	.animate-slide-in {
		animation: slide-in 0.3s cubic-bezier(0.16, 1, 0.3, 1) forwards;
	}
</style>
