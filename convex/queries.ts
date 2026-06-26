import { query } from "./_generated/server";
import { v } from "convex/values";

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
      .unique();
  },
});

// Fetches a single task by its Convex ID
export const getTask = query({
  args: { taskId: v.id("tasks") },
  handler: async (ctx, args) => {
    return await ctx.db.get(args.taskId);
  },
});
