# METADATA
# title: NBOX Authorization Policy
# description: Role-based access control (RBAC) with a public whitelist and UI capability hints
package authz

import rego.v1

# ==============================================================================
# ROLES & GRANTED PERMISSIONS
# Shared building blocks reused by the access decision and the UI hints.
# ==============================================================================

# Caller roles as a set; defaults to {"anonymous"} when the payload carries none.
default roles := {"anonymous"}

# Every authenticated caller also holds the implicit "authenticated" pseudo-role
# (K8s system:authenticated style): base grants live in ONE role, never repeated.
roles := ({role | some role in input.payload.roles} | {"authenticated"}) if {
	count(input.payload.roles) > 0
}

# METADATA
# entrypoint: true
# description: Permission names granted to the caller's roles (data.authz.permissions).
permissions contains perm if {
	some role in roles
	some perm in data.roles[role].permissions
}

# ==============================================================================
# AUTHORIZATION DECISION
# AND within a rule, OR across rules; default deny.
# ==============================================================================

# METADATA
# entrypoint: true
# description: Access decision queried by the Authorize middleware (data.authz.allow).
default allow := false

# public endpoints that don't require authentication
allow if {
	some action in data.whitelist
	regex.match(action, input.action)
}

# a granted permission's pattern matches the requested action
allow if {
	some perm in permissions
	some pattern in data.permissions[perm].patterns
	regex.match(pattern, input.action)
}

# ==============================================================================
# UI HINTS
# Not a security boundary; roles come from the authenticated context.
# ==============================================================================

# METADATA
# entrypoint: true
# description: Resource regex patterns the caller's roles can act on (data.authz.allowed_resources).
allowed_resources contains pattern if {
	some perm in permissions
	some pattern in data.permissions[perm].patterns
}
