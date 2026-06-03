// Package testkit is the Go port of the TypeScript event templates that
// used to live at apps/api/scripts/_templates.ts. Same banks, same
// scenarios, same composer - just no Node runtime required.
//
// Consumed by cmd/testsuite (the end-to-end runner) and any future
// cmd/hammer / cmd/genpayload tools.
package testkit

type Category string

const (
	CatIdentity         Category = "identity"
	CatAdmin            Category = "admin"
	CatDataAccess       Category = "data_access"
	CatDataModification Category = "data_modification"
	CatAgent            Category = "agent"
	CatCompliance       Category = "compliance"
	CatVendor           Category = "vendor"
	CatSecurity         Category = "security"
	CatSystem           Category = "system"
)

var Categories = []Category{
	CatIdentity, CatAdmin, CatDataAccess, CatDataModification,
	CatAgent, CatCompliance, CatVendor, CatSecurity, CatSystem,
}

type ActorType string

const (
	ActorHuman          ActorType = "human"
	ActorAIAgent        ActorType = "ai_agent"
	ActorSystem         ActorType = "system"
	ActorServiceAccount ActorType = "service_account"
	ActorExternalVendor ActorType = "external_vendor"
)

type ActionResult string

const (
	ResultAllowed ActionResult = "allowed"
	ResultDenied  ActionResult = "denied"
	ResultError   ActionResult = "error"
	ResultPending ActionResult = "pending"
)

var EventTypes = map[Category][]string{
	CatIdentity: {
		"identity.login.succeeded", "identity.login.failed", "identity.logout",
		"identity.mfa.enabled", "identity.mfa.disabled", "identity.mfa.challenged",
		"identity.password.reset", "identity.password.changed",
		"identity.permission.granted", "identity.permission.revoked", "identity.permission.check",
		"identity.session.expired", "identity.account.locked",
	},
	CatAdmin: {
		"admin.production_access_granted", "admin.production_access_requested",
		"admin.production_access_revoked", "admin.api_key_created", "admin.api_key_revoked",
		"admin.user_impersonated", "admin.user_invited", "admin.user_removed",
		"admin.org_settings_changed", "admin.refund_approved", "admin.approval_granted",
		"admin.approval_denied", "admin.feature_flag_toggled",
	},
	CatDataAccess: {
		"data.accessed", "data.access.denied", "data.read",
		"data.searched", "data.viewed", "data.downloaded",
	},
	CatDataModification: {
		"data.modified", "data.created", "data.deleted",
		"data.exported", "data.imported", "data.shared_external",
	},
	CatAgent: {
		"agent.tool_called", "agent.action_proposed", "agent.action_approved",
		"agent.action_executed", "agent.action_denied", "agent.action_failed",
		"agent.human_review_requested", "agent.data_queried",
		"agent.context_loaded", "agent.completion_returned",
	},
	CatCompliance: {
		"control.check_passed", "control.check_failed", "control.check_skipped",
		"evidence.attached", "exception.opened", "exception.closed",
	},
	CatVendor: {
		"vendor.data_sent", "vendor.data_received", "vendor.webhook_received",
		"vendor.api_called", "vendor.connection_failed",
	},
	CatSecurity: {
		"security.rate_limit_breached", "security.suspicious_activity",
		"security.malware_detected", "security.intrusion_attempt",
		"security.policy_violation", "security.ddos_mitigated",
	},
	CatSystem: {
		"system.backup_completed", "system.backup_failed",
		"system.deployment_completed", "system.deployment_failed",
		"system.cron_executed", "system.health_check",
		"system.scaling_event", "system.maintenance_started",
	},
}

var ActionNames = map[Category][]string{
	CatIdentity: {
		"login", "logout", "mfa.enable", "mfa.disable",
		"password.reset", "password.change",
		"permission.grant", "permission.revoke", "permission.check", "session.expire",
	},
	CatAdmin: {
		"access.grant", "access.request", "access.revoke",
		"api_key.create", "api_key.revoke",
		"user.impersonate", "user.invite", "user.remove",
		"refund.approve", "feature_flag.toggle",
	},
	CatDataAccess: {
		"customer.read", "invoice.read", "report.read",
		"record.search", "table.scan",
	},
	CatDataModification: {
		"customer.update", "customer.delete", "invoice.create",
		"data.export", "data.import", "data.share_external",
	},
	CatAgent: {
		"sql.query", "http.request", "tool.invoke",
		"refund.propose", "refund.execute", "ticket.respond",
		"completion.generate", "request.escalation",
	},
	CatCompliance: {"control.check", "evidence.attach", "exception.open", "exception.close"},
	CatVendor:     {"stripe.customer.sync", "webhook.received", "api.call", "connection.test"},
	CatSecurity:   {"block", "rate_limit", "threat.flagged", "incident.create"},
	CatSystem:     {"backup.complete", "deploy", "cron.run", "healthcheck.run", "scale.event"},
}

// ActionResults is weighted toward allowed (real traffic looks like this).
var ActionResults = []ActionResult{
	ResultAllowed, ResultAllowed, ResultAllowed, ResultAllowed,
	ResultAllowed, ResultAllowed, ResultAllowed,
	ResultDenied, ResultError, ResultPending,
}

var ActionReasons = []string{
	"bad_password", "rate_limited", "mfa_failed", "insufficient_scope",
	"policy_violation", "token_expired", "over_threshold", "low_confidence",
	"manual_override", "unexpected_error", "network_timeout", "invalid_state",
}

var ResourceTypes = []string{
	"customer", "invoice", "report", "session", "user", "role", "refund",
	"ticket", "control", "service", "api_key", "webhook", "database",
	"permission", "organization", "team", "feature_flag", "incident",
	"document", "integration",
}

var Classifications = []string{
	"pii", "customer_data", "phi", "financial", "pci",
	"employee_data", "business_confidential", "public", "internal",
	"ai_agent_action", "data_event", "privileged_admin",
	"identity_event", "vendor_data_egress",
}

var FieldsAccessed = []string{
	"email", "phone", "address", "ssn", "dob", "last_4_card",
	"username", "created_at", "updated_at", "plan_tier", "mrr",
	"first_name", "last_name", "ip", "device_fingerprint", "consent_state",
}

var SourceSystems = []string{
	"production-api", "auth", "ai-platform", "billing", "compliance",
	"edge", "ops", "ci", "platform", "data-warehouse", "crm",
	"internal-tools", "mobile-app",
}

var SourceServices = []string{
	"customer-service", "web", "invoice-agent", "triage-bot", "support-copilot",
	"access-control", "control-runner", "stripe-sync", "rate-limiter",
	"detector", "backup-scheduler", "deployer", "authz", "export-service",
	"crm", "webhook-router", "admin", "mobile-api", "gateway",
}

var Environments = []string{"prod", "staging", "development", "sandbox"}

var Regions = []string{
	"us-east-1", "us-east-2", "us-west-2", "eu-west-1",
	"eu-central-1", "ap-southeast-1", "ap-northeast-1",
}

var PolicyIDs = []string{
	"pol_prod_access_v3", "pol_refund_limit_v2", "pol_data_export_v1",
	"pol_ai_agent_safety_v4", "pol_pii_handling_v2", "pol_vendor_egress_v1",
}

var Roles = []string{
	"member", "admin", "developer", "support",
	"analyst", "finance", "sre", "compliance",
}

var HumanFirstNames = []string{
	"alice", "bob", "carol", "dave", "erin", "frank", "grace",
	"henry", "iris", "jamie", "kayla", "liam", "mia", "noah",
}

var EmailDomains = []string{
	"example.com", "acme-corp.example",
	"starlight-co.example", "northwind-eu.example",
}

var AIAgentNames = []string{
	"Triage Bot", "Invoice Drafter", "Support Copilot", "Refund Analyst",
	"Onboarding Wizard", "Compliance Checker", "Sales Researcher", "Code Reviewer",
}

var SystemNames = []string{
	"scheduler", "cron", "replicator", "webhook-router",
	"cleanup-worker", "event-publisher",
}

var VendorNames = []string{
	"Stripe", "Twilio", "SendGrid", "Snowflake",
	"Mailgun", "Datadog", "Cloudflare",
}
