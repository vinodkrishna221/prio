# 🚀 Production Deployment Guide: The Last-Minute Life Saver

> **Who is this for?** Complete beginners. This guide explains every single step in plain English.
> **Estimated time:** 60–90 minutes for first-time setup.
> **Estimated cost:** ~$0–5/month (all services have generous free tiers).

---

## What You're Deploying (The Big Picture)

Your project has **4 services** that need to live on the internet:

```
┌─────────────────────────────────────────────────────────┐
│  YOUR USERS (Browser)                                   │
│           │                                             │
│           ▼                                             │
│  ┌──────────────────┐        ┌────────────────────┐     │
│  │  Vercel           │        │  Convex Cloud      │     │
│  │  (Your Website)   │◄──────►│  (Your Database)   │     │
│  └──────────────────┘        └────────────────────┘     │
│           │                                             │
│           ▼                                             │
│  ┌──────────────────┐        ┌────────────────────┐     │
│  │  Google Cloud Run │        │  Google Cloud Run  │     │
│  │  (Go Gateway)     │──gRPC─►│  (Python AI Agent) │     │
│  └──────────────────┘        └────────────────────┘     │
│           ▲                           │                 │
│           │                           ▼                 │
│  ┌──────────────────┐        ┌────────────────────┐     │
│  │  Gmail Pub/Sub    │        │  Vertex AI Gemini  │     │
│  │  (Email Watcher)  │        │  (AI Brain)        │     │
│  └──────────────────┘        └────────────────────┘     │
│                                                         │
│           ┌──────────────────┐                          │
│           │  Upstash Redis   │                          │
│           │  (Fast Cache)    │                          │
│           └──────────────────┘                          │
└─────────────────────────────────────────────────────────┘
```

**In plain English:**
- **Vercel** → Hosts your website (the dashboard users see)
- **Convex Cloud** → Your database (stores users, tasks, schedules)
- **Cloud Run (Go Gateway)** → The "brain connector" that talks to Gmail, Calendar, and routes work to the AI
- **Cloud Run (Python Agent)** → The AI that reads emails and decides what to do
- **Pub/Sub** → A "mail carrier" that tells your Go Gateway when a new email arrives
- **Upstash Redis** → A fast temporary storage (cache) so your app doesn't ask Google the same thing twice
- **Vertex AI Gemini** → Google's AI model that the Python Agent uses to understand emails

---

## Prerequisites — Things You Need Before Starting

Make sure you have these installed on your computer. Open PowerShell and run each command to check:

| Tool | Check Command | If Missing |
|:-----|:-------------|:-----------|
| **Node.js** (v20+) | `node --version` | Download from https://nodejs.org |
| **pnpm** | `pnpm --version` | Run `npm install -g pnpm` |
| **Go** (1.21+) | `go version` | Download from https://go.dev/dl/ |
| **Docker Desktop** | `docker --version` | Download from https://www.docker.com/products/docker-desktop |
| **Google Cloud CLI** | `gcloud --version` | Download from https://cloud.google.com/sdk/docs/install |
| **Git** | `git --version` | Download from https://git-scm.com/downloads |

> **Important:** Make sure Docker Desktop is **running** (open the app) before you start.

---

## Phase 1: Deploy Convex (Your Database) to Production

You already have Convex running in dev mode. Now we push it to production.

### Step 1.1: Deploy Your Schema and Functions

Open PowerShell in your project root folder and run:

```powershell
cd d:\last_minute_life_saver
npx convex deploy
```

It will ask you questions — answer them:
- **Select project**: Choose your existing project (e.g., `prio`)
- **Deploy to production?**: Type `y` and press Enter

### Step 1.2: Get Your Production Keys

1. Open your browser and go to https://dashboard.convex.dev/
2. Click on your project name
3. Switch to the **Production** deployment (not Development)
4. Click **Settings** (the gear icon ⚙️)
5. Find and copy these two values:

| What to Copy | Where to Find It | Example Value |
|:-------------|:-----------------|:-------------|
| **Deployment URL** | Settings page, labeled "URL" | `https://precise-hornet-895.eu-west-1.convex.cloud` |
| **Deploy Key** | Click "Generate Production Deploy Key" | `prod:precise-hornet-895\|eyJ...` |

📝 **Write these down somewhere safe** (a text file on your desktop is fine). You'll need them later.

✅ **Done!** Your database is now live in production.

---

## Phase 2: Set Up Upstash Redis (Your Cache)

### Step 2.1: Create an Account

1. Go to https://console.upstash.com/ in your browser
2. Click **Sign Up** (you can use your Google or GitHub account — it's free)

### Step 2.2: Create a Redis Database

1. After logging in, click the big **"Create Database"** button
2. Fill in:
   - **Name:** `last-minute-life-saver`
   - **Type:** Regional
   - **Region:** `US-Central1 (Iowa)` (or whatever is closest to you)
3. Click **Create**

### Step 2.3: Copy Your Connection Details

After creation, you'll see a dashboard for your database. Look for the **Connection** section:

1. Find the **Redis URL** — it looks like:
   ```
   redis://default:AbCdEfG123456@guiding-piglet-12345.upstash.io:6379
   ```
2. Copy the entire URL

📝 **Write down the full Redis URL.** You'll need it later.

✅ **Done!** Your cache is ready.

---

## Phase 3: Set Up Google Cloud Project

### Step 3.1: Create a Project

1. Go to https://console.cloud.google.com/ in your browser
2. Sign in with your Google account
3. At the top of the page, click the project dropdown (it might say "Select a project")
4. Click **"New Project"** in the popup
5. Enter:
   - **Project name:** `last-minute-life-saver` (or any name)
6. Click **Create**
7. Wait 10–30 seconds for it to be created
8. Make sure the new project is selected in the dropdown

### Step 3.2: Enable Billing

1. In the left sidebar, click **Billing**
2. Click **Link a billing account**
3. Add a credit card (Google gives you **$300 free credits** for 90 days — you won't be charged for this small project)

### Step 3.3: Sign In to gcloud CLI

Go back to your PowerShell and run:

```powershell
gcloud auth login
```

This opens your browser. Sign in with the same Google account. Then set your project:

```powershell
gcloud config set project YOUR_PROJECT_ID
```

> **How to find your Project ID:**
> - Go to https://console.cloud.google.com/
> - Click the project dropdown at the top
> - Your Project ID is shown in gray text under the project name (e.g., `last-minute-life-saver-12345`)
> - It's NOT the project name — it's a unique ID that might have numbers at the end

### Step 3.4: Enable All Required APIs

Run this single command (copy the entire thing):

```powershell
gcloud services enable run.googleapis.com cloudtasks.googleapis.com pubsub.googleapis.com secretmanager.googleapis.com artifactregistry.googleapis.com cloudkms.googleapis.com iam.googleapis.com aiplatform.googleapis.com
```

⏳ This takes 1–2 minutes. Wait for it to finish.

### Step 3.5: Create Artifact Registry

This is where your Docker images (packaged apps) will be stored:

```powershell
gcloud artifacts repositories create app-repo --repository-format=docker --location=us-central1 --description="App container images"
```

### Step 3.6: Connect Docker to Google Cloud

```powershell
gcloud auth configure-docker us-central1-docker.pkg.dev
```

When it asks "Do you want to continue?", type `Y` and press Enter.

✅ **Done!** Google Cloud is ready.

---

## Phase 4: Store Secrets in Google Cloud

Secrets are passwords and API keys. Instead of putting them in your code (dangerous!), we store them safely in Google Cloud.

### Step 4.1: Create Each Secret

Run each of these commands **one at a time**. Replace the placeholder values with YOUR actual values:

**Secret 1 — Convex Deploy Key:**
```powershell
"YOUR_CONVEX_PRODUCTION_DEPLOY_KEY" | gcloud secrets create CONVEX_DEPLOY_KEY --data-file=-
```
Replace `YOUR_CONVEX_PRODUCTION_DEPLOY_KEY` with the production deploy key from Phase 1.

**Secret 2 — Google OAuth Client ID:**
```powershell
"YOUR_GOOGLE_CLIENT_ID.apps.googleusercontent.com" | gcloud secrets create GOOGLE_CLIENT_ID --data-file=-
```

**Secret 3 — Google OAuth Client Secret:**
```powershell
"YOUR_GOOGLE_CLIENT_SECRET" | gcloud secrets create GOOGLE_CLIENT_SECRET --data-file=-
```

**Secret 4 — Session Secret (auto-generated):**
```powershell
python -c "import secrets; print(secrets.token_hex(32), end='')" | gcloud secrets create SESSION_SECRET --data-file=-
```

**Secret 5 — Redis URL:**
```powershell
"redis://default:YOUR_PASSWORD@your-host.upstash.io:6379" | gcloud secrets create REDIS_URL --data-file=-
```
Replace with your actual Upstash Redis URL from Phase 2.

> **If you see "already exists" error:** That's fine! Update it with:
> ```powershell
> "new_value" | gcloud secrets versions add SECRET_NAME --data-file=-
> ```

✅ **Done!** All secrets are safely stored.

---

## Phase 5: Build & Deploy the Python AI Agent

### Step 5.1: Build the Docker Image

Make sure Docker Desktop is running, then:

```powershell
cd d:\last_minute_life_saver

docker build -t us-central1-docker.pkg.dev/YOUR_PROJECT_ID/app-repo/python-agent:latest -f services/python-agent/Dockerfile .
```

> **Replace `YOUR_PROJECT_ID`** with your actual project ID from Phase 3.
>
> **The `.` at the end is important!** It tells Docker to use the current folder as the build context.

⏳ First build takes 2–5 minutes (downloading Python, installing packages).

### Step 5.2: Push the Image to Google Cloud

```powershell
docker push us-central1-docker.pkg.dev/YOUR_PROJECT_ID/app-repo/python-agent:latest
```

⏳ This takes 1–3 minutes (uploading the image).

### Step 5.3: Deploy to Cloud Run

```powershell
gcloud run deploy python-agent --image us-central1-docker.pkg.dev/YOUR_PROJECT_ID/app-repo/python-agent:latest --region us-central1 --port 50051 --use-http2 --update-env-vars GCP_PROJECT=YOUR_PROJECT_ID,GCP_LOCATION=us-central1,GEMINI_MODEL=gemini-2.0-flash --no-allow-unauthenticated --min-instances=1
```

**What these flags mean (so you understand):**
| Flag | What It Does |
|:-----|:------------|
| `--port 50051` | The Python agent listens on port 50051 |
| `--use-http2` | Required for gRPC communication |
| `--no-allow-unauthenticated` | Only the Go Gateway can call it (security) |
| `--min-instances=1` | Keeps one copy always running (no slow cold starts) |

### Step 5.4: Get the Python Agent's URL

```powershell
gcloud run services describe python-agent --region us-central1 --format="value(status.url)"
```

You'll see something like: `https://python-agent-abc123-uc.a.run.app`

📝 **Write this URL down.** You need it in the next phase.

### Step 5.5: Allow Go Gateway to Call This Service

The Go Gateway needs permission to invoke the Python Agent. First, get the default compute service account email:

```powershell
gcloud iam service-accounts list --format="value(email)" --filter="displayName:'Compute Engine default'"
```

Then grant it permission:

```powershell
gcloud run services add-iam-policy-binding python-agent --region us-central1 --member="serviceAccount:YOUR_COMPUTE_SA_EMAIL" --role="roles/run.invoker"
```

✅ **Done!** The AI Agent is live in the cloud.

---

## Phase 6: Build & Deploy the Go Gateway

### Step 6.1: Build the Docker Image

```powershell
cd d:\last_minute_life_saver

docker build -t us-central1-docker.pkg.dev/YOUR_PROJECT_ID/app-repo/go-gateway:latest -f services/go-gateway/Dockerfile services/go-gateway/
```

⏳ First build takes 2–5 minutes.

### Step 6.2: Push to Google Cloud

```powershell
docker push us-central1-docker.pkg.dev/YOUR_PROJECT_ID/app-repo/go-gateway:latest
```

### Step 6.3: Deploy to Cloud Run

This is the biggest command. Replace **ALL** placeholder values:

```powershell
gcloud run deploy go-gateway --image us-central1-docker.pkg.dev/YOUR_PROJECT_ID/app-repo/go-gateway:latest --region us-central1 --port 8080 --update-env-vars "CONVEX_URL=https://precise-hornet-895.eu-west-1.convex.cloud,DASHBOARD_URL=https://placeholder.vercel.app,GMAIL_PUBSUB_TOPIC=projects/YOUR_PROJECT_ID/topics/gmail-watch-notifications,PYTHON_AGENT_ADDR=python-agent-abc123-uc.a.run.app:443,KMS_KEY_ID=mock://local-dev-mock-kms-passphrase-32b,DISABLE_OIDC_VALIDATION=false" --update-secrets "CONVEX_DEPLOY_KEY=CONVEX_DEPLOY_KEY:latest,GOOGLE_CLIENT_ID=GOOGLE_CLIENT_ID:latest,GOOGLE_CLIENT_SECRET=GOOGLE_CLIENT_SECRET:latest,SESSION_SECRET=SESSION_SECRET:latest,REDIS_URL=REDIS_URL:latest" --allow-unauthenticated --min-instances=1
```

**Things to replace:**
| Placeholder | Replace With |
|:------------|:------------|
| `YOUR_PROJECT_ID` | Your Google Cloud project ID |
| `python-agent-abc123-uc.a.run.app` | The Python Agent URL from Phase 5.4 |
| `https://precise-hornet-895.eu-west-1.convex.cloud` | Your production Convex URL from Phase 1.2 |

> **Note:** We set `DASHBOARD_URL` to a placeholder for now. We'll update it after deploying the frontend in Phase 8.

### Step 6.4: Get the Go Gateway's URL

```powershell
gcloud run services describe go-gateway --region us-central1 --format="value(status.url)"
```

📝 **Write this URL down!** It looks like: `https://go-gateway-xyz789-uc.a.run.app`

### Step 6.5: Update Google OAuth Redirect URL

1. Go to https://console.cloud.google.com/apis/credentials
2. Find and click on your **OAuth 2.0 Client ID**
3. Under **Authorized JavaScript origins**, add:
   ```
   https://go-gateway-xyz789-uc.a.run.app
   ```
4. Under **Authorized redirect URIs**, add:
   ```
   https://go-gateway-xyz789-uc.a.run.app/auth/callback
   ```
5. Click **Save**

✅ **Done!** The Go Gateway is live.

---

## Phase 7: Set Up Pub/Sub (Gmail Email Notifications)

When someone sends you an email, Gmail needs a way to tell your Go Gateway. That's what Pub/Sub does.

### Step 7.1: Create the Pub/Sub Topic

```powershell
gcloud pubsub topics create gmail-watch-notifications
```

### Step 7.2: Give Gmail Permission to Send Notifications

```powershell
gcloud pubsub topics add-iam-policy-binding gmail-watch-notifications --member="serviceAccount:gmail-api-push@system.gserviceaccount.com" --role="roles/pubsub.publisher"
```

### Step 7.3: Create a Push Subscription

This tells Pub/Sub: "When you get a message, forward it to the Go Gateway":

```powershell
gcloud pubsub subscriptions create gmail-watch-subscription --topic=gmail-watch-notifications --push-endpoint=https://go-gateway-xyz789-uc.a.run.app/webhooks/gmail --ack-deadline=30
```

Replace `go-gateway-xyz789-uc.a.run.app` with your actual Go Gateway URL from Phase 6.4.

### Step 7.4: Verify Your Domain (Required for Gmail Watch)

Gmail only sends notifications to domains that Google has verified you own.

1. Go to https://console.cloud.google.com/apis/credentials/domainverification
2. Click **"Add domain"**
3. Enter your Cloud Run domain: `go-gateway-xyz789-uc.a.run.app`
4. Follow Google's verification steps

> **Tip:** Cloud Run `*.run.app` domains might need special handling. If you get stuck, you can set up a custom domain later.

### Step 7.5: Create Cloud Tasks Queue

This is used for scheduling deferred task execution:

```powershell
gcloud tasks queues create schedule-trigger-queue --location=us-central1
```

✅ **Done!** Gmail can now notify your app when new emails arrive.

---

## Phase 8: Deploy the Frontend to Vercel

### Step 8.1: Push Your Code to GitHub

If you haven't already:

```powershell
cd d:\last_minute_life_saver
git init
git add -A
git commit -m "Initial deployment"
```

Then create a repository on https://github.com/new and push:

```powershell
git remote add origin https://github.com/YOUR_USERNAME/last-minute-life-saver.git
git branch -M main
git push -u origin main
```

### Step 8.2: Connect to Vercel

1. Go to https://vercel.com/ and sign up with your GitHub account
2. Click **"Add New Project"**
3. Find and import your `last-minute-life-saver` repository
4. **Configure the build settings:**

| Setting | Value |
|:--------|:------|
| **Framework Preset** | SvelteKit (Vercel usually auto-detects this) |
| **Root Directory** | Click "Edit" and type `apps/web` |
| **Build Command** | `pnpm run build` |

### Step 8.3: Add Environment Variables

Before clicking Deploy, click **"Environment Variables"** and add:

| Variable Name | Value |
|:-------------|:------|
| `PUBLIC_CONVEX_URL` | Your Convex URL (e.g. `https://precise-hornet-895.eu-west-1.convex.cloud`) |
| `PUBLIC_GATEWAY_URL` | Your Go Gateway URL (e.g. `https://go-gateway-xyz789-uc.a.run.app`) |
| `PUBLIC_ENV` | `production` |
| `SESSION_SECRET` | The same session secret used by Go Gateway (run `gcloud secrets versions access latest --secret=SESSION_SECRET` to retrieve it) |

### Step 8.4: Deploy!

Click the **Deploy** button. Wait 1–2 minutes.

Vercel will give you a URL like: `https://last-minute-life-saver.vercel.app`

📝 **Write this URL down.**

### Step 8.5: Update Go Gateway with Your Vercel URL

Now go back to PowerShell and update the Go Gateway to know about your frontend:

```powershell
gcloud run services update go-gateway --region us-central1 --update-env-vars DASHBOARD_URL=https://last-minute-life-saver.vercel.app
```

Also go back to https://console.cloud.google.com/apis/credentials and add your Vercel URL to the **Authorized JavaScript origins**:
```
https://last-minute-life-saver.vercel.app
```

✅ **Done!** Your website is live!

---

## Phase 9: Test Everything 🎉

### Step 9.1: Open Your Website

Go to your Vercel URL in a browser (e.g. `https://last-minute-life-saver.vercel.app`)

### Step 9.2: Sign In

1. Click **"Sign in with Google"**
2. You should be redirected to Google → log in → redirected back to your dashboard
3. If it works, you'll see the dashboard with your energy score slider

### Step 9.3: Sync Workspace

1. Click the **"🔄 Sync Workspace"** button
2. This syncs your Google Tasks and activates Gmail watching
3. You should see a success toast notification

### Step 9.4: Test Email Processing

1. From a **different** Gmail account, send an email to the account you signed in with
2. Wait 30–60 seconds (Pub/Sub has a small delay)
3. You should see an **Action Card** appear on your dashboard!
4. The AI agent will have:
   - Analyzed the email content
   - Created a draft reply in your Gmail
   - Assigned a priority score

---

## Troubleshooting — Common Problems

| Problem | What's Wrong | How to Fix |
|:--------|:------------|:-----------|
| **"Sign in" goes to a blank page or error** | OAuth redirect URL mismatch | Add your Go Gateway URL + `/auth/callback` to Google OAuth Console under "Authorized redirect URIs" |
| **Dashboard shows but no tasks appear** | Convex connection issue | Check that `PUBLIC_CONVEX_URL` in Vercel matches your Convex production URL |
| **No action cards after sending email** | Pub/Sub not delivering | Run `gcloud run services logs read go-gateway --region us-central1 --limit=50` to check logs |
| **"unauthorized" errors** | Session cookie mismatch | Make sure `SESSION_SECRET` is identical in Go Gateway secrets AND Vercel env vars |
| **Python Agent returning default responses** | Missing GCP Project env | Verify `GCP_PROJECT` is set correctly in the Python Agent's Cloud Run env |
| **Gmail watch fails** | Domain not verified | Complete domain verification at https://console.cloud.google.com/apis/credentials/domainverification |

### How to Check Logs (Your Best Debugging Friend)

```powershell
# See Go Gateway logs (last 50 entries)
gcloud run services logs read go-gateway --region us-central1 --limit=50

# See Python Agent logs (last 50 entries)
gcloud run services logs read python-agent --region us-central1 --limit=50

# See Pub/Sub subscription status
gcloud pubsub subscriptions describe gmail-watch-subscription
```

---

## Cost Estimate

| Service | Free Tier | Your Estimated Monthly Cost |
|:--------|:---------|:--------------------------|
| **Convex** | 1M function calls/month | $0 |
| **Upstash Redis** | 10K commands/day | $0 |
| **Cloud Run** (Go Gateway) | 2M requests/month, 360K vCPU-seconds | $0–2 |
| **Cloud Run** (Python Agent) | 2M requests/month, 360K vCPU-seconds | $0–2 |
| **Vercel** | 100GB bandwidth/month | $0 |
| **Pub/Sub** | 10GB/month | $0 |
| **Vertex AI (Gemini)** | Pay per token | $0–1 (very low for email triage) |
| **Total** | | **~$0–5/month** |

---

## Quick Reference: All Your URLs & Keys

After deployment, fill in this table and keep it safe:

| Item | Your Value |
|:-----|:----------|
| **Vercel Frontend URL** | `https://_____.vercel.app` |
| **Go Gateway Cloud Run URL** | `https://go-gateway-_____-uc.a.run.app` |
| **Python Agent Cloud Run URL** | `https://python-agent-_____-uc.a.run.app` |
| **Convex Production URL** | `https://_____.convex.cloud` |
| **Google Cloud Project ID** | `_____` |
| **Upstash Redis URL** | `redis://default:_____@_____.upstash.io:6379` |
