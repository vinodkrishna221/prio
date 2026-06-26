import { writable } from 'svelte/store';
import { client } from './client';
import { api } from '../../../../../convex/_generated/api';

export function useActiveTasks(userId: string) {
	if (typeof window !== 'undefined' && (window as any).__MOCK_TASKS__) {
		const { subscribe } = writable<any[]>((window as any).__MOCK_TASKS__);
		return { subscribe, destroy() {} };
	}

	const { subscribe, set } = writable<any[]>([]);

	let unsubscribe: (() => void) | null = null;

	if (userId) {
		// client.onUpdate subscribes to Convex query updates
		unsubscribe = client.onUpdate(
			api.queries.getActiveTasks,
			{ userId: userId as any },
			(tasks) => {
				set(tasks);
			}
		);
	}

	return {
		subscribe,
		destroy() {
			if (unsubscribe) {
				unsubscribe();
			}
		}
	};
}
