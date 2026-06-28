import { writable } from 'svelte/store';
import { client } from './client';
import { api } from '../../../../../convex/_generated/api';

export function useLatestGenome(userId: string) {
	const { subscribe, set } = writable<any | null>(null);

	let unsubscribe: (() => void) | null = null;

	if (userId) {
		unsubscribe = client.onUpdate(
			api.queries.getLatestGenome,
			{ userId: userId as any },
			(genome) => {
				set(genome);
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
