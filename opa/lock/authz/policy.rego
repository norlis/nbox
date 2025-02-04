# METADATA
# title: Example
# description: Example package with documentation
package authz

import rego.v1

default allow := false

#default is_token_valid := false
default action_allowed := false

roles := data.roles

users := data.users

username := input.payload.username

allow if {
	action_allowed
}

#allow if {
#    input.method == "GET"
#    input.path = "/health"
#}
#
#allow if {
#    input.method == "POST"
#    input.path == "/api/auth/token"
#}
#
#allow if {
#    input.path == "/swagger/*"
#}

#allow if {
##	is_token_valid
#	action_allowed
#}

#is_token_valid if {
##	token.valid
#	now := time.now_ns() / 1000000000
#	token.payload.nbf <= now
#	now < token.payload.exp
#}

action_allowed if {
	some role
	users[input.payload.username][_] == role
	roles[role].permissions[_] == {"method": input.method, "path": input.path}
}

# Helper to get the token payload.
#token := {"payload": payload} if {
#	[header, payload, signature] := io.jwt.decode(input.token)
#}
#token := {"valid": valid, "payload": payload} if {
#	[_, encoded] := split(http.headers.authorization, " ")
#	[valid, _, payload] := io.jwt.decode_verify(encoded, {"secret": "secret"})
#}
