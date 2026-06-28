import { writable } from 'svelte/store';
import { client } from './client';
import { api } from '../../../../../convex/_generated/api';

export interface CurrentUserData {
	_id: string;
	email: string;
	createdAt: number;
	currentEnergyScore: number;
	energyLastUpdated: number;
	completedTour?: boolean;
}

/**
 * Reactive Convex subscription to the current user's profile document.
 * Exposes `completedTour` so the dashboard can auto-start the tour for
 * first-time visitors without an additional round-trip.
 */
export function useCurrentUser(userId: string) {
	const { subscribe, set } = writable<CurrentUserData | null>(null);

	let unsubscribe: (() => void) | null = null;

	if (userId) {
		unsubscribe = client.onUpdate(
			api.queries.getUserById,
			{ userId: userId as any },
			(data) => {
				if (data) {
					set(data as unknown as CurrentUserData);
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
