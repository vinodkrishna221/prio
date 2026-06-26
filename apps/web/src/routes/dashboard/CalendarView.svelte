<script lang="ts">
	// Props definition for Svelte 5
	let { schedules = [] } = $props<{
		schedules: any[];
	}>();

	// Format timestamp to readable time string
	function formatTime(timestamp: number): string {
		return new Date(timestamp).toLocaleTimeString([], {
			hour: '2-digit',
			minute: '2-digit',
			hour12: true
		});
	}

	// Format timestamp to day representation
	function formatDay(timestamp: number): string {
		return new Date(timestamp).toLocaleDateString([], {
			weekday: 'short',
			month: 'short',
			day: 'numeric'
		});
	}

	// Sort schedules chronologically by start time
	let sortedSchedules = $derived(
		[...schedules].sort((a, b) => a.startTime - b.startTime)
	);
</script>

<div class="rounded-2xl border border-white/5 bg-brand-card p-6 shadow-xl backdrop-blur-md">
	<div class="flex items-center justify-between border-b border-white/5 pb-4 mb-6">
		<div>
			<h2 class="text-xl font-bold tracking-tight text-brand-textMain font-heading">
				Micro-Gap Focus Calendar
			</h2>
			<p class="text-xs text-brand-textMuted mt-0.5">
				Proactively carved focus blocks & reclaimed slots
			</p>
		</div>
		<span class="inline-flex h-8 w-8 items-center justify-center rounded-xl bg-brand-accent/10 text-brand-accent text-sm">
			📅
		</span>
	</div>

	{#if sortedSchedules.length === 0}
		<div class="flex flex-col items-center justify-center py-16 text-center border border-dashed border-white/5 rounded-2xl bg-[#090911]/30">
			<span class="text-3xl mb-3 opacity-30">⏳</span>
			<h4 class="text-sm font-semibold text-brand-textMain">No Active Focus Slots</h4>
			<p class="text-xs text-brand-textMuted max-w-[200px] mt-1">
				AI will auto-allocate blocks in your next calendar micro-gaps.
			</p>
		</div>
	{:else}
		<div class="space-y-4 relative before:absolute before:left-[17px] before:top-2 before:bottom-2 before:w-[1px] before:bg-white/5">
			{#each sortedSchedules as slot (slot._id)}
				<div class="flex gap-4 relative group">
					<!-- Timeline bullet -->
					<div class="z-10 flex h-9 w-9 items-center justify-center rounded-full border bg-brand-bg transition-all duration-300
						{slot.status === 'COMMITTED' ? 'border-brand-success/40 text-brand-success ring-4 ring-brand-success/10' :
						 slot.status === 'DISSOLVED' ? 'border-white/5 text-brand-textMuted bg-neutral-900/50' :
						 'border-brand-warn/40 text-brand-warn ring-4 ring-brand-warn/10 animate-pulse'}"
					>
						{#if slot.status === 'COMMITTED'}
							🔒
						{:else if slot.status === 'DISSOLVED'}
							✨
						{:else if slot.allocationType === 'GHOST_BLOCK'}
							👻
						{:else}
							⏳
						{/if}
					</div>

					<!-- Block Body -->
					<div class="flex-1 rounded-xl p-4 transition-all duration-300 border backdrop-blur-xs
						{slot.status === 'COMMITTED' ? 'bg-brand-success/5 border-brand-success/20 text-brand-textMain' :
						 slot.status === 'DISSOLVED' ? 'bg-white/[0.01] border-white/5 text-brand-textMuted/50 line-through opacity-50' :
						 'bg-brand-warn/5 border-brand-warn/20 border-dashed text-brand-textMain hover:border-brand-warn/40'}"
					>
						<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-1 mb-2">
							<span class="text-xs font-mono font-medium text-brand-textMuted flex items-center gap-1.5">
								<span>{formatDay(slot.startTime)}</span>
								<span class="h-1 w-1 rounded-full bg-white/10"></span>
								<span class="text-brand-textMain">{formatTime(slot.startTime)} – {formatTime(slot.endTime)}</span>
							</span>

							<!-- Status Badge -->
							<span class="inline-flex max-w-fit items-center rounded-md px-1.5 py-0.5 text-[10px] font-medium tracking-wide uppercase
								{slot.status === 'COMMITTED' ? 'bg-brand-success/15 text-brand-success' :
								 slot.status === 'DISSOLVED' ? 'bg-white/5 text-brand-textMuted' :
								 'bg-brand-warn/15 text-brand-warn'}"
							>
								{slot.status === 'COMMITTED' ? 'Committed' : slot.status === 'DISSOLVED' ? 'Freed' : 'Reserved'}
							</span>
						</div>

						<h4 class="text-sm font-semibold tracking-tight">
							{#if slot.status === 'DISSOLVED'}
								Reclaimed open time
							{:else if slot.allocationType === 'GHOST_BLOCK'}
								Tentative Focus Block (Ghost)
							{:else}
								Micro-Gap Allocation
							{/if}
						</h4>
						
						<p class="text-xs mt-1 leading-relaxed
							{slot.status === 'DISSOLVED' ? 'text-brand-textMuted/30' : 'text-brand-textMuted'}"
						>
							{#if slot.status === 'DISSOLVED'}
								Task was completed early! Free calendar slot returned to your open pool.
							{:else if slot.status === 'COMMITTED'}
								Confirmed focus block for tasks requiring consecutive concentration. Locked in Google Calendar.
							{:else}
								Reserved placeholder block. Self-dissolves automatically if task is completed or deleted.
							{/if}
						</p>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
