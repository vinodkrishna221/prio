import { redirect, type RequestHandler } from '@sveltejs/kit';

export const GET: RequestHandler = async ({ url, cookies }) => {
	const sessionId = url.searchParams.get('session_id');
	if (sessionId) {
		cookies.set('session_id', sessionId, {
			path: '/',
			httpOnly: true,
			secure: true,
			sameSite: 'lax',
			maxAge: 60 * 60 * 24 // 1 day
		});
	}
	throw redirect(303, '/dashboard');
};
