import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';
import { sveltekit } from '@sveltejs/kit/vite';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit()
	],
	resolve: {
		alias: {
			'convex/server': path.resolve(__dirname, './node_modules/convex/dist/esm/server/index.js'),
			'convex/values': path.resolve(__dirname, './node_modules/convex/dist/esm/values/index.js')
		}
	},
	server: {
		fs: {
			allow: ['../..']
		}
	},
	preview: {
		port: 7777,
		strictPort: true
	},
	test: {
		expect: { requireAssertions: true },
		projects: [
			{
				extends: './vite.config.ts',
				test: {
					name: 'server',
					environment: 'node',
					include: ['src/**/*.{test,spec}.{js,ts}'],
					exclude: ['src/**/*.svelte.{test,spec}.{js,ts}']
				}
			}
		]
	}
});
