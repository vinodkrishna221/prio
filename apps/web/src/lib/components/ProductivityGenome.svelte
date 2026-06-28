<script lang="ts">
	import { slide, fade } from 'svelte/transition';

	let { userId, latestGenome, onGenerate } = $props<{
		userId: string;
		latestGenome: any | null;
		onGenerate: () => Promise<void>;
	}>();

	let isGenerating = $state(false);

	async function handleGenerate() {
		if (isGenerating) return;
		isGenerating = true;
		try {
			await onGenerate();
		} finally {
			isGenerating = false;
		}
	}

	// Helper for categorizing colors and icons
	function getCategoryStyles(category: string) {
		switch (category) {
			case 'ENERGY':
				return {
					bg: 'from-amber-500/10 to-orange-500/10 border-orange-500/20',
					text: 'text-orange-400',
					glow: 'shadow-orange-500/5',
					icon: `<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>`
				};
			case 'FRICTION':
				return {
					bg: 'from-emerald-500/10 to-teal-500/10 border-emerald-500/20',
					text: 'text-emerald-400',
					glow: 'shadow-emerald-500/5',
					icon: `<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/></svg>`
				};
			case 'SCHEDULE':
			default:
				return {
					bg: 'from-indigo-500/10 to-blue-500/10 border-indigo-500/20',
					text: 'text-indigo-400',
					glow: 'shadow-indigo-500/5',
					icon: `<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>`
				};
		}
	}
</script>

<div class="rounded-2xl border border-white/5 bg-[#080812]/60 backdrop-blur-md p-6 shadow-2xl relative overflow-hidden transition-all duration-300 hover:border-white/10">
	<!-- Top Header -->
	<div class="flex items-center justify-between mb-6 relative z-10">
		<div>
			<h2 class="text-lg font-bold tracking-tight text-white font-heading">
				Productivity Genome <span class="text-brand-accent">.</span>
			</h2>
			<p class="text-xs text-brand-textMuted mt-0.5">
				Retrospective weekly analysis and machine-learned preference adjustment
			</p>
		</div>
		<button
			onclick={handleGenerate}
			disabled={isGenerating}
			class="inline-flex items-center gap-2 rounded-xl bg-gradient-to-r from-brand-accent to-brand-ambient hover:opacity-90 text-white text-xs font-semibold px-4 py-2.5 transition-all cursor-pointer disabled:opacity-50 shadow-lg shadow-brand-accent/15"
		>
			{#if isGenerating}
				<svg class="animate-spin h-3.5 w-3.5 text-white" fill="none" viewBox="0 0 24 24">
					<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
					<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
				</svg>
				<span>Analyzing Genome...</span>
			{:else}
				<span>🧬 Analyze Weekly Genome</span>
			{/if}
		</button>
	</div>

	<!-- Main Body -->
	{#if !latestGenome}
		<div class="flex flex-col items-center justify-center py-12 text-center border border-dashed border-white/5 rounded-xl bg-white/2" in:fade>
			<span class="text-3xl mb-3 animate-pulse">📊</span>
			<h3 class="text-sm font-semibold text-brand-textMain">No Genome Compiled Yet</h3>
			<p class="text-xs text-brand-textMuted max-w-xs mt-1">
				Generate your first genome retrospective to analyze your week's activity and adjust scheduling behavior.
			</p>
		</div>
	{:else}
		<div class="grid grid-cols-1 md:grid-cols-12 gap-6" in:fade>
			<!-- Risk Score Arc & Peak Hours (Left) -->
			<div class="md:col-span-4 flex flex-col items-center justify-center p-6 rounded-xl bg-white/[0.02] border border-white/5 shadow-inner">
				<h4 class="text-xs font-semibold uppercase tracking-wider text-brand-textMuted mb-4">
					Deadline Risk Score
				</h4>
				
				<!-- Arc SVG -->
				<div class="relative w-36 h-36 flex items-center justify-center">
					<svg class="w-full h-full transform -rotate-90" viewBox="0 0 100 100">
						<!-- Background track -->
						<circle
							cx="50"
							cy="50"
							r="40"
							stroke="#1a1a2e"
							stroke-width="8"
							fill="transparent"
						/>
						<!-- Progress path -->
						<circle
							cx="50"
							cy="50"
							r="40"
							stroke="url(#genome-grad)"
							stroke-width="8"
							stroke-dasharray="251.2"
							stroke-dashoffset={251.2 - (251.2 * latestGenome.deadlineRiskScore) / 100}
							stroke-linecap="round"
							fill="transparent"
						/>
						<defs>
							<linearGradient id="genome-grad" x1="0%" y1="0%" x2="100%" y2="100%">
								<stop offset="0%" stop-color="#3b82f6" />
								<stop offset="60%" stop-color="#6366f1" />
								<stop offset="100%" stop-color="#ec4899" />
							</linearGradient>
						</defs>
					</svg>
					<div class="absolute flex flex-col items-center text-center">
						<span class="text-3xl font-extrabold text-white font-heading">
							{latestGenome.deadlineRiskScore}%
						</span>
						<span class="text-[9px] text-brand-textMuted font-mono uppercase tracking-widest mt-0.5">
							{latestGenome.deadlineRiskScore < 30 ? 'Stable' : latestGenome.deadlineRiskScore < 70 ? 'Moderate' : 'High Risk'}
						</span>
					</div>
				</div>

				<div class="mt-6 w-full">
					<h5 class="text-[10px] font-bold text-brand-textMuted uppercase tracking-wider text-center mb-2">
						Peak Productivity Windows
					</h5>
					<div class="flex flex-wrap gap-1.5 justify-center">
						{#each latestGenome.peakHours as hour}
							<span class="text-[10px] font-semibold text-brand-accent bg-brand-accent/10 border border-brand-accent/20 rounded-full px-2.5 py-0.5 shadow-sm shadow-brand-accent/5">
								{hour}
							</span>
						{/each}
					</div>
				</div>
			</div>

			<!-- AI Insights List (Right) -->
			<div class="md:col-span-8 flex flex-col gap-3">
				<h4 class="text-xs font-semibold uppercase tracking-wider text-brand-textMuted mb-1">
					Genome Insights
				</h4>
				<div class="flex flex-col gap-3 max-h-[280px] overflow-y-auto pr-1">
					{#each latestGenome.insights as insight}
						{@const style = getCategoryStyles(insight.category)}
						<div class="rounded-xl border p-4 bg-gradient-to-r {style.bg} transition-all duration-300 flex items-start gap-3 shadow-md {style.glow} hover:scale-[1.01]">
							<div class="p-2 rounded-lg bg-white/5 border border-white/5 {style.text}">
								{@html style.icon}
							</div>
							<div class="flex-1">
								<div class="flex items-center justify-between">
									<h5 class="text-xs font-bold text-white font-heading">{insight.title}</h5>
									<span class="text-[9px] font-mono tracking-widest uppercase {style.text} bg-white/5 border border-white/5 rounded px-1.5 font-semibold">
										{insight.category}
									</span>
								</div>
								<p class="text-[11px] text-brand-textMuted mt-1 leading-relaxed">{insight.description}</p>
								<div class="flex items-center gap-1 mt-2 text-[10px] font-bold {style.text}">
									<span>⚡ Impact:</span>
									<span>{insight.impact}</span>
								</div>
							</div>
						</div>
					{/each}
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	/* Custom scrollbar styling for insights list */
	div::-webkit-scrollbar {
		width: 4px;
	}
	div::-webkit-scrollbar-track {
		background: rgba(255, 255, 255, 0.02);
		border-radius: 4px;
	}
	div::-webkit-scrollbar-thumb {
		background: rgba(255, 255, 255, 0.1);
		border-radius: 4px;
	}
	div::-webkit-scrollbar-thumb:hover {
		background: rgba(255, 255, 255, 0.2);
	}
</style>
