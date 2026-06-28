import { query } from "./_generated/server";
import { v } from "convex/values";

// Fetches a user record by its Convex document ID.
// Used by the dashboard to reactively subscribe to the completedTour flag.
export const getUserById = query({
  args: { userId: v.id("users") },
  handler: async (ctx, args) => {
    return await ctx.db.get(args.userId);
  },
});



// Fetches the real-time active task cards sorted by priority
export const getActiveTasks = query({
  args: { userId: v.id("users") },
  handler: async (ctx, args) => {
    return await ctx.db
      .query("tasks")
      .withIndex("by_user_status_priority", (q: any) =>
        q.eq("userId", args.userId).eq("status", "ACTIVE")
      )
      .order("desc")
      .collect();
  },
});

// Fetches active calendar scheduling coordinates
export const getActiveSchedules = query({
  args: { userId: v.id("users") },
  handler: async (ctx, args) => {
    return await ctx.db
      .query("schedules")
      .withIndex("by_user", (q) => q.eq("userId", args.userId))
      .filter((q) => q.eq(q.field("status"), "RESERVED"))
      .collect();
  },
});

// Fetches user integration by provider
export const getIntegration = query({
  args: { userId: v.id("users"), provider: v.string() },
  handler: async (ctx, args) => {
    return await ctx.db
      .query("integrations")
      .withIndex("by_user_provider", (q: any) =>
        q.eq("userId", args.userId).eq("provider", args.provider)
      )
      .unique();
  },
});

// Fetches a task by its external ID
export const getTaskByExternalId = query({
  args: { userId: v.id("users"), externalTaskId: v.string() },
  handler: async (ctx, args) => {
    return await ctx.db
      .query("tasks")
      .withIndex("by_user", (q) => q.eq("userId", args.userId))
      .filter((q) => q.eq(q.field("externalTaskId"), args.externalTaskId))
      .first();
  },
});

// Fetches a single task by its Convex ID
export const getTask = query({
  args: { taskId: v.id("tasks") },
  handler: async (ctx, args) => {
    return await ctx.db.get(args.taskId);
  },
});

// Calculate total friction saved minutes (realized and potential) for a user
export const getFrictionSaved = query({
  args: { userId: v.id("users") },
  handler: async (ctx, args) => {
    const tasks = await ctx.db
      .query("tasks")
      .withIndex("by_user", (q) => q.eq("userId", args.userId))
      .collect();

    const completedSaved = tasks
      .filter((t) => t.status === "COMPLETED")
      .reduce((acc, t) => acc + (t.actionCard?.savesMinutes || 0), 0);

    const activeSaved = tasks
      .filter((t) => t.status === "ACTIVE")
      .reduce((acc, t) => acc + (t.actionCard?.savesMinutes || 0), 0);

    return {
      completed: completedSaved,
      active: activeSaved,
      total: completedSaved + activeSaved,
    };
  },
});

// Fetches the most recent retrospective genome report for the user
export const getLatestGenome = query({
  args: { userId: v.id("users") },
  handler: async (ctx, args) => {
    return await ctx.db
      .query("genomes")
      .withIndex("by_user", (q) => q.eq("userId", args.userId))
      .order("desc")
      .first();
  },
});

// Gathers tasks, schedules, and biometric logs for the last 7 days
export const getWeeklyStats = query({
  args: { userId: v.id("users") },
  handler: async (ctx, args) => {
    const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;

    // 1. Fetch recent tasks
    const allTasks = await ctx.db
      .query("tasks")
      .withIndex("by_user", (q) => q.eq("userId", args.userId))
      .collect();
    const tasks = allTasks.filter((t) => t.dueAt >= sevenDaysAgo || t.status === "ACTIVE" || t.status === "COMPLETED");

    // 2. Fetch recent schedules
    const allSchedules = await ctx.db
      .query("schedules")
      .withIndex("by_user", (q) => q.eq("userId", args.userId))
      .collect();
    const schedules = allSchedules.filter((s) => s.startTime >= sevenDaysAgo);

    // 3. Fetch recent biometric logs (last 7 days)
    const allBiometrics = await ctx.db
      .query("biometric_logs")
      .withIndex("by_user", (q) => q.eq("userId", args.userId))
      .collect();
    const dateLimitStr = new Date(sevenDaysAgo).toISOString().split("T")[0];
    const biometricLogs = allBiometrics.filter((b) => b.logDate >= dateLimitStr);

    return {
      tasks,
      schedules,
      biometricLogs,
    };
  },
});

