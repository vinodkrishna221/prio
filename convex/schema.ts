import { defineSchema, defineTable } from "convex/server";
import { v } from "convex/values";

export default defineSchema({
  // Users Collection
  users: defineTable({
    email: v.string(),
    createdAt: v.number(),
    currentEnergyScore: v.number(),
    energyLastUpdated: v.number(),
    // Onboarding tour — optional so existing rows are unaffected (no migration needed)
    completedTour: v.optional(v.boolean()),
    // Weekly scheduling preferences derived from genome insights
    schedulingPreferences: v.optional(v.string()),
  }).index("by_email", ["email"]),

  // Tasks Collection
  tasks: defineTable({
    userId: v.id("users"),
    title: v.string(),
    source: v.union(v.literal("GMAIL"), v.literal("TASKS"), v.literal("MANUAL")),
    status: v.union(
      v.literal("QUEUED"),
      v.literal("ACTIVE"),
      v.literal("COMPLETED"),
      v.literal("IGNORED")
    ),
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
        payloadJson: v.string(), // Stringified JSON configuration payload
      })
    ),
    externalTaskId: v.optional(v.string()),
    // Email send retry tracking — all optional so existing rows are unaffected
    sendAttempts: v.optional(v.number()),   // how many times a GMAIL_DRAFT send has been attempted
    lastError: v.optional(v.string()),       // last Gmail API error message
    errorStatus: v.optional(v.string()),     // "SEND_FAILED" when permanently failed after max retries
  })
    .index("by_user", ["userId"])
    .index("by_user_status_priority", ["userId", "status", "priorityScore"])
    .index("by_due_date", ["userId", "dueAt"]),

  // Schedules / Calendar Allocations Collection
  schedules: defineTable({
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
  })
    .index("by_user", ["userId"])
    .index("by_task", ["taskId"])
    .index("by_time_window", ["userId", "startTime", "endTime"]),

  // Biometric Logs Collection
  biometric_logs: defineTable({
    userId: v.id("users"),
    logDate: v.string(), // ISO format: YYYY-MM-DD
    sleepDurationHours: v.number(),
    restingHeartRate: v.number(),
    stepCount: v.number(),
    computedEnergyScore: v.number(),
  })
    .index("by_user", ["userId"])
    .index("by_user_date", ["userId", "logDate"]),

  // OAuth Integrations Collection
  integrations: defineTable({
    userId: v.id("users"),
    provider: v.string(),
    accessTokenEncrypted: v.string(),
    refreshTokenEncrypted: v.string(),
    lastSyncCursor: v.optional(v.string()),
    watchExpiration: v.number(),
  }).index("by_user_provider", ["userId", "provider"]),

  // Retrospective Genome reports
  genomes: defineTable({
    userId: v.id("users"),
    weekStartDate: v.string(), // ISO format: YYYY-MM-DD
    deadlineRiskScore: v.number(),
    peakHours: v.array(v.string()),
    insights: v.array(
      v.object({
        category: v.union(v.literal("ENERGY"), v.literal("FRICTION"), v.literal("SCHEDULE")),
        title: v.string(),
        description: v.string(),
        impact: v.string(),
      })
    ),
    createdAt: v.number(),
  })
    .index("by_user", ["userId"])
    .index("by_user_week", ["userId", "weekStartDate"]),
});
