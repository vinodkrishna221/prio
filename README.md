# prio

**prio** (previously known as *The Last-Minute Life Saver*) is a proactive, context-aware AI productivity companion built on a **High Autonomy with 1-Tap Execution** philosophy. Rather than simply alerting the user to an upcoming deadline, the agent runs in the background to pre-compile the required assets and execute the administrative heavy lifting, presenting resolutions to the user as actionable items.

When a task is due, the AI presents a completed resolution (e.g., a fully drafted email reply in Gmail, a pre-carved focus slot on Google Calendar, a pre-populated utility payment deep link, or a compiled briefing package) on a single **1-Tap Action Card**. The user is transitioned from an overwhelmed task organizer to a single-click executor, drastically reducing cognitive friction and eliminating task inertia.

---

## 🚀 Key Features

- **Background Ingestion & Triage Pipeline**: Evaluates incoming Gmail notifications via Pub/Sub, parses urgency, and auto-generates drafts and event requests.
- **Circadian Energy Profiler**: Dynamically scales task relevance based on active biometric/cognitive states. Recommends administrative chores during low energy blocks and strategic work during peak periods.
- **1-Tap Action Queue**: View, approve, or instantly dispatch pre-drafted replies and checkouts in under 5 seconds.
- **Micro-Gap Focus Calendar**: Proactively schedules "Ghost Blocks" on your Google Calendar to safeguard focused time. Self-dissolves blocks if corresponding tasks are completed early.
- **Interactive Onboarding Tour**: Cinematic dark-mode guided overlay to walk new users through key widgets.

---

## 📂 Repository File Structure

```
.
├── apps/
│   └── web/                     # SvelteKit + Tailwind v4 Web Client (runs on port 5173)
├── services/
│   ├── go-gateway/              # Go 1.22 Ingestion Gateway (REST, SSE, OAuth - runs on port 8080)
│   └── python-agent/            # Python LangGraph AI Reasoning Service (gRPC - runs on port 50051)
├── convex/                      # Convex Edge Database Schemas, Queries, & Mutations
├── proto/                       # Protocol Buffer Contract Definitions
├── docs/                        # Project documentation & overview files
├── .env.example                 # Environment variables template for local setup
└── package.json                 # Project dependencies configuration
```

---

## 🛠️ How to Run Locally

### Prerequisites
Make sure you have the following installed on your machine:
- **Node.js** (v20+) & **pnpm**
- **Go** (1.21+)
- **Python** (3.10+)
- **Docker Desktop** (optional, for containerization)
- **Redis** (running locally on port `6379`, or an Upstash Redis database URI)

---

### Step-by-Step Setup

#### 1. Configure Environment Variables
Copy `.env.example` in the root folder to `.env`:
```powershell
cp .env.example .env
```
Fill in the credentials in `.env` (including Google OAuth IDs/Secrets, and Convex URL keys). Also configure `apps/web/.env` pointing to `PUBLIC_CONVEX_URL` and `PUBLIC_GATEWAY_URL`.

---

#### 2. Start Convex (Database & Functions)
Install dependencies and run the Convex dev server to synchronize schemas and start the local environment:
```powershell
# Install root package dependencies
pnpm install

# Start the Convex development server (generates type bindings and deploys queries/mutations)
npx convex dev
```

---

#### 3. Run the Python AI Reasoning Service
Open a new terminal window:
```powershell
cd services/python-agent

# Create virtual environment and install dependencies
python -m venv .venv
.venv\Scripts\activate # On Windows PowerShell
pip install -r requirements.txt

# Run the gRPC server locally on port 50051
python main.py
```

---

#### 4. Run the Go Ingestion Gateway
Open a new terminal window:
```powershell
cd services/go-gateway

# Download dependencies
go mod download

# Start the Go gateway on port 8080
go run cmd/server/main.go
```

---

#### 5. Run the SvelteKit Frontend
Open a new terminal window:
```powershell
cd apps/web

# Install dependencies and launch Vite development server
pnpm install
pnpm run dev
```

Once all processes are running, visit **`http://localhost:5173`** to access your dashboard!
