variable "project_id" {
  type        = string
  description = "The GCP Project ID"
}

variable "region" {
  type        = string
  description = "The GCP region for deployments"
  default     = "us-central1"
}

variable "go_gateway_url" {
  type        = string
  description = "The deployed URL of the Go Ingestion Gateway for Pub/Sub push"
}
