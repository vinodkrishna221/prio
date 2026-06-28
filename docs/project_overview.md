# Project Overview: prio

**prio** (previously *The Last-Minute Life Saver*) is a proactive, context-aware AI productivity companion built on a **High Autonomy with 1-Tap Execution** philosophy. Rather than simply alerting the user to an upcoming deadline, the agent runs in the background to pre-compile the required assets and execute the administrative heavy lifting, presenting resolutions to the user as actionable items.

---

## 1. Problem Statement
Traditional task managers (Google Tasks, Todoist, Apple Reminders) are fundamentally passive. They rely on time-based alerts that are easily ignored, leading to:
- **Task Paralysis**: Stressful major tasks (like tax forms or client agreements) cause users to freeze. A passive notification acts as an external stressor rather than a path to resolution.
- **Context-Switching Fatigue**: Moving between corporate tools, emails, LMS systems, and personal calendars wastes cognitive energy.
- **Deadline Blindness**: Busy individuals struggle to visualize tasks in the micro-gaps of their schedules, leading to late payments, missed work, and degraded reliability.

---

## 2. Product Vision
**prio** shifts the paradigm from *reminding* to *resolving*. By integrating deeply with Google Workspace and telemetry sources (e.g., biometrics), the platform:
1. Triages incoming items in the background.
2. Drafts resolution actions (e.g., pre-compiles email replies, prepares invoice checkouts).
3. Allocates target focus slots in the user's Google Calendar.
4. Renders a unified stream of **1-Tap Action Cards** that the user can approve, edit, or reject in under 5 seconds.

---

## 3. Core Persona Targets

### Alex (20) — The Overwhelmed Student
- **Job-to-be-Done**: When major papers are due, auto-carve dedicated focus slots and draft outlines so I can start without freezing up.
- **Value**: Overcomes task paralysis, organizes class assignments, and manages study blocks smoothly.

### Sarah (32) — The Hectic Professional
- **Job-to-be-Done**: When client emails arrive during meetings, pre-draft responses based on context and past threads to resolve inside small schedule gaps.
- **Value**: Meets critical SLAs without getting bogged down in writing standard administrative draft responses.

### Marcus (45) — The Busy Consumer
- **Job-to-be-Done**: When bills or chores are due, check out directly via pre-populated links inside calendar micro-gaps.
- **Value**: Avoids late fees and manages personal tasks in seconds.

---

## 4. Key Features (MoSCoW Matrix)

### Must-Haves
- **Intelligent Ingestion & Triage**: Background email and task parser utilizing Google Workspace webhooks.
- **Circadian Energy Profiler**: Learns from active biometric scores (computed energy state) to dynamically rank tasks (e.g., low-energy recommends low-effort bills; high-energy unlocks strategic planning).
- **1-Tap Action Queue**: Seamless action approval and instant execution (e.g., sending the pre-compiled email draft or booking calendar events).
- **Micro-Gap Focus Calendar**: Carves out tentative placeholder "Ghost Blocks" on the calendar to protect focus time, automatically freeing them if tasks are completed ahead of schedule.

### Should-Haves
- **Friction Reduction Index**: Real-time counter of total minutes saved by delegating chores directly to the agent.
- **Interactive Onboarding Tour**: Cinematic dark-mode guided walkthrough for new users.
- **Deferred Retries**: Robust background queuing for email draft execution with retry logic.
