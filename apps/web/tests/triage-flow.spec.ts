import { test, expect } from '@playwright/test';
import crypto from 'crypto';

const SESSION_SECRET = process.env.SESSION_SECRET || 'local-dev-session-signing-secret-key-32b';

function createSignedCookie(userId: string): string {
	const expiration = Math.floor(Date.now() / 1000) + 24 * 60 * 60; // 24 hours
	const payload = `${userId}:${expiration}`;
	const hmac = crypto.createHmac('sha256', SESSION_SECRET);
	hmac.update(payload);
	const signature = hmac.digest('base64url');
	return `${payload}:${signature}`;
}

const mockTasks = [
	{
		_id: 'task-123456',
		userId: 'mock-user-id',
		title: 'Review Q3 Client Agreement',
		source: 'GMAIL',
		status: 'ACTIVE',
		priorityScore: 92,
		durationMinutes: 15,
		dueAt: Date.now() + 1.5 * 60 * 60 * 1000, // Due in 1.5 hours (Urgent: Red)
		actionCard: {
			actionType: 'GMAIL_DRAFT',
			savesMinutes: 15,
			draftId: 'draft-98765',
			payloadJson: JSON.stringify({
				to: 'client@acme.com',
				body: 'Hi Team, following up on our sync, I have compiled the revised pricing sheet. Let me know if we can proceed...'
			})
		}
	}
];

const mockSchedules = [
	{
		_id: 'sched-123',
		userId: 'mock-user-id',
		taskId: 'task-123456',
		startTime: Date.now() + 30 * 60 * 1000, // Starts in 30 mins
		endTime: Date.now() + 60 * 60 * 1000,
		allocationType: 'GHOST_BLOCK',
		calendarEventId: 'evt-cal-456',
		status: 'RESERVED'
	}
];

test.describe('Last-Minute Life Saver E2E Triage & Execution Flow', () => {
	test.beforeEach(async ({ context }) => {
		const signedCookie = createSignedCookie('mock-user-id');
		await context.addCookies([
			{
				name: 'session_id',
				value: signedCookie,
				domain: 'localhost',
				path: '/'
			}
		]);
	});

	test('should render active queue and execute action card successfully', async ({ page }) => {
		// Inject mock data before the page scripts run
		await page.addInitScript(({ tasks, schedules }) => {
			(window as any).__MOCK_TASKS__ = tasks;
			(window as any).__MOCK_SCHEDULES__ = schedules;
		}, { tasks: mockTasks, schedules: mockSchedules });

		// Route mock Go Gateway execution endpoint
		await page.route('**/v1/tasks/task-123456/execute', async (route) => {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ status: 'success' })
			});
		});

		// Go to dashboard
		await page.goto('/dashboard');

		// Assert page loaded and renders title
		await expect(page.locator('h1')).toContainText('Last-Minute Life Saver');

		// Assert Task Queue contains our mock task card
		await expect(page.getByRole('heading', { name: 'Review Q3 Client Agreement' })).toBeVisible();
		await expect(page.locator('span:has-text("Due in")').first()).toBeVisible();
		await expect(page.locator('span:has-text("Saves 15m")')).toBeVisible();

		// Assert Calendar View has reserved slot
		await expect(page.locator('h4:has-text("Tentative Focus Block")')).toBeVisible();

		// Click the execute action button
		const executeBtn = page.locator('#execute-btn-task-123456');
		await expect(executeBtn).toBeVisible();
		await executeBtn.click();

		// Assert button shows success state
		await expect(executeBtn).toContainText('Approved & Sent!');
	});
});
