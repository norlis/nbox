# METADATA
# title: Example
# description: Example package with documentation
package policies.rbac

import rego.v1

default allow := false

default action_allowed := false

default role := "anonymous"

role := data.users[input.user] if {
	data.users[input.user] != ""
}

allow if {
	some action in {"GET:/health", "POST:/api/auth/token", "(GET|POST):/swagger/"}
	regex.match(action, input.action)
}

allow if {
	action_allowed
}

action_allowed if {
	required_permissions = data.roles[role]

	some permission in required_permissions
	regex.match(permission, input.action)
}
