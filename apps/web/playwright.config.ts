import { defineConfig } from '@playwright/test';

export default defineConfig({
	webServer: { 
		command: 'pnpm run build && pnpm run preview', 
		port: 7777,
		reuseExistingServer: true 
	},
	testMatch: ['**/*.e2e.ts', 'tests/**/*.spec.ts']
});
