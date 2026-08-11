package auth

// Identity is the normalized result of any successful M2M authentication.
// Independent of the method (approle, aws-sts, ...). Consumers
// (nbox JWT issuer, OPA) get a uniform shape.
type Identity struct {
	// Method is the authenticator that produced this identity:
	// "approle", "aws-sts", etc.
	Method string

	// Subject identifies the principal in the method's namespace.
	// For "approle": the role_id (UUID).
	// For "aws-sts" (future): the IAM ARN.
	Subject string

	// Roles are the OPA roles assigned to this principal. nbox uses
	// these to build the JWT claim that drives authz.
	Roles []string

	// Meta carries method-specific extras (display name, AWS account
	// id, region, etc.). Optional; consumers may ignore.
	Meta map[string]string
}
