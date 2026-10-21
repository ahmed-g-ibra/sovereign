variable "aws_region" {
  description = "Primary region for the Sovereign platform."
  type        = string
  default     = "me-central-1"
}

variable "github_owner" {
  description = "GitHub user or organisation that owns the repository."
  type        = string
}

variable "github_repo" {
  description = "Repository name, without the owner."
  type        = string
  default     = "sovereign"
}

variable "state_bucket_name" {
  description = "Terraform state bucket created by the bootstrap stack. CI needs write access to it."
  type        = string
}

variable "budget_limit_usd" {
  description = "Monthly cost ceiling. Alerts fire against this, nothing is enforced."
  type        = number
  default     = 50
}

variable "budget_alert_emails" {
  description = "Addresses that receive budget alerts. AWS sends a confirmation to each."
  type        = list(string)

  validation {
    condition     = length(var.budget_alert_emails) > 0
    error_message = "Set at least one address, or you will find out about the bill from your statement."
  }
}
