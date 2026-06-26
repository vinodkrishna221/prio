<script lang="ts">
	// Read the Go Gateway URL from public env or fallback to local port
	import { env } from '$env/dynamic/public';
	const gatewayUrl = env.PUBLIC_GATEWAY_URL || 'http://localhost:8080';
	const loginUrl = `${gatewayUrl}/auth/google`;
</script>

<div class="relative flex min-h-screen flex-col items-center justify-center overflow-hidden bg-[#040408] px-4">
	<!-- Glowing background ambient blobs -->
	<div class="absolute -top-40 -left-40 h-96 w-96 rounded-full bg-brand-accent/20 blur-[120px]"></div>
	<div class="absolute -bottom-40 -right-40 h-96 w-96 rounded-full bg-brand-ambient/20 blur-[120px]"></div>

	<div class="z-10 w-full max-w-md">
		<!-- Glassmorphic Card -->
		<div class="relative overflow-hidden rounded-2xl border border-white/5 bg-brand-card p-8 shadow-2xl backdrop-blur-md">
			<div class="absolute inset-0 bg-gradient-to-tr from-white/[0.02] to-transparent pointer-events-none"></div>
			
			<div class="text-center">
				<div class="inline-flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-tr from-brand-accent to-brand-ambient p-0.5 shadow-lg shadow-brand-accent/20 mb-6">
					<div class="flex h-full w-full items-center justify-center rounded-[14px] bg-[#0d0d1e] text-2xl font-bold text-white font-heading">
						⚡
					</div>
				</div>

				<h1 class="text-3xl font-extrabold tracking-tight text-transparent bg-clip-text bg-gradient-to-r from-white via-slate-200 to-slate-400 font-heading mb-3">
					Last-Minute Life Saver
				</h1>
				<p class="text-sm text-brand-textMuted max-w-sm mx-auto mb-8 font-sans">
					A context-aware, proactive AI companion that pre-carves focus blocks and compiles Workspace draft resolutions before deadline anxiety hits.
				</p>

				<a
					id="login-button"
					href={loginUrl}
					class="relative group inline-flex w-full items-center justify-center gap-3 overflow-hidden rounded-xl bg-white px-5 py-3.5 text-sm font-semibold text-slate-900 transition-all duration-300 hover:bg-slate-100 hover:scale-[1.02] hover:shadow-xl hover:shadow-white/5 focus:outline-none focus:ring-2 focus:ring-brand-accent focus:ring-offset-2 focus:ring-offset-brand-bg active:scale-95"
				>
					<svg class="h-5 w-5" viewBox="0 0 24 24" fill="currentColor">
						<path
							d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
							fill="#4285F4"
						/>
						<path
							d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
							fill="#34A853"
						/>
						<path
							d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.06H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.94l2.85-2.22.81-.63z"
							fill="#FBBC05"
						/>
						<path
							d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.06l3.66 2.84c.87-2.6 3.3-4.52 6.16-4.52z"
							fill="#EA4335"
						/>
					</svg>
					<span>Sign in with Google</span>
				</a>
			</div>
		</div>

		<!-- Footer information -->
		<div class="mt-8 text-center text-xs text-brand-textMuted/50 font-sans">
			Securely managed under Least Privilege OAuth scopes and CMEK Envelope Encryption.
		</div>
	</div>
</div>
