import { env } from '$env/dynamic/private';
import { env as publicEnv } from '$env/dynamic/public';
import { error, type RequestHandler } from '@sveltejs/kit';

/**
 * Catch-all proxy from SvelteKit -> go-gateway.
 *
 * All dashboard client-side fetch() calls go to /proxy/<path> instead of
 * directly to PUBLIC_GATEWAY_URL. The browser cookie (session_id on *.vercel.app)
 * is never sent to go-gateway because it lives on a different domain.
 * Instead, this server-side route:
 *  1. Validates the session via event.locals.user (already done by hooks.server.ts)
 *  2. Forwards the request to go-gateway with X-Internal-Auth + X-User-Id headers
 *  3. Streams the response body back to the browser
 *
 * SSE (/v1/events) is intentionally NOT proxied here - it goes direct to
 * go-gateway with a short-lived token to avoid Vercel serverless timeouts.
 */
async function proxyToGateway(event: Parameters<RequestHandler>[0]): Promise<Response> {
	if (!event.locals.user) {
		throw error(401, 'Unauthorized');
	}

	const gatewayUrl =
		env.GATEWAY_URL || publicEnv.PUBLIC_GATEWAY_URL || 'http://localhost:8080';
	const internalSecret = env.INTERNAL_API_SECRET || '';

	const path = event.params.path ?? '';
	const search = event.url.search;
	const targetUrl = `${gatewayUrl.replace(/\/$/, '')}/${path}${search}`;

	const upstreamHeaders = new Headers();
	const contentType = event.request.headers.get('content-type');
	if (contentType) {
		upstreamHeaders.set('content-type', contentType);
	}
	upstreamHeaders.set('x-internal-auth', internalSecret);
	upstreamHeaders.set('x-user-id', event.locals.user.id);

	const hasBody = !['GET', 'HEAD'].includes(event.request.method);
	const body = hasBody ? await event.request.arrayBuffer() : undefined;

	const upstreamResponse = await fetch(targetUrl, {
		method: event.request.method,
		headers: upstreamHeaders,
		body
	});

	const responseHeaders = new Headers();
	const ct = upstreamResponse.headers.get('content-type');
	if (ct) responseHeaders.set('content-type', ct);

	return new Response(upstreamResponse.body, {
		status: upstreamResponse.status,
		headers: responseHeaders
	});
}

export const GET: RequestHandler = (event) => proxyToGateway(event);
export const POST: RequestHandler = (event) => proxyToGateway(event);
export const PUT: RequestHandler = (event) => proxyToGateway(event);
export const PATCH: RequestHandler = (event) => proxyToGateway(event);
export const DELETE: RequestHandler = (event) => proxyToGateway(event);