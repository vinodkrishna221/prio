# AGENT.md — Developer Instruction Manual
## Project: The Last-Minute Life Saver
### For: AI Coding Agents (Antigravity / Claude / Gemini Developer Models)
### Authority Level: Principal Product Manager → Software Engineering Lead

---

> [!IMPORTANT]
> This file is the **single source of truth** for all coding decisions on this project.
> Before writing a single line of code, you **MUST** read this entire file and all linked specification documents.
> Treat this file exactly as a human engineer would treat a PM's sprint brief and engineering charter.

---

## 0. The Three Laws of This Project

1. **Never write speculative code.** Every function, route, and schema must trace back to a specification in the PRD, System Architecture Document, or Database Design Document.
2. **Never merge a failing test.** If you write a feature, you must write its tests. If a test fails, you must fix the feature — not the test.
3. **Never skip a pre-flight check.** Before starting any task, run the mandatory pre-flight checklist in Section 2.

---

## 1. Mandatory Documentation to Read BEFORE Writing Any Code

Read all linked documents in this order. They are your specification baseline:

| Priority | Document | Path | Purpose |
| :---: | :--- | :--- | :--- |
| **P0** | Product Requirements Document (PRD) | `d:\last_minute_life_saver\PRD_Last_Minute_Life_Saver.md` | North Star — defines what to build and why |
| **P0** | System Architecture Document | `d:\last_minute_life_saver\System_Architecture_Last_Minute_Life_Saver.md` | How services communicate, Protobuf contracts, Terraform |
| **P0** | Database Design Specification | `d:\last_minute_life_saver\Database_Design_Last_Minute_Life_Saver.md` | Convex schemas, indexes, queries, and mutations |
| **P1** | Hackathon Guidelines | `d:\last_minute_life_saver\Vibe2Ship - Problem Statements & Submission Guidelines.md` | Deadline, submission constraints, evaluation rubric |

> [!CAUTION]
> If any specification in the docs above conflicts with a task instruction you receive, **stop and flag the conflict** rather than making a judgment call. Consult the PRD Section 3 MoSCoW framework to resolve priority.

---

## 2. Pre-Flight Checklist (Run Before EVERY Task)

Before starting any coding task, you must answer YES to all of the following:

```
PRE-FLIGHT CHECKLIST
─────────────────────────────────────────────────────────────
[ ] 1. Have I read the relevant section in the PRD for this feature?
[ ] 2. Have I verified the Convex schema fields in Database_Design.md?
[ ] 3. Have I verified the gRPC Protobuf contracts in System_Architecture.md?
[ ] 4. Have I checked that no existing test will break before I start?
[ ] 5. Have I identified which Phase (1-4) this task belongs to?
[ ] 6. Is this task within the current active Phase?
[ ] 7. Have I confirmed I will write unit tests BEFORE or ALONGSIDE the implementation?
─────────────────────────────────────────────────────────────
```

If you answer NO to any item, resolve it first before proceeding.

---

## 3. Repository Structure

The project is organized as a **monorepo**. Follow this exact directory layout:

```
d:\last_minute_life_saver\
├── AGENT.md                    ← You are here
├── .agents/
│   └── AGENTS.md               ← Workspace-scoped coding rules
│
├── apps/
│   └── web/                    ← SvelteKit Frontend (Phase 4)
│       ├── src/
│       │   ├── routes/
│       │   ├── lib/
│       │   │   ├── stores/     ← Svelte reactive stores (task queue, card state)
│       │   │   └── convex/     ← Convex client hooks
│       │   └── components/
│       │       └── cards/      ← 1-Tap Action Card components
│       ├── package.json
│       ├── svelte.config.js
│       └── vite.config.ts
│
├── services/
│   ├── go-gateway/             ← Go Ingestion API (Phase 2)
│   │   ├── cmd/server/main.go
│   │   ├── internal/
│   │   │   ├── handlers/       ← HTTP + SSE + Pub/Sub webhook handlers
│   │   │   ├── oauth/          ← Token encrypt/decrypt with KMS
│   │   │   ├── workspace/      ← Gmail, Calendar, Tasks API clients
│   │   │   └── cache/          ← Redis (Memorystore) helpers
│   │   ├── gen/                ← Generated Protobuf Go stubs
│   │   ├── go.mod
│   │   └── Makefile
│   │
│   └── python-agent/           ← Python LangGraph Service (Phase 3)
│       ├── main.py
│       ├── agents/
│       │   ├── triage_agent.py
│       │   ├── scheduler_agent.py
│       │   └── biometric_agent.py
│       ├── protos/             ← Generated Protobuf Python stubs
│       ├── tests/
│       ├── pyproject.toml
│       └── Dockerfile
│
├── convex/                     ← Convex DB Functions (Phase 1)
│   ├── schema.ts
│   ├── queries.ts
│   ├── mutations.ts
│   └── actions.ts
│
├── proto/                      ← Source Protobuf Definitions
│   ├── triage.proto
│   └── scheduler.proto
│
└── infra/                      ← Terraform IaC (Phase 1 / Post-MVP)
    ├── main.tf
    ├── variables.tf
    └── outputs.tf
```

---

## 4. Development Phases & Task Sequencing (The PM Roadmap)

Development is organized into 4 strictly sequential phases. **You must not start Phase N+1 until Phase N is complete and all tests pass.**

### Phase 1: Foundation — Convex Database & OAuth Security
**Goal**: Establish the live reactive data model and encrypted credential storage.

| Task ID | Task | Acceptance Criteria |
| :--- | :--- | :--- |
| `P1-01` | Write `convex/schema.ts` with all 5 tables | All indexes deployed to Convex dev project, Convex type-check passes |
| `P1-02` | Write `convex/queries.ts` (getActiveTasks, getActiveSchedules) | Reactive queries verified in Convex Dashboard |
| `P1-03` | Write `convex/mutations.ts` (ingestTriagedTask, updateUserEnergy, deleteUserAccount) | Cascading delete verified against dev database |
| `P1-04` | Write `convex/actions.ts` (triggerAgentReasoning stub) | Action invokes with HTTP 200 from mock endpoint |
| `P1-05` | Implement Go KMS envelope encryption + Convex token storage | `go test ./internal/oauth/...` passes with mock KMS |
| `P1-06` | Implement Google OAuth 2.0 login flow in Go (auth routes) | Successful redirect and token retrieval in dev environment |

**Phase 1 Exit Gate**: `npx convex dev` runs cleanly. `go test ./...` passes all `internal/oauth` tests.

---

### Phase 2: Ingestion Pipeline — Go API Gateway
**Goal**: Build the high-performance Go gateway handling all Workspace API integrations and event routing.

| Task ID | Task | Acceptance Criteria |
| :--- | :--- | :--- |
| `P2-01` | Implement Gmail watch registration & Pub/Sub push receiver | Mock Pub/Sub push trigger returns 200 and logs message ID |
| `P2-02` | Implement Gmail thread fetcher (`GET /v1/users/me/messages/{id}`) | Returns parsed thread struct with subject, sender, body |
| `P2-03` | Implement Google Calendar free-busy query + Redis caching | Redis `SETEX` verifiable via `redis-cli TTL` |
| `P2-04` | Implement Google Tasks sync (pull and write) | Round-trip create/read verified against Google Tasks API sandbox |
| `P2-05` | Implement Cloud Tasks queue creation for deferred triggers | Cloud Tasks item verifiable in GCP console |
| `P2-06` | Implement SSE stream broadcaster to SvelteKit clients | `curl -N http://localhost:8080/events` streams newline-delimited JSON |
| `P2-07` | Generate Protobuf Go stubs and implement gRPC client to Python agent | gRPC call to mock Python server returns valid `ProcessTriageResponse` |

**Phase 2 Exit Gate**: `go test ./...` passes all handler tests. Pub/Sub push → SSE stream verified end-to-end locally.

---

### Phase 3: Intelligence — Python LangGraph Reasoning Service
**Goal**: Build the multi-agent AI reasoning service that produces 1-Tap Action Card payloads.

| Task ID | Task | Acceptance Criteria |
| :--- | :--- | :--- |
| `P3-01` | Define LangGraph state graph with TriageAgent, SchedulerAgent, BiometricAgent nodes | `pytest tests/test_graph.py` verifies node traversal logic |
| `P3-02` | Implement Vertex AI Gemini call inside TriageAgent | Mock Vertex AI returns valid JSON; `mypy` passes on the file |
| `P3-03` | Implement SchedulerAgent (match task durations with calendar gaps) | Unit test covers edge case: zero gap → no allocation |
| `P3-04` | Implement BiometricAgent (consume energy score, re-rank tasks) | Task list re-ordering asserted in unit test |
| `P3-05` | Implement gRPC server (Python, port 50051) matching `triage.proto` | gRPC call from Go test client returns valid Protobuf response |
| `P3-06` | Dockerize Python service | `docker build` succeeds; `docker run` accepts gRPC connections on 50051 |

**Phase 3 Exit Gate**: `pytest tests/` 100% pass rate. `mypy agents/` exits with 0 errors. Docker image builds cleanly.

---

### Phase 4: Experience — SvelteKit UI & Real-Time Sync
**Goal**: Build the reactive web dashboard with 1-Tap Action Card UI connected to live Convex data.

| Task ID | Task | Acceptance Criteria |
| :--- | :--- | :--- |
| `P4-01` | Initialize SvelteKit project with TailwindCSS and Convex client SDK | `npm run dev` serves on localhost:5173 |
| `P4-02` | Build Convex client integration (reactive `useQuery` stores) | Active tasks re-render on mutation without page reload |
| `P4-03` | Build 1-Tap Action Card component (urgency badge, preview, action button) | Card renders correct color tier (Red/Yellow/Blue) based on deadline delta |
| `P4-04` | Connect SSE stream from Go Gateway to live card queue | New triage event appears on dashboard within 2 seconds |
| `P4-05` | Build Google OAuth login page (redirect to Go `/auth/google`) | Full login loop completes and stores session cookie |
| `P4-06` | Build Calendar micro-gap viewer (shows Ghost Blocks) | Calendar grid renders allocated slots from `getActiveSchedules` query |
| `P4-07` | End-to-end (E2E) integration test | Playwright: Gmail push → LangGraph → Convex → UI card appears |

**Phase 4 Exit Gate**: `npm run test` passes. Playwright E2E runs without failures. `npm run build` produces zero TypeScript errors.

---

## 5. Coding Standards & Quality Gates

### 5.1 Go (go-gateway)
- **Formatter**: Run `gofmt -w .` before every commit. No unformatted files.
- **Linter**: Run `golangci-lint run` and fix all errors. `nolint` comments are **prohibited**.
- **Error Handling**:
  ```go
  // ✅ CORRECT — wrap errors with context
  if err != nil {
      return fmt.Errorf("workspace/gmail: failed to fetch thread %s: %w", threadID, err)
  }
  // ❌ PROHIBITED — silent panic, naked log.Fatal
  if err != nil {
      panic(err)
  }
  ```
- **Secrets**: Never hardcode credentials. Use `Secret Manager` SDK to retrieve at startup.
- **Testing**: Every exported function in `internal/` must have a `_test.go` file using `testify/mock`.

### 5.2 Python (python-agent)
- **Formatter**: Run `black .` before committing. Line length max: 88.
- **Type Annotations**: Every function must have full type annotations. `mypy --strict` must pass.
  ```python
  # ✅ CORRECT
  def compute_energy_score(sleep_hours: float, steps: int) -> int:
      ...
  # ❌ PROHIBITED — missing annotations
  def compute_energy_score(sleep_hours, steps):
      ...
  ```
- **Testing**: All agents must have `pytest` tests. Mock `google.cloud.aiplatform` in all tests — never call live Vertex AI in unit tests.
- **Dependency Management**: All packages must be pinned in `pyproject.toml` with exact versions.

### 5.3 TypeScript / SvelteKit (web)
- **Formatter**: Run `prettier --write .` before committing.
- **Linter**: Run `eslint .` and fix all errors. Zero warnings allowed.
- **Type Safety**:
  ```typescript
  // ✅ CORRECT — explicit interface
  interface TaskCard { id: string; title: string; priorityScore: number; }
  // ❌ PROHIBITED — any type
  const card: any = fetchCard();
  ```
- **No Direct API Calls**: SvelteKit components must NEVER call Google APIs directly. All mutations go through Convex or the Go Gateway.
- **Testing**: Use `vitest` for unit tests. Use `playwright` for E2E tests.

---

## 6. Definition of Done (DoD)

A task is **Done** only when ALL of the following are true:

```
DEFINITION OF DONE CHECKLIST
─────────────────────────────────────────────────────────────────────
[ ] Feature implements the specification in the linked PRD/Arch doc
[ ] Code is formatted (gofmt / black / prettier — zero formatter diffs)
[ ] Linter passes with zero errors (golangci-lint / mypy / eslint)
[ ] Unit tests are written and all pass (go test / pytest / vitest)
[ ] No hardcoded secrets or API keys in any source file
[ ] Error handling is explicit (no silent panics / uncaught promise rejections)
[ ] New Convex schema/index changes are deployed to dev project
[ ] Any new environment variables are documented in the service's README
─────────────────────────────────────────────────────────────────────
```

---

## 7. Environment & Secrets Management

| Variable Name | Service | Source | Description |
| :--- | :--- | :--- | :--- |
| `GOOGLE_CLIENT_ID` | Go Gateway | Secret Manager | OAuth 2.0 Client ID |
| `GOOGLE_CLIENT_SECRET` | Go Gateway | Secret Manager | OAuth 2.0 Client Secret |
| `KMS_KEY_ID` | Go Gateway | Env Var / Terraform Output | Cloud KMS crypto key path |
| `REDIS_HOST` | Go Gateway | Env Var / Terraform Output | Memorystore Redis host |
| `VERTEX_PROJECT_ID` | Python Agent | Env Var | GCP project for Vertex AI |
| `VERTEX_LOCATION` | Python Agent | Env Var | GCP region for Gemini API |
| `CONVEX_URL` | Web / Actions | Env Var | Convex deployment URL |
| `CONVEX_DEPLOY_KEY` | CI / Actions | Secret Manager | Convex server-side deploy key |

> [!WARNING]
> Creating a `.env` file at the project root for local development is acceptable. This file **must** be listed in `.gitignore` and must **never** be committed to the repository.

---

## 8. Running the Full Local Development Stack

Run services in this order:

```powershell
# Step 1: Start Convex dev server (watch mode)
cd d:\last_minute_life_saver
npx convex dev

# Step 2: Start Redis (Docker)
docker run -p 6379:6379 redis:alpine

# Step 3: Start Python LangGraph gRPC service
cd services/python-agent
uvicorn main:app --port 50051

# Step 4: Start Go Ingestion Gateway
cd services/go-gateway
go run cmd/server/main.go

# Step 5: Start SvelteKit frontend
cd apps/web
npm run dev
```

---

## 9. Git Commit Message Format

All commits must follow the **Conventional Commits** standard:

```
<type>(<scope>): <short summary>

Types:  feat | fix | test | refactor | chore | docs | style
Scope:  convex | go-gateway | python-agent | web | infra | proto

Examples:
  feat(go-gateway): add Pub/Sub push subscription endpoint for Gmail watch
  test(python-agent): add pytest mock for Vertex AI GenerateContentRequest
  fix(convex): correct biometric_logs index field ordering
  chore(infra): add Cloud Tasks queue Terraform resource
```

---

## 10. Escalation Protocol

If a coding agent encounters any of the following, it must **stop and report** rather than guess:

- A specification ambiguity between the PRD and System Architecture Document.
- A Google API error (e.g. 403 Insufficient Permission scope) requiring an OAuth scope change.
- A Convex schema migration that would delete existing data.
- A Terraform change that modifies IAM role bindings.
- Any situation where `panic()` would seem like the only option.

Report the blocker with the following structure:
```
BLOCKER REPORT
Task: <Task ID>
Phase: <Phase Number>
Issue: <Clear description of the conflict or error>
Options Considered: <List 2-3 possible solutions>
Recommended: <Your recommended resolution>
Needs PM approval: YES
```
