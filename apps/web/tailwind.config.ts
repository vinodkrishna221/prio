import type { Config } from 'tailwindcss';

export default {
	content: ['./src/**/*.{html,js,svelte,ts}'],
	theme: {
		extend: {
			colors: {
				brand: {
					bg: '#08080d',
					card: 'rgba(18, 18, 29, 0.7)',
					cardBorder: 'rgba(255, 255, 255, 0.06)',
					textMain: '#f3f4f6',
					textMuted: '#9ca3af',
					accent: '#6366f1', // Indigo 500
					accentDark: '#4f46e5', // Indigo 600
					
					// Priority levels & Statuses
					urgent: '#ef4444', // Red 500
					warn: '#f59e0b', // Amber 500
					ambient: '#3b82f6', // Blue 500
					success: '#10b981', // Emerald 500
					muted: '#4b5563' // Gray 600
				}
			},
			fontFamily: {
				sans: ['Inter', 'ui-sans-serif', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'Helvetica Neue', 'Arial', 'sans-serif'],
				heading: ['Outfit', 'sans-serif']
			},
			backdropBlur: {
				xs: '2px'
			}
		}
	},
	plugins: []
} satisfies Config;
