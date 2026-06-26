import { redirect, type Handle } from '@sveltejs/kit';
import crypto from 'crypto';

export const handle: Handle = async ({ event, resolve }) => {
	const sessionId = event.cookies.get('session_id');

	if (sessionId) {
		const parts = sessionId.split(':');
		if (parts.length === 3) {
			const [userId, expStr, signature] = parts;
			const expUnix = parseInt(expStr, 10);

			if (!isNaN(expUnix) && Date.now() / 1000 < expUnix) {
				// Get shared SESSION_SECRET (ensure fallback for dev setup compatibility)
				const key = process.env.SESSION_SECRET || 'local-dev-session-signing-secret-key-32b';
				
				const hmac = crypto.createHmac('sha256', key);
				hmac.update(`${userId}:${expStr}`);
				const expectedSignature = hmac.digest('base64url');

				if (crypto.timingSafeEqual(Buffer.from(signature), Buffer.from(expectedSignature))) {
					event.locals.user = { id: userId };
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
