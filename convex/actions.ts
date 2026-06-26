import { action } from "./_generated/server";
import { v } from "convex/values";

// Stub action for triggering agent reasoning
export const triggerAgentReasoning = action({
  args: {
    userId: v.id("users"),
    taskId: v.id("tasks"),
    taskContent: v.string(),
  },
  handler: async (ctx, args) => {
    console.log(`[Stub Agent Reasoning] processing taskId=${args.taskId} for userId=${args.userId}`);
    
    return {
      status: 200,
      message: "Agent reasoning triggered successfully (stub)",
      taskId: args.taskId,
      priorityScore: 75,
      savesMinutes: 15,
    };
  },
});
