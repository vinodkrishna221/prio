# Hackathon Submission Overview: prio

**prio** is a proactive, context-aware AI productivity companion built on a **High Autonomy with 1-Tap Execution** philosophy. Submitted for the Google Vibe2Ship Hackathon, it shifts the productivity paradigm from passive time-based notifications (which users easily ignore or snooze) to autonomous background triage and single-click administrative resolution.

---

## 💡 The Core Innovation

Traditional productivity tools are **passive reminders**—they act as external stressors rather than catalysts for resolution. When confronted with complex tasks (e.g., preparing client proposals, resolving billing disputes, or replying to urgent contract emails), users suffer from **cognitive overload** and freeze.

**prio** resolves this by operating as an autonomous, background agent that:
1. **Triages Incoming Events**: Synthesizes and prioritizes new emails and calendar updates.
2. **Pre-Compiles Resolutions**: Drafts Gmail responses, pre-populates bill pay forms, and prepares scheduling slots.
3. **Optimizes Focus Time**: Auto-schedules calendar blocks based on the user's focus availability.
4. **Delivers 1-Tap Action Cards**: Consolidates all resolutions into a unified dashboard stream where the user merely reviews, modifies, and clicks once to execute the entire background task.

---

## 🛠️ The Tech Innovation Pillars (Why It Wins)

### 🧠 Stateful AI Multi-Agent Orchestration (Python + LangGraph + Gemini)
At the core of the reasoning layer is a stateful Python microservice powered by **LangGraph** and the **Gemini 3.5 Flash** model. Rather than simple single-prompt calls, the AI operates as a graph of specialized agents:
- **Triage Agent**: Analyzes message content and schedules tasks dynamically.
- **Drafting Agent**: Auto-generates professional email responses matching past correspondence style.
- **Schedule Agent**: Carves out focus gaps in calendar events.

### ⚡ Sub-Millisecond Reactive Edge (Convex)
Prio uses **Convex** as its database and serverless function engine. Instead of standard REST polling or complex WebSockets, Convex sets up a persistent reactive edge. As the Python and Go services update tasks, database mutations instantly recalculate state and stream the new UI down to the client via reactive subscriptions, guaranteeing zero-latency interface updates.

### 🩺 Biometric & Circadian Rhythm Integration
Task priorities are not static. The platform computes a real-time **Circadian Energy Score** (1-10) using biometric data logs (sleep duration, step counts, and resting heart rate).
- **High Energy Score**: prioritizes complex, strategic drafts (e.g., Q3 Client Agreements).
- **Low Energy Score**: prioritizes low-effort administrative actions (e.g., checking out utility bills).

---

## 🚀 Fully Implemented Features

### 1. 1-Tap Actions Queue
A beautifully organized card queue sorted dynamically by Priority Score.
- **Gmail Draft Integration**: Pre-written replies appear directly on the dashboard. One click automatically sends the email via the Gmail API.
- **Smart Scheduling**: Pre-carved slots can be approved to lock calendar appointments immediately.

### 2. Circadian Energy Profiler
An interactive slider representing the user's cognitive state. Adjusting the energy score dynamically triggers the backend engine to re-rank the task priority list and contextually modify agent recommendations in real-time.

### 3. Friction Reduction Index
A stats dashboard tracking active and realized minutes saved. It aggregates the estimated cognitive time saved by delegating emails, scheduling tasks, and admin payments directly to the agent.

### 4. Micro-Gap Focus Calendar
A timeline visualizer displaying tentative **Ghost Blocks** (placeholder reservations) created by the agent in open calendar gaps.
- If a task is resolved or deleted, the system automatically **dissolves** the block to free the calendar slot.
- Once approved, the block is **committed** and written to Google Calendar.

### 5. Cinematic Onboarding Tour
A custom-built, premium onboarding walkthrough. Uses high-contrast backdrop overlays (creating a dynamic focus spotlight around active widgets) and spring-physics animated popovers to introduce new users to the interface. Supports complete keyboard accessibility (Escape, Enter, and Arrow keys).

---

## 🏗️ Decoupled Production-Grade Architecture

Prio is built as a polyglot microservices system:
- **SvelteKit Web Client**: Renders the dark-mode dashboard, integrates the onboarding tour, and handles client SSE connections.
- **Go Ingestion Gateway**: High-concurrency server handling OAuth 2.0 redirection, syncing Google Workspace API states (Gmail, Calendar, Tasks), and publishing SSE streams.
- **Python Reasoning Service**: Handles CPU-heavy LangGraph and Vertex AI SDK coordination.
- **Redis Cache & Google Pub/Sub**: Manages fast workspace context lookups and handles real-time push events from Gmail watch hookups.

---

## 🔒 Enterprise-Grade Security Framework

1. **AES-256-GCM Envelope Encryption**: OAuth tokens are never stored in raw plaintext. The Go Gateway encrypts tokens using AES-GCM prior to database persistence.
2. **Biometric Privacy Safeguards**: The database persists computed energy scores (0-100) only. Raw biometric telemetry (raw heart rates, exact sleep duration values) is parsed and discarded immediately at the ingest edge.
3. **Workload Identity Protection**: Production microservices are secured with non-public Cloud Run endpoints, requiring service-to-service credentials and denying public unauthenticated calls.

