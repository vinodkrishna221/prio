import { writable } from 'svelte/store';
import { client } from './client';
import { api } from '../../../../../convex/_generated/api';

export function useActiveSchedules(userId: string) {
	if (typeof window !== 'undefined' && (window as any).__MOCK_SCHEDULES__) {
		const { subscribe } = writable<any[]>((window as any).__MOCK_SCHEDULES__);
		return { subscribe, destroy() {} };
	}

	const { subscribe, set } = writable<any[]>([]);

	let unsubscribe: (() => void) | null = null;

	if (userId) {
		unsubscribe = client.onUpdate(
			api.queries.getActiveSchedules,
			{ userId: userId as any },
			(schedules) => {
				set(schedules);
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
