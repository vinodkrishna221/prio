import { writable } from 'svelte/store';
import { client } from './client';
import { api } from '../../../../../convex/_generated/api';

export interface FrictionSavedData {
	completed: number;
	active: number;
	total: number;
}

export function useFrictionSaved(userId: string) {
	if (typeof window !== 'undefined' && (window as any).__MOCK_FRICTION_SAVED__) {
		const { subscribe } = writable<FrictionSavedData>((window as any).__MOCK_FRICTION_SAVED__);
		return { subscribe, destroy() {} };
	}

	const { subscribe, set } = writable<FrictionSavedData>({ completed: 0, active: 0, total: 0 });

	let unsubscribe: (() => void) | null = null;

	if (userId) {
		unsubscribe = client.onUpdate(
			api.queries.getFrictionSaved,
			{ userId: userId as any },
			(data) => {
				if (data) {
					set(data);
				}
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
