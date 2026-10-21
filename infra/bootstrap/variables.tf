variable "aws_region" {
  description = "Region for the state bucket. Keep state close to the stacks that use it."
  type        = string
  default     = "me-central-1"
}

variable "state_bucket_name" {
  description = "Globally unique S3 bucket name for Terraform state, e.g. sovereign-tfstate-7f3a."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9.-]{2,62}$", var.state_bucket_name))
    error_message = "Bucket names must be lowercase, 3-63 characters, and start with a letter or digit."
  }
}
