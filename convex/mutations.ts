import { mutation, query } from "./_generated/server";
import { v } from "convex/values";

// Inserts a new user record with default parameters
export const createUser = mutation({
  args: { email: v.string() },
  handler: async (ctx, args) => {
    return await ctx.db.insert("users", {
      email: args.email,
      createdAt: Date.now(),
      currentEnergyScore: 5,
      energyLastUpdated: Date.now(),
    });
  },
});

// Returns the user with matching email, or null
export const getUserByEmail = query({
  args: { email: v.string() },
  handler: async (ctx, args) => {
    return await ctx.db
      .query("users")
      .withIndex("by_email", (q) => q.eq("email", args.email))
      .unique();
  },
});

// Ingests a new triaged task
export const ingestTriagedTask = mutation({
  args: {
    userId: v.id("users"),
    title: v.string(),
    source: v.union(v.literal("GMAIL"), v.literal("TASKS"), v.literal("MANUAL")),
    priorityScore: v.number(),
    durationMinutes: v.number(),
    dueAt: v.number(),
    actionCard: v.optional(
      v.object({
        actionType: v.union(
          v.literal("GMAIL_DRAFT"),
          v.literal("CALENDAR_BOOKING"),
          v.literal("BILL_PAY")
        ),
        savesMinutes: v.number(),
        draftId: v.optional(v.string()),
        payloadJson: v.string(),
      })
    ),
    externalTaskId: v.optional(v.string()),
  },
  handler: async (ctx, args) => {
    return await ctx.db.insert("tasks", {
      userId: args.userId,
      title: args.title,
      source: args.source,
      status: "ACTIVE",
      priorityScore: args.priorityScore,
      durationMinutes: args.durationMinutes,
      dueAt: args.dueAt,
      actionCard: args.actionCard,
      externalTaskId: args.externalTaskId,
    });
  },
});

// Patches currentEnergyScore and energyLastUpdated, and logs biometric log
export const updateUserEnergy = mutation({
  args: {
    userId: v.id("users"),
    score: v.number(),
  },
  handler: async (ctx, args) => {
    const timestamp = Date.now();
    await ctx.db.patch(args.userId, {
      currentEnergyScore: args.score,
      energyLastUpdated: timestamp,
    });

    const dateStr = new Date(timestamp).toISOString().split("T")[0];
    const existingLog = await ctx.db
      .query("biometric_logs")
      .withIndex("by_user_date", (q: any) =>
        q.eq("userId", args.userId).eq("logDate", dateStr)
      )
      .unique();

    if (existingLog) {
      await ctx.db.patch(existingLog._id, { computedEnergyScore: args.score });
    } else {
      await ctx.db.insert("biometric_logs", {
        userId: args.userId,
        logDate: dateStr,
        sleepDurationHours: 8.0,
        restingHeartRate: 70,
        stepCount: 5000,
        computedEnergyScore: args.score,
      });
    }
  },
});

// Upserts integration details
export const saveIntegration = mutation({
  args: {
    userId: v.id("users"),
    provider: v.string(),
    accessTokenEncrypted: v.string(),
    refreshTokenEncrypted: v.string(),
    watchExpiration: v.number(),
  },
  handler: async (ctx, args) => {
    const existing = await ctx.db
      .query("integrations")
      .withIndex("by_user_provider", (q: any) =>
        q.eq("userId", args.userId).eq("provider", args.provider)
      )
      .unique();

    if (existing) {
      await ctx.db.patch(existing._id, {
        accessTokenEncrypted: args.accessTokenEncrypted,
        refreshTokenEncrypted: args.refreshTokenEncrypted,
        watchExpiration: args.watchExpiration,
      });
      return existing._id;
    } else {
      return await ctx.db.insert("integrations", {
        userId: args.userId,
        provider: args.provider,
        accessTokenEncrypted: args.accessTokenEncrypted,
        refreshTokenEncrypted: args.refreshTokenEncrypted,
        watchExpiration: args.watchExpiration,
      });
    }
  },
});

// Programmatically performs a cascading delete of all schedules, tasks, biometric logs, integrations, and user profile
export const deleteUserAccount = mutation({
  args: { userId: v.id("users") },
  handler: async (ctx, args) => {
    // 1. Fetch and delete all schedules
    const schedules = await ctx.db
      .query("schedules")
      .withIndex("by_user", (q) => q.eq("userId", args.userId))
      .collect();
    for (const schedule of schedules) {
      await ctx.db.delete(schedule._id);
    }

    // 2. Fetch and delete all tasks
    const tasks = await ctx.db
      .query("tasks")
      .withIndex("by_user", (q) => q.eq("userId", args.userId))
      .collect();
    for (const task of tasks) {
      await ctx.db.delete(task._id);
    }

    // 3. Fetch and delete biometric logs
    const logs = await ctx.db
      .query("biometric_logs")
      .withIndex("by_user", (q) => q.eq("userId", args.userId))
      .collect();
    for (const log of logs) {
      await ctx.db.delete(log._id);
    }

    // 4. Fetch and delete all integrations
    const integrations = await ctx.db
      .query("integrations")
      .withIndex("by_user_provider", (q) => q.eq("userId", args.userId))
      .collect();
    for (const integration of integrations) {
      await ctx.db.delete(integration._id);
    }

    // 5. Finally, delete the User profile
    await ctx.db.delete(args.userId);
  },
});

// Updates the synchronization cursor for an integration
export const updateLastSyncCursor = mutation({
  args: {
    userId: v.id("users"),
    provider: v.string(),
    lastSyncCursor: v.string(),
  },
  handler: async (ctx, args) => {
    const existing = await ctx.db
      .query("integrations")
      .withIndex("by_user_provider", (q: any) =>
        q.eq("userId", args.userId).eq("provider", args.provider)
      )
      .unique();
    if (existing) {
      await ctx.db.patch(existing._id, {
        lastSyncCursor: args.lastSyncCursor,
      });
    }
  },
});

// Creates a synced task from Google Tasks
export const createSyncTask = mutation({
  args: {
    userId: v.id("users"),
    title: v.string(),
    status: v.union(
      v.literal("QUEUED"),
      v.literal("ACTIVE"),
      v.literal("COMPLETED"),
      v.literal("IGNORED")
    ),
    externalTaskId: v.string(),
    dueAt: v.number(),
  },
  handler: async (ctx, args) => {
    return await ctx.db.insert("tasks", {
      userId: args.userId,
      title: args.title,
      source: "TASKS",
      status: args.status,
      priorityScore: 50,
      durationMinutes: 30,
      dueAt: args.dueAt,
      externalTaskId: args.externalTaskId,
    });
  },
});

// Updates the status of an existing task
export const updateTaskStatus = mutation({
  args: {
    taskId: v.id("tasks"),
    status: v.union(
      v.literal("QUEUED"),
      v.literal("ACTIVE"),
      v.literal("COMPLETED"),
      v.literal("IGNORED")
    ),
  },
  handler: async (ctx, args) => {
    await ctx.db.patch(args.taskId, {
      status: args.status,
    });
  },
});

// Creates a new committed schedule record after a calendar event is created
export const createSchedule = mutation({
  args: {
    userId: v.id("users"),
    taskId: v.id("tasks"),
    startTime: v.number(),
    endTime: v.number(),
    allocationType: v.union(v.literal("GHOST_BLOCK"), v.literal("MICRO_GAP")),
    calendarEventId: v.string(),
    status: v.union(
      v.literal("RESERVED"),
      v.literal("DISSOLVED"),
      v.literal("COMMITTED")
    ),
  },
  handler: async (ctx, args) => {
    return await ctx.db.insert("schedules", {
      userId: args.userId,
      taskId: args.taskId,
      startTime: args.startTime,
      endTime: args.endTime,
      allocationType: args.allocationType,
      calendarEventId: args.calendarEventId,
      status: args.status,
    });
  },
});

// Updates the status of a schedule allocation
export const updateScheduleStatus = mutation({
  args: {
    scheduleId: v.id("schedules"),
    status: v.union(
      v.literal("RESERVED"),
      v.literal("DISSOLVED"),
      v.literal("COMMITTED")
    ),
  },
  handler: async (ctx, args) => {
    await ctx.db.patch(args.scheduleId, {
      status: args.status,
    });
  },
});

// Records a Gmail send failure against a task.
// When isFinal=true (3rd attempt exhausted) the task is marked IGNORED so
// the dashboard surfaces it as a permanently-failed card instead of an
// actionable item. The raw error message is stored for the UI to display.
export const recordTaskError = mutation({
  args: {
    taskId: v.id("tasks"),
    errorMessage: v.string(),
    sendAttempts: v.number(),
    isFinal: v.boolean(),
  },
  handler: async (ctx, args) => {
    const patch: Record<string, unknown> = {
      sendAttempts: args.sendAttempts,
      lastError: args.errorMessage,
    };
    if (args.isFinal) {
      patch.errorStatus = "SEND_FAILED";
      patch.status = "IGNORED"; // Remove from active queue; UI reads errorStatus to show the error badge
    }
    await ctx.db.patch(args.taskId, patch);
  },
});
