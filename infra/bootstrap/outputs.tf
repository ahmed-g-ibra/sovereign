output "state_bucket" {
  description = "Name of the Terraform state bucket."
  value       = aws_s3_bucket.state.id
}

output "backend_config" {
  description = "Paste this into the init command for the main stack."
  value       = <<-CFG
    terraform -chdir=infra/aws init \
      -backend-config="bucket=${aws_s3_bucket.state.id}" \
      -backend-config="key=aws/terraform.tfstate" \
      -backend-config="region=${var.aws_region}"
  CFG
}
