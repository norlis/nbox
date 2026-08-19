// Package logfields defines nbox's domain log field names, complementing
// httpgate/logging (OTel core) and event-driven/logfields (messaging).
// One concept = one name across nbox and entrypushd.
package logfields

const (
	KeyNboxKey      = "nbox.key"      // full entry key (/passbox/...)
	KeyNboxPrefix   = "nbox.prefix"   // routing/export/snapshot prefix
	KeyNboxBackend  = "nbox.backend"  // storage backend type
	KeyNboxService  = "nbox.service"  // box service name
	KeyNboxStage    = "nbox.stage"    // box stage
	KeyNboxTemplate = "nbox.template" // template name
	KeyNboxFormat   = "nbox.format"   // export format (dotenv|json)

	KeyEntriesTotal  = "nbox.entries.total"  // batch summaries
	KeyEntriesFailed = "nbox.entries.failed" // batch summaries

	KeyAuthScheme = "auth.scheme"     // basic | approle | aws-sts
	KeyAppRoleID  = "approle.role_id" // never secret_id
	KeyUserID     = "user.id"         // OTel: authenticated principal
	KeyRPCMethod  = "rpc.method"      // OTel: gRPC method
	KeySourceIP   = "client.address"  // OTel: reuse for gRPC peer too
	KeyFilePath   = "file.path"       // ECS: file path in spec-loading logs

	KeyEventType = "cloudevents.event_type" // reuse of event-driven name
	KeyEventID   = "messaging.message.id"   // reuse of event-driven name
)
