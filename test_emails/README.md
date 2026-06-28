# 📧 Email Integration Testing Guide

This directory contains test emails and tools to test the ingestion, triage, and execution workflows in **The Last-Minute Life Saver**.

## Table of Contents
- [Test Email Templates](#test-email-templates)
  - [Email 1: Gmail Reply Draft (GMAIL_DRAFT)](#email-1-gmail-reply-draft-gmail_draft)
  - [Email 2: Calendar Slot Booking (CALENDAR_BOOKING)](#email-2-calendar-slot-booking-calendar_booking)
  - [Email 3: Bill Payment Checkout (BILL_PAY)](#email-3-bill-payment-checkout-bill_pay)
- [How to Run the Tests](#how-to-run-the-tests)
  - [Prerequisites (Local Dev Stack)](#prerequisites-local-dev-stack)
  - [Method A: Real E2E Testing (With Live Pub/Sub)](#method-a-real-e2e-testing-with-live-pubsub)
  - [Method B: Local Webhook Bypass (Recommended for Local Dev)](#method-b-local-webhook-bypass-recommended-for-local-dev)
- [How to Verify Actions](#how-to-verify-actions)

---

## Test Email Templates

These templates are designed to trigger specific agent actions based on the system prompts configured in `triage_agent.py`.

### Email 1: Gmail Reply Draft (GMAIL_DRAFT)
* **File:** [email_1_gmail_draft.txt](file:///d:/last_minute_life_saver/test_emails/email_1_gmail_draft.txt)
* **Goal:** The AI should classify this as needing a text reply, generate a reply draft, and show a card with the reply text.
* **Subject:** `Urgent: Status update on Project Apollo presentation slides`
* **Body:** 
  ```text
  Hi there,

  Hope you are having a productive week.

  I wanted to quickly check in on the progress of the Project Apollo presentation slides. Are they ready for review? The client just emailed asking if we can share a draft copy by the end of today. 

  Please let me know if you need any help finishing them up, or if they are already uploaded to the shared folder.

  Best regards,
  Sarah Jenkins
  Project Lead | Apollo Tech Group
  sarah.jenkins@example.com
  ```

### Email 2: Calendar Slot Booking (CALENDAR_BOOKING)
* **File:** [email_2_calendar_booking.txt](file:///d:/last_minute_life_saver/test_emails/email_2_calendar_booking.txt)
* **Goal:** The AI should classify this as scheduling, extract meeting details (title, description, attendees, time, and location), place a tentative "Ghost Block" on your Google Calendar, and render a booking card.
* **Subject:** `Let's schedule: Project Apollo Architecture Review Sync`
* **Body:**
  ```text
  Hi,

  We need to schedule a 30-minute sync to finalize the Project Apollo architecture. 

  Can we meet tomorrow at 3:15 PM - 3:45 PM to review the final details? Let's hop on a Google Meet. I'd like to invite David and Elena as well to make sure we are all aligned.

  Let me know if that time slot works for you.

  Best,
  Marcus Chen
  Technical Architect | Apollo Tech Group
  marcus.chen@example.com
  david.lee@example.com
  elena.rodriguez@example.com
  ```

### Email 3: Bill Payment Checkout (BILL_PAY)
* **File:** [email_3_bill_payment.txt](file:///d:/last_minute_life_saver/test_emails/email_3_bill_payment.txt)
* **Goal:** The AI should classify this as a bill/invoice, extract the vendor (`Comcast`), the amount (`$75.00`), and due date, and display a payment card.
* **Subject:** `Your Comcast Internet Statement is Ready - Account #849920192`
* **Body:**
  ```text
  Dear Customer,

  Your monthly statement for Comcast High-Speed Internet service is now available online. 

  Account Details:
  - Account Number: 849920192
  - Statement Date: Jun 28, 2026
  - Payment Due Date: Jun 30, 2026
  - Total Amount Due: $75.00

  To avoid service disruption or late fees, please pay the balance by the due date. You can pay with your saved payment method by visiting your online account or clicking the instant payment checkout button on your portal.

  Thank you for being a valued Comcast customer.

  Sincerely,
  Comcast Billing Support
  billing@comcast-billing.com
  ```

---

## How to Run the Tests

### Prerequisites (Local Dev Stack)
Make sure the full local stack is running:
1. **Convex Backend:** `npx convex dev`
2. **Redis:** `docker run -p 6379:6379 redis:alpine`
3. **Python Agent:** `cd services/python-agent && uvicorn main:app --port 50051` (or activate your venv and run `python main.py`)
4. **Go Gateway:** `cd services/go-gateway && go run cmd/server/main.go`
5. **SvelteKit App:** `cd apps/web && npm run dev`
6. Open `http://localhost:5173/` in your browser and sign in via Google OAuth.

---

### Method A: Real E2E Testing (With Live Pub/Sub)
*Use this method only if you have fully deployed the project to Google Cloud and set up verified domain webhook routing for Gmail Pub/Sub.*

1. Log in to the SvelteKit frontend and click **🔄 Sync Workspace** in the top right to start Gmail watching.
2. From a **different** Gmail account (or ask a colleague), send one of the test emails above to the email account you used to log in.
3. Wait 30–60 seconds. The Pub/Sub subscription will capture the new message and trigger your server webhook, creating the corresponding action card on your SvelteKit dashboard.

---

### Method B: Local Webhook Bypass (Recommended for Local Dev)
*Use this method to test locally without configuring Google Cloud Pub/Sub subscriptions or verified domains.*

1. Open the project root `.env` file and append:
   ```env
   DISABLE_OIDC_VALIDATION=true
   ```
   *(This tells the Go Gateway to accept manual webhook triggers without demanding GCP OIDC identity tokens).*
2. Restart the Go Gateway.
3. From a different email account, send one of the test emails to your registered Gmail account. **Wait a few seconds for the email to arrive in your inbox.**
4. Run the helper script from your terminal:
   ```powershell
   python test_emails/trigger_webhook.py --email your_logged_in_email@gmail.com
   ```
5. The script encodes a mock Pub/Sub request and calls your local Go Gateway webhook.
6. The Go Gateway will lookup your Convex profile, fetch the latest email you just sent to your Gmail inbox, run it through the Python triage agent, create the appropriate database tasks/schedules, and broadcast it to SvelteKit!
7. Watch your browser: the action card will appear instantly.

---

## How to Verify Actions

Once an Action Card appears on your dashboard, you can verify if the system successfully carries out the task:

### 1. Gmail Reply Drafts (`GMAIL_DRAFT`)
* **Execution:** On the SvelteKit card, review the pre-written email body, make any edits, and click **"1-Tap Send Response"**.
* **Verification:**
  1. The card should disappear from your dashboard.
  2. Open your registered Gmail account's **Sent** folder in a browser.
  3. You should see the reply sent to the sender of the test email, properly threaded (`In-Reply-To` and `References` headers intact).
  4. In your Convex database dashboard, verify the task's status has transitioned to `COMPLETED`.

### 2. Calendar Booking (`CALENDAR_BOOKING`)
* **Execution:** When the booking card appears, open your Google Calendar.
* **Verification:**
  1. **Before Clicking Confirm (Ghost Block):** Look at your calendar for the proposed slot (e.g., tomorrow at 3:15 PM). You should see a tentative calendar event already placed there.
  2. **1-Tap Confirm Slot:** Click the button on the card. The card will disappear. Go back to Google Calendar: the event status will change to "confirmed" (and any invited attendees will receive calendar invites). The schedule row in Convex will be marked `COMMITTED`.
  3. **Self-Dissolution (Reclaiming Time):** Send another calendar test email. Once the card appears, instead of confirming it, complete the task manually or delete it. The tentative calendar event (Ghost Block) will be **automatically deleted** from your Google Calendar, reclaiming the free slot.

### 3. Bill Payment (`BILL_PAY`)
* **Execution:** View the Comcast payment card, edit the amount if desired, and click **"1-Tap Pay Bill"**.
* **Verification:**
  1. The payment processing is simulated or redirected.
  2. The card disappears, and the task status in Convex transitions to `COMPLETED`.
