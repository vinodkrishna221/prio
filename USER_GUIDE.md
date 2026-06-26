# User Guide & Technical Explanation: The Last-Minute Life Saver

Welcome to **The Last-Minute Life Saver**! This document provides a complete guide on how to use the application, how it operates behind the scenes, and a detailed breakdown of each of its features.

---

## 1. Executive Summary: What is "The Last-Minute Life Saver"?

Existing productivity systems are **passive**: they notify you of a deadline, but leave you to do the cognitive and administrative heavy lifting. This often results in *task paralysis* (cognitive overload under stress), *deadline blindness*, and *context-switching fatigue*.

**The Last-Minute Life Saver** is a proactive AI-powered productivity companion built on a **High Autonomy with 1-Tap Execution** philosophy:
* It runs silently in the background, monitoring your Gmail inbox and Google Tasks.
* It evaluates tasks dynamically using Vertex AI Gemini models, weighting them by complexity and priority.
* It scans your Google Calendar to locate empty time windows (**Micro-Gaps**), matching them with appropriate tasks based on your current physical/cognitive **Energy Score**.
* It pre-compiles task resolutions and presents them on the SvelteKit frontend as **1-Tap Action Cards** (e.g., pre-written Gmail replies, payment checkouts, or confirmed calendar reservations). 
* With a single click, you execute the task—instantly completing the administrative loop.

---

## 2. System Architecture & Components

The application is built on a decoupled, event-driven microservices architecture deployed on **Google Cloud Platform (GCP)** and **Convex**:

```
                       ┌──────────────────────────────────────┐
                       │    SvelteKit Web Client (Frontend)   │
                       └──────────────────┬───▲───────────────┘
                                    HTTPS │   │ Server-Sent Events (SSE)
                                    (REST)│   │ (Live Card Streams)
                                          ▼   │
 ┌────────────────────────────────────────┴───┴────────────────────────────────────────┐
 │                                   GCP CLOUD RUN                                     │
 │                                                                                     │
 │  ┌──────────────────────────────────────┐     gRPC over HTTP/2     ┌─────────────┐  │
 │  │         Go Ingestion Gateway         │◄────────────────────────►│   Python    │  │
 │  │   - Port: 8080 (Ingress Gateway)     │  Proto: triage.proto     │  LangGraph  │  │
 │  │   - Language: Go 1.22                │  Proto: scheduler.proto  │  Reasoning  │  │
 │  │   - Binary Size: ~18MB (No runtime)  │                          │   Service   │  │
 │  └──────────────────┬───────────────────┘                          └──────┬──────┘  │
 └─────────────────────┼─────────────────────────────────────────────────────┼─────────┘
                       │                                                     │
       OAuth Actions / │                                                     │ Tool Calls /
       JSON Storage    ▼                                                     ▼ Context Checks
                ┌──────┴──────┐       Free-Busy Caches       ┌───────────────┴──────┐
                │  Convex DB  │◄─────────────────────────────┤ Memorystore (Redis)  │
                │  (Reactive) │   Sub-millisecond queries    │ - Port: 6379         │
                └─────────────┘                              └──────────────────────┘
```

### 2.1 SvelteKit Web Dashboard (`apps/web`)
* **Framework**: SvelteKit with TypeScript and custom Vanilla CSS layout rules (offering a premium glassmorphic dark-mode interface).
* **Reactivity**: Subscribes directly to Convex stores for real-time, sub-millisecond updates without REST polling.
* **Server-Sent Events (SSE)**: Establishes a persistent SSE channel with the Go Ingestion Gateway to receive instant notifications (e.g., when a task has finished agent triaging or when a scheduled focus block is about to start).

### 2.2 Go Ingestion Gateway (`services/go-gateway`)
* **Role**: Primary ingress gateway for OAuth authentication, Google Workspace syncing, SSE streaming, and action card execution.
* **Security & KMS**: Decrypts user credentials on-the-fly. Google OAuth refresh tokens are stored in Convex encrypted via AES-256-GCM using keys managed by Google Cloud KMS (Envelope Encryption).
* **Speed**: Compiled binary runs with minimal RAM (~15MB), ensuring sub-200ms cold starts on Cloud Run.

### 2.3 Python LangGraph Reasoning Service (`services/python-agent`)
* **Role**: Coordinates background agent reasoning using stateful directed acyclic graphs (**LangGraph**).
* **AI Engine**: Interacts with the **Gemini 2.0 Flash** model (via Vertex AI SDK) to triage incoming items and structure response JSON payloads.
* **gRPC Boundaries**: Communicates with the Go Gateway over high-performance HTTP/2 gRPC using Protobuf contracts (`triage.proto` and `scheduler.proto`).

### 2.4 Convex Database (`convex/`)
* **Role**: Serverless edge database. It hosts user profiles, task queues, scheduled blocks, biometric logs, and OAuth integration data.
* **Reactivity**: Automates data sync to SvelteKit clients using live websocket queries.

---

## 3. Detailed Feature Breakdown

### 3.1 Intelligent Task Triaging (Gemini-Powered)
When a new email is received, or a manual sync is run:
1. **Gmail Pub/Sub Webhook**: Gmail fires a push notification to GCP Pub/Sub, which routes to the Go Gateway webhook (`HandleGmailWebhook`).
2. **Context Assembly**: The Go Gateway retrieves the email content and joins it with cached user data (current location, active tags, and biometric energy levels).
3. **LangGraph Processing**: A gRPC request is sent to the Python agent. The agent runs a Vertex AI Gemini model to evaluate the text against the user context.
4. **Triage Results**: Gemini determines:
   * **Triage Priority Score (1-100)**: Rank based on deadline proximity and difficulty.
   * **Urgency Level**: `AMBIENT`, `QUIET`, or `CRITICAL`.
   * **Action Type**: `GMAIL_DRAFT` (creates an email draft), `CALENDAR_BOOKING` (reserves calendar slot), or `BILL_PAY` (creates a utility payment link).
   * **Draft Payload**: Context-specific values (e.g., pre-written reply body, payee name, bill amount).
   * **Friction Saved**: Estimated time saved (e.g., "Saves 15 mins").
   * **Cognitive Effort**: `HIGH`, `MEDIUM`, or `LOW` required to complete the task.

### 3.2 Circadian Energy Profiler (Biometric Load Sync)
This feature bridges health telemetry and productivity:
* **Telemetry Input**: In production, reads daily sleep efficiency, heart rate, and step counts from the user's mobile device (Android Health Connect API). In the Web Dashboard, this is represented by an **Energy Slider (1 to 10)**.
* **Database Updates**: Setting a new energy score writes to the Convex `users` profile and logs history in `biometric_logs`.
* **Agent Modification node (`biometric_agent_node`)**:
  * **Demotion**: If energy is low ($\le 4$) and a task requires `HIGH` cognitive effort, the agent lowers its priority by 20 points and demotes the urgency tier (e.g. CRITICAL to QUIET), preventing stressful alerts.
  * **Promotion**: If energy is high ($\ge 7$) and a task is `HIGH` effort, it raises the priority by 15 points and escalates the urgency tier to prompt quick execution.
* **Scheduler Modification node (`biometric_scheduler_node`)**:
  * **Low Energy**: Swaps `HIGH` effort tasks scheduled within the next 24 hours with `LOW/MEDIUM` effort tasks scheduled later.
  * **High Energy**: Promotes `HIGH` effort tasks to peak focus blocks (morning hours 9:00 AM - 12:00 PM UTC or blocks $\ge 1$ hour).

### 3.3 Micro-Gap Scheduling Engine
Calculates exactly *when* the user should work on a task:
1. **Busy Calendar Retrieval**: Queries the Google Calendar API for busy slots over the next 7 days (results are cached in Redis for 5 minutes).
2. **Free-Gap Calculation**: Subtracts busy segments from default working hours (9:00 AM to 6:00 PM UTC). Ignores any free gaps shorter than 15 minutes.
3. **Greedy Allocation**: Sorts tasks chronologically by deadline and priority, matching them sequentially with the earliest fitting free gap before their hard deadline. Low-duration tasks (< 60 mins) are booked as **Micro-Gaps**, while longer ones are booked as **Focus Blocks**.

### 3.4 "Ghost" Time-Blocking & Self-Dissolving Reserve Pools
If a high-priority task (score $\ge 70$) has no available free calendar gaps:
* **Ghost Booking**: The scheduler reserves a tentative placeholder block on Google Calendar immediately prior to the deadline (**Ghost Block**).
* **Automated Reclaiming**: If the task is completed (either via Svelte UI or Google Tasks completion webhooks), the Go Gateway receives a task completed notification.
* **Self-Dissolution**: The Go Gateway instantly calls the Google Calendar API to delete the tentative reservation, returning the free time to the user's schedule automatically.

### 3.5 1-Tap Action Cards (The Front-End Interface)
Displayed on the Web Dashboard, these glassmorphic cards represent pre-compiled solutions that can be executed in 1-Tap:
1. **Gmail Draft Card**:
   * Shows the recipient and subject.
   * Displays an **editable** preview text box containing the pre-written email body.
   * Tapping **"1-Tap Send Response"** triggers the Go Gateway to transition the draft directly to Gmail's outbox.
2. **Calendar Booking Card**:
   * Displays the scheduled date, time slot, and an **editable** location input (defaults to Google Meet).
   * Tapping **"1-Tap Confirm Slot"** patches the tentative Google Calendar event to "confirmed" and updates the Convex schedule table to `COMMITTED`.
3. **Bill Pay Card**:
   * Displays the utility merchant payee name, due date, and an **editable** bill amount.
   * Tapping **"1-Tap Pay Bill"** initiates the payment redirect gateway (e.g. Chrome Autofill or Google Pay) and marks the task as completed.

---

## 4. How to Use the Application: User Workflow

Follow these steps to experience the end-to-end flow of the application:

### Step 1: Sign-In via Google OAuth
1. Open the Web Dashboard (runs by default at `http://localhost:5173`).
2. Click **"Sign In with Google"**. This redirects you to the Google Consent screen.
3. Grant permissions for Gmail Compose/Read, Calendar events, and Google Tasks.
4. Once completed, you will be redirected back to the **Dashboard** (`/dashboard`).

### Step 2: Establish Real-time Sync
1. On the top right of the dashboard, click **🔄 Sync Workspace**.
2. This manually triggers a Google Tasks import (fetching your active task list) and registers a webhook watch with Google Pub/Sub for your Gmail Inbox.
3. You will see a green **Connected** badge in the header, indicating a live Server-Sent Events (SSE) connection to the Go Gateway.

### Step 3: View Tasks and Calendar Gaps
1. **Action Queue**: Ingested tasks appear in the left-hand column under the **1-Tap Actions Queue**, automatically prioritized by their AI score.
2. **Focus Calendar**: The right-hand column display (**Micro-Gap Focus Calendar**) displays your upcoming time-blocks.
   * **Reserved (Yellow)** blocks denote AI-allocated micro-gaps or ghost blocks.
   * **Freed (Gray/Line-through)** blocks show reclaimed time slots where tasks were completed early.
   * **Committed (Green)** blocks denote locked-in slots confirmed by the user.

### Step 4: Interact and Execute
1. Set your **Circadian Energy Profiler** slider on the dashboard to reflect how you feel:
   * Drag to **2 (Exceeded)**: Complex drafts will be deprioritized, and light administrative tasks (like bill payments) will bubble to the top.
   * Drag to **9 (Peak)**: Complex email drafts will be prioritized and scheduled into peak morning hours.
2. Review a card in the queue, make edits to the email text or payment amount inside the card preview, and click the **Action Button** (e.g., "1-Tap Send Response").
3. A glowing success notification pops up, the card scales out of view, and the task is archived.

---

## 5. Developer Guide: How to Run the Project Locally

To run the full stack locally, follow these instructions:

### 5.1 Environment Configuration (.env files)
Create the following environment files in their respective folders:

#### Workspace Root `.env`
```env
# Convex URL (obtained after running npx convex dev)
CONVEX_DEPLOYMENT=...
PUBLIC_CONVEX_URL=https://...

# Google OAuth API Credentials
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret
GOOGLE_REDIRECT_URL=http://localhost:8080/oauth/callback

# Go Gateway Configuration
PORT=8080
REDIS_HOST=localhost:6379
KMS_KEY_ID=mock-kms-key-id
GMAIL_PUBSUB_TOPIC=projects/your-gcp-project/topics/gmail-watch
COOKIE_KEY=your-32-byte-hexadecimal-cookie-signing-key
DASHBOARD_URL=http://localhost:5173/dashboard
PYTHON_AGENT_GRPC_ADDR=localhost:50051
```

#### SvelteKit Frontend `.env` (`apps/web/.env`)
```env
PUBLIC_CONVEX_URL=https://...
PUBLIC_GATEWAY_URL=http://localhost:8080
PUBLIC_ENV=development
```

### 5.2 Starting the Microservices
Open separate terminal sessions in the project folder and start the services in order:

#### 1. Start Convex Backend
Convex handles data schema syncing and client subscriptions.
```powershell
npx convex dev
```

#### 2. Start Redis (Local Cache)
Make sure a local Redis server is running on `localhost:6379`.
```powershell
docker run -d -p 6379:6379 redis:alpine
```

#### 3. Start Python LangGraph Agent
Install python packages and run the gRPC server:
```powershell
cd services/python-agent
# Create and activate virtual environment
python -m venv .venv
.venv\Scripts\Activate.ps1
pip install -r requirements.txt
python main.py
```
*(Runs the gRPC server on port `50051`)*

#### 4. Start Go Ingestion Gateway
Run the server:
```powershell
cd services/go-gateway
go run cmd/server/main.go
```
*(Runs the HTTP Ingress API gateway on port `8080`)*

#### 5. Start SvelteKit Web App
Install dependencies and run Svelte dev server:
```powershell
cd apps/web
pnpm install
pnpm run dev
```
*(Runs the SvelteKit frontend server on `http://localhost:5173`)*
