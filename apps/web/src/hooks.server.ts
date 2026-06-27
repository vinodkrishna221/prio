import { redirect, type Handle } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import crypto from 'crypto';

export const handle: Handle = async ({ event, resolve }) => {
	const sessionId = event.cookies.get('session_id');

	if (sessionId) {
		const parts = sessionId.split(':');
		if (parts.length === 3) {
			const [userId, expStr, signature] = parts;
			const expUnix = parseInt(expStr, 10);

			if (!isNaN(expUnix) && Date.now() / 1000 < expUnix) {
				// Use SvelteKit's $env/dynamic/private for reliable env var access
				// across all deployment targets (Vercel, Node, etc.)
				const key = env.SESSION_SECRET || '934e3c2b960f84b81b181bee6f4a40f29ee7f0ae0d22daec5e1fa137b4f02810';

				const hmac = crypto.createHmac('sha256', key);
				hmac.update(`${userId}:${expStr}`);
				const expectedSignature = hmac.digest('base64url');

				try {
					const sigBuf = Buffer.from(signature);
					const expectedBuf = Buffer.from(expectedSignature);

					// timingSafeEqual throws if buffers differ in length;
					// guard against that to prevent silent auth failures.
					if (sigBuf.length === expectedBuf.length &&
						crypto.timingSafeEqual(sigBuf, expectedBuf)) {
						event.locals.user = { id: userId };
					}
				} catch {
					// Signature length mismatch or other crypto error — treat as invalid session
				}
			}
		}
	}

	const pathname = event.url.pathname;

	// Route guards
	if (pathname.startsWith('/dashboard')) {
		if (!event.locals.user) {
			throw redirect(303, '/login');
		}
	}

	if (pathname === '/login' || pathname === '/') {
		if (event.locals.user) {
			throw redirect(303, '/dashboard');
		}
	}

	const response = await resolve(event);
	return response;
};
