import type { PageServerLoad } from './$types';
import { env } from '$env/dynamic/private';
import crypto from 'crypto';

/**
 * Generate a short-lived SSE token for the dashboard page.
 *
 * The token is passed to the client and appended as ?token= when opening the
 * EventSource connection to go-gateway. Because SSE cannot be proxied through
 * Vercel serverless functions (10s timeout), the browser connects directly to
 * go-gateway with this token as an alternative to the session cookie.
 *
 * Token format: userId:expUnixSecs:hmac-sha256-base64url
 * Expiry: 5 minutes — enough time to establish the connection; SSE itself is
 * then kept alive by go-gateway directly.
 */
export const load: PageServerLoad = ({ locals }) => {
	const userId = locals.user?.id;
	if (!userId) return {};

	const internalSecret = env.INTERNAL_API_SECRET || '';
	if (!internalSecret) {
		// In local dev INTERNAL_API_SECRET may not be set; SSE will fall back to
		// the session cookie which works fine on localhost.
		return {};
	}

	const expUnix = Math.floor(Date.now() / 1000) + 300; // 5 minutes
	const payload = `${userId}:${expUnix}`;
	const sig = crypto.createHmac('sha256', internalSecret).update(payload).digest('base64url');
	const sseToken = `${payload}:${sig}`;

	return { sseToken };
};