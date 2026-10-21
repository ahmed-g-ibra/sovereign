output "account_id" {
  description = "Account these roles live in."
  value       = data.aws_caller_identity.current.account_id
}

output "ci_plan_role_arn" {
  description = "Set as the GitHub Actions variable AWS_PLAN_ROLE_ARN."
  value       = aws_iam_role.ci_plan.arn
}

output "ci_deploy_role_arn" {
  description = "Set as the GitHub Actions variable AWS_DEPLOY_ROLE_ARN."
  value       = aws_iam_role.ci_deploy.arn
}

output "github_oidc_provider_arn" {
  description = "The OIDC provider trusted by both CI roles."
  value       = aws_iam_openid_connect_provider.github.arn
}
