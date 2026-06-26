provider "google" {
  project = var.project_id
  region  = var.region
}

# KMS Keyring & Cryptokey for OAuth token envelope encryption
resource "google_kms_key_ring" "keyring" {
  name     = "last-minute-keyring"
  location = var.region
}

resource "google_kms_crypto_key" "oauth_key" {
  name            = "oauth-token-encryption-key"
  key_ring        = google_kms_key_ring.keyring.id
  rotation_period = "7776000s" # 90 days

  lifecycle {
    prevent_destroy = true
  }
}

# Pub/Sub Topic for Gmail Watch Notifications
resource "google_pubsub_topic" "gmail_watch_topic" {
  name = "gmail-watch-notifications"
}

# Pub/Sub Subscription forwarding events to Go Gateway Cloud Run
resource "google_pubsub_subscription" "gmail_watch_sub" {
  name  = "gmail-watch-subscription"
  topic = google_pubsub_topic.gmail_watch_topic.name

  push_config {
    push_endpoint = var.go_gateway_url
    oidc_token {
      service_account_email = google_service_account.pubsub_invoker.email
    }
  }
}

# Cloud Tasks Queue for scheduled trigger execution
resource "google_cloud_tasks_queue" "task_trigger_queue" {
  name     = "schedule-trigger-queue"
  location = var.region

  rate_limits {
    max_concurrent_dispatches = 100
    max_dispatches_per_second = 500
  }

  retry_config {
    max_attempts       = 5
    min_backoff        = "1s"
    max_backoff        = "30s"
    max_double_backoff = 3
  }
}

# Service Accounts
resource "google_service_account" "pubsub_invoker" {
  account_id   = "pubsub-invoker-sa"
  display_name = "Pub/Sub Invoker Service Account"
}

resource "google_service_account" "gateway_sa" {
  account_id   = "go-gateway-sa"
  display_name = "Go Gateway Service Account"
}

resource "google_service_account" "agent_sa" {
  account_id   = "python-agent-sa"
  display_name = "Python Reasoning Agent Service Account"
}
