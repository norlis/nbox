package authz_test

import data.authz
import rego.v1

# ==============================================================================
# WHITELIST TESTS
# Tests for public endpoints that don't require authentication
# ==============================================================================

test_whitelist_health_endpoint if {
	authz.allow with input as {"payload": {"roles": []}, "action": "GET:/health"}
		with data.whitelist as ["GET:/health$", "POST:/api/auth/token$", "(GET|POST):/swagger/", "GET:/api/static/environments$"]
}

test_whitelist_auth_token_endpoint if {
	authz.allow with input as {"payload": {"roles": []}, "action": "POST:/api/auth/token"}
		with data.whitelist as ["GET:/health$", "POST:/api/auth/token$", "(GET|POST):/swagger/", "GET:/api/static/environments$"]
}

test_whitelist_swagger_get_endpoint if {
	authz.allow with input as {"payload": {"roles": []}, "action": "GET:/swagger/doc.json"}
		with data.whitelist as ["GET:/health$", "POST:/api/auth/token$", "(GET|POST):/swagger/", "GET:/api/static/environments$"]
}

test_whitelist_swagger_post_endpoint if {
	authz.allow with input as {"payload": {"roles": []}, "action": "POST:/swagger/doc.json"}
		with data.whitelist as ["GET:/health$", "POST:/api/auth/token$", "(GET|POST):/swagger/", "GET:/api/static/environments$"]
}

test_whitelist_static_environments_endpoint if {
	authz.allow with input as {"payload": {"roles": []}, "action": "GET:/api/static/environments"}
		with data.whitelist as ["GET:/health$", "POST:/api/auth/token$", "(GET|POST):/swagger/", "GET:/api/static/environments$"]
}

# ==============================================================================
# ANONYMOUS ROLE TESTS
# Default role with minimal access
# ==============================================================================

test_anonymous_can_access_health if {
	authz.allow with input as {"payload": {"roles": ["anonymous"]}, "action": "GET:/health"}
		with data.roles as {"anonymous": {"permissions": ["public:health"]}}
		with data.permissions as {"public:health": {"patterns": ["^GET:/health$", "^GET:/test$"]}}
}

test_anonymous_cannot_access_api if {
	not authz.allow with input as {"payload": {"roles": ["anonymous"]}, "action": "GET:/api/entry/key?v=global/test"}
		with data.roles as {"anonymous": {"permissions": ["public:health"]}}
		with data.permissions as {"public:health": {"patterns": ["^GET:/health$"]}}
}

# ==============================================================================
# VIEWER ROLE TESTS
# Read-only access to non-production environments
# ==============================================================================

test_viewer_can_read_development_entries_key if {
	authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "GET:/api/entry/key?v=development/app/config"}
		with data.roles as {"viewer": {"permissions": ["entries:read:key:non_production"]}}
		with data.permissions as {"entries:read:key:non_production": {"patterns": ["^GET:/api/entry/key\\?v=(development|qa|global)/(.*)"]}}
}

test_viewer_can_read_qa_entries_key if {
	authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "GET:/api/entry/key?v=qa/app/config"}
		with data.roles as {"viewer": {"permissions": ["entries:read:key:non_production"]}}
		with data.permissions as {"entries:read:key:non_production": {"patterns": ["^GET:/api/entry/key\\?v=(development|qa|global)/(.*)"]}}
}

test_viewer_can_read_global_entries_key if {
	authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "GET:/api/entry/key?v=global/app/config"}
		with data.roles as {"viewer": {"permissions": ["entries:read:key:non_production"]}}
		with data.permissions as {"entries:read:key:non_production": {"patterns": ["^GET:/api/entry/key\\?v=(development|qa|global)/(.*)"]}}
}

test_viewer_cannot_read_production_entries_key if {
	not authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "GET:/api/entry/key?v=production/app/config"}
		with data.roles as {"viewer": {"permissions": ["entries:read:key:non_production"]}}
		with data.permissions as {"entries:read:key:non_production": {"patterns": ["^GET:/api/entry/key\\?v=(development|qa|global)/(.*)"]}}
}

test_viewer_can_read_development_entries_prefix if {
	authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "GET:/api/entry/prefix?v=development/app"}
		with data.roles as {"viewer": {"permissions": ["entries:read:prefix:non_production"]}}
		with data.permissions as {"entries:read:prefix:non_production": {"patterns": ["^GET:/api/entry/prefix\\?v=(development|qa|global)/(.*)"]}}
}

test_viewer_cannot_read_production_entries_prefix if {
	not authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "GET:/api/entry/prefix?v=production/app"}
		with data.roles as {"viewer": {"permissions": ["entries:read:prefix:non_production"]}}
		with data.permissions as {"entries:read:prefix:non_production": {"patterns": ["^GET:/api/entry/prefix\\?v=(development|qa|global)/(.*)"]}}
}

test_viewer_can_read_non_production_templates if {
	authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "GET:/api/box/myapp/development/task.json"}
		with data.roles as {"viewer": {"permissions": ["templates:read:non_production"]}}
		with data.permissions as {"templates:read:non_production": {"patterns": ["^GET:/api/box/(.*)/(qa|development)/[^/]+\\.json$"]}}
}

test_viewer_cannot_read_production_templates if {
	not authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "GET:/api/box/myapp/production/task.json"}
		with data.roles as {"viewer": {"permissions": ["templates:read:non_production"]}}
		with data.permissions as {"templates:read:non_production": {"patterns": ["^GET:/api/box/(.*)/(qa|development)/[^/]+\\.json$"]}}
}

test_viewer_can_read_tracking if {
	authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "GET:/api/track/key?v=development/app/config"}
		with data.roles as {"viewer": {"permissions": ["tracking:read"]}}
		with data.permissions as {"tracking:read": {"patterns": ["^GET:/api/track/key\\?v=(.*)"]}}
}

test_viewer_cannot_write_entries if {
	not authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "POST:/api/entry"}
		with data.roles as {"viewer": {"permissions": ["entries:read:key:non_production"]}}
		with data.permissions as {"entries:read:key:non_production": {"patterns": ["^GET:/api/entry/key\\?v=(development|qa|global)/(.*)"]}}
}

test_viewer_cannot_delete_entries if {
	not authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "DELETE:/api/entry/key?v=development/app/config"}
		with data.roles as {"viewer": {"permissions": ["entries:read:key:non_production"]}}
		with data.permissions as {"entries:read:key:non_production": {"patterns": ["^GET:/api/entry/key\\?v=(development|qa|global)/(.*)"]}}
}

# ==============================================================================
# VIEWER_PROD ROLE TESTS
# Read-only access including production
# ==============================================================================

test_viewer_prod_can_read_production_entries_key if {
	authz.allow with input as {"payload": {"roles": ["viewer_prod"]}, "action": "GET:/api/entry/key?v=production/app/config"}
		with data.roles as {"viewer_prod": {"permissions": ["entries:read:key"]}}
		with data.permissions as {"entries:read:key": {"patterns": ["^GET:/api/entry/key\\?v=(.*)"]}}
}

test_viewer_prod_can_read_production_entries_prefix if {
	authz.allow with input as {"payload": {"roles": ["viewer_prod"]}, "action": "GET:/api/entry/prefix?v=production/app"}
		with data.roles as {"viewer_prod": {"permissions": ["entries:read:prefix"]}}
		with data.permissions as {"entries:read:prefix": {"patterns": ["^GET:/api/entry/prefix\\?v=(.*)"]}}
}

test_viewer_prod_can_export_entries if {
	authz.allow with input as {"payload": {"roles": ["viewer_prod"]}, "action": "GET:/api/entry/export?prefix=production&format=json"}
		with data.roles as {"viewer_prod": {"permissions": ["entries:read:export"]}}
		with data.permissions as {"entries:read:export": {"patterns": ["^GET:/api/entry/export\\?(.*)"]}}
}

test_viewer_prod_cannot_write_entries if {
	not authz.allow with input as {"payload": {"roles": ["viewer_prod"]}, "action": "POST:/api/entry"}
		with data.roles as {"viewer_prod": {"permissions": ["entries:read:key"]}}
		with data.permissions as {"entries:read:key": {"patterns": ["^GET:/api/entry/key\\?v=(.*)"]}}
}

# ==============================================================================
# EDITOR ROLE TESTS
# Can read and write entries and templates
# ==============================================================================

test_editor_can_write_templates if {
	authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "POST:/api/box"}
		with data.roles as {"editor": {"permissions": ["templates:write"]}}
		with data.permissions as {"templates:write": {"patterns": ["^POST:/api/box$"]}}
}

test_editor_can_read_template_build if {
	authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "GET:/api/box/myapp/production/task.json/build"}
		with data.roles as {"editor": {"permissions": ["templates:read:build"]}}
		with data.permissions as {"templates:read:build": {"patterns": ["^GET:/api/box/(.*)/(.*)/(.*)/build$"]}}
}

test_editor_can_read_template_vars if {
	authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "GET:/api/box/myapp/production/task.json/vars"}
		with data.roles as {"editor": {"permissions": ["templates:read:vars"]}}
		with data.permissions as {"templates:read:vars": {"patterns": ["^GET:/api/box/(.*)/(.*)/(.*)/vars$"]}}
}

test_editor_can_write_entries if {
	authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "POST:/api/entry"}
		with data.roles as {"editor": {"permissions": ["entries:write"]}}
		with data.permissions as {"entries:write": {"patterns": ["^POST:/api/entry$"]}}
}

test_editor_can_read_entries_key if {
	authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "GET:/api/entry/key?v=production/app/config"}
		with data.roles as {"editor": {"permissions": ["entries:read:key"]}}
		with data.permissions as {"entries:read:key": {"patterns": ["^GET:/api/entry/key\\?v=(.*)"]}}
}

test_editor_can_read_entries_prefix if {
	authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "GET:/api/entry/prefix?v=production/app"}
		with data.roles as {"editor": {"permissions": ["entries:read:prefix"]}}
		with data.permissions as {"entries:read:prefix": {"patterns": ["^GET:/api/entry/prefix\\?v=(.*)"]}}
}

test_editor_cannot_delete_entries if {
	not authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "DELETE:/api/entry/key?v=production/app/config"}
		with data.roles as {"editor": {"permissions": ["entries:write"]}}
		with data.permissions as {"entries:write": {"patterns": ["^POST:/api/entry$"]}}
}

test_editor_cannot_read_secret_values if {
	not authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "GET:/api/entry/secret-value?v=production/app/password"}
		with data.roles as {"editor": {"permissions": ["entries:read:key"]}}
		with data.permissions as {"entries:read:key": {"patterns": ["^GET:/api/entry/key\\?v=(.*)"]}}
}

# ==============================================================================
# SECRETS_READER ROLE TESTS
# Can read plain secret values (sensitive permission)
# ==============================================================================

test_secrets_reader_can_read_secret_values if {
	authz.allow with input as {"payload": {"roles": ["secrets_reader"]}, "action": "GET:/api/entry/secret-value?v=production/app/password"}
		with data.roles as {"secrets_reader": {"permissions": ["secrets:read:value"]}}
		with data.permissions as {"secrets:read:value": {"patterns": ["^GET:/api/entry/secret-value\\?v=(.*)"]}}
}

test_secrets_reader_cannot_write_entries if {
	not authz.allow with input as {"payload": {"roles": ["secrets_reader"]}, "action": "POST:/api/entry"}
		with data.roles as {"secrets_reader": {"permissions": ["secrets:read:value"]}}
		with data.permissions as {"secrets:read:value": {"patterns": ["^GET:/api/entry/secret-value\\?v=(.*)"]}}
}

# ==============================================================================
# MAINTAINER ROLE TESTS
# Can delete entries
# ==============================================================================

test_maintainer_can_delete_entries if {
	authz.allow with input as {"payload": {"roles": ["maintainer"]}, "action": "DELETE:/api/entry/key?v=development/app/config"}
		with data.roles as {"maintainer": {"permissions": ["entries:delete"]}}
		with data.permissions as {"entries:delete": {"patterns": ["^DELETE:/api/entry/key\\?v=(.*)"]}}
}

test_maintainer_cannot_write_entries if {
	not authz.allow with input as {"payload": {"roles": ["maintainer"]}, "action": "POST:/api/entry"}
		with data.roles as {"maintainer": {"permissions": ["entries:delete"]}}
		with data.permissions as {"entries:delete": {"patterns": ["^DELETE:/api/entry/key\\?v=(.*)"]}}
}

# ==============================================================================
# CICD ROLE TESTS
# CI/CD automation access
# ==============================================================================

test_cicd_can_read_templates if {
	authz.allow with input as {"payload": {"roles": ["cicd"]}, "action": "GET:/api/box/myapp/production/task.json"}
		with data.roles as {"cicd": {"permissions": ["templates:read"]}}
		with data.permissions as {"templates:read": {"patterns": ["^GET:/api/box$", "^GET:/api/box/(.*)/(.*)/(.*)$", "^HEAD:/api/box/(.*)/(.*)/(.)"]}}
}

test_cicd_can_build_templates if {
	authz.allow with input as {"payload": {"roles": ["cicd"]}, "action": "GET:/api/box/myapp/production/task.json/build"}
		with data.roles as {"cicd": {"permissions": ["templates:read:build"]}}
		with data.permissions as {"templates:read:build": {"patterns": ["^GET:/api/box/(.*)/(.*)/(.*)/build$"]}}
}

test_cicd_can_read_entries_key if {
	authz.allow with input as {"payload": {"roles": ["cicd"]}, "action": "GET:/api/entry/key?v=production/app/config"}
		with data.roles as {"cicd": {"permissions": ["entries:read:key"]}}
		with data.permissions as {"entries:read:key": {"patterns": ["^GET:/api/entry/key\\?v=(.*)"]}}
}

test_cicd_can_read_entries_prefix if {
	authz.allow with input as {"payload": {"roles": ["cicd"]}, "action": "GET:/api/entry/prefix?v=production/app"}
		with data.roles as {"cicd": {"permissions": ["entries:read:prefix"]}}
		with data.permissions as {"entries:read:prefix": {"patterns": ["^GET:/api/entry/prefix\\?v=(.*)"]}}
}

test_cicd_can_head_templates if {
	authz.allow with input as {"payload": {"roles": ["cicd"]}, "action": "HEAD:/api/box/myapp/production/task.json"}
		with data.roles as {"cicd": {"permissions": ["templates:read"]}}
		with data.permissions as {"templates:read": {"patterns": ["^GET:/api/box$", "^GET:/api/box/(.*)/(.*)/(.*)$", "^HEAD:/api/box/(.*)/(.*)/(.)"]}}
}

test_cicd_cannot_write_entries if {
	not authz.allow with input as {"payload": {"roles": ["cicd"]}, "action": "POST:/api/entry"}
		with data.roles as {"cicd": {"permissions": ["entries:read:key"]}}
		with data.permissions as {"entries:read:key": {"patterns": ["^GET:/api/entry/key\\?v=(.*)"]}}
}

test_cicd_cannot_write_templates if {
	not authz.allow with input as {"payload": {"roles": ["cicd"]}, "action": "POST:/api/box"}
		with data.roles as {"cicd": {"permissions": ["templates:read"]}}
		with data.permissions as {"templates:read": {"patterns": ["^GET:/api/box$", "^GET:/api/box/(.*)/(.*)/(.*)$"]}}
}

# ==============================================================================
# ADMIN ROLE TESTS
# Full system access
# ==============================================================================

test_admin_can_access_everything_get if {
	authz.allow with input as {"payload": {"roles": ["admin"]}, "action": "GET:/api/anything"}
		with data.roles as {"admin": {"permissions": ["admin:full_access"]}}
		with data.permissions as {"admin:full_access": {"patterns": ["(GET|POST|PUT|DELETE|HEAD):/.*"]}}
}

test_admin_can_access_everything_post if {
	authz.allow with input as {"payload": {"roles": ["admin"]}, "action": "POST:/api/entry"}
		with data.roles as {"admin": {"permissions": ["admin:full_access"]}}
		with data.permissions as {"admin:full_access": {"patterns": ["(GET|POST|PUT|DELETE|HEAD):/.*"]}}
}

test_admin_can_access_everything_delete if {
	authz.allow with input as {"payload": {"roles": ["admin"]}, "action": "DELETE:/api/entry/key?v=production/app/config"}
		with data.roles as {"admin": {"permissions": ["admin:full_access"]}}
		with data.permissions as {"admin:full_access": {"patterns": ["(GET|POST|PUT|DELETE|HEAD):/.*"]}}
}

test_admin_can_access_everything_put if {
	authz.allow with input as {"payload": {"roles": ["admin"]}, "action": "PUT:/api/anything"}
		with data.roles as {"admin": {"permissions": ["admin:full_access"]}}
		with data.permissions as {"admin:full_access": {"patterns": ["(GET|POST|PUT|DELETE|HEAD):/.*"]}}
}

test_admin_can_access_everything_head if {
	authz.allow with input as {"payload": {"roles": ["admin"]}, "action": "HEAD:/api/box/myapp/production/task.json"}
		with data.roles as {"admin": {"permissions": ["admin:full_access"]}}
		with data.permissions as {"admin:full_access": {"patterns": ["(GET|POST|PUT|DELETE|HEAD):/.*"]}}
}

# ==============================================================================
# MULTIPLE ROLES TESTS
# Tests for users with multiple roles combined
# ==============================================================================

test_multiple_roles_viewer_and_secrets_reader if {
	authz.allow with input as {"payload": {"roles": ["viewer", "secrets_reader"]}, "action": "GET:/api/entry/secret-value?v=development/app/password"}
		with data.roles as {
			"viewer": {"permissions": ["entries:read:key:non_production"]},
			"secrets_reader": {"permissions": ["secrets:read:value"]},
		}
		with data.permissions as {
			"entries:read:key:non_production": {"patterns": ["^GET:/api/entry/key\\?v=(development|qa|global)/(.*)"]},
			"secrets:read:value": {"patterns": ["^GET:/api/entry/secret-value\\?v=(.*)"]},
		}
}

test_multiple_roles_editor_and_maintainer_delete if {
	authz.allow with input as {"payload": {"roles": ["editor", "maintainer"]}, "action": "DELETE:/api/entry/key?v=production/app/config"}
		with data.roles as {
			"editor": {"permissions": ["entries:write"]},
			"maintainer": {"permissions": ["entries:delete"]},
		}
		with data.permissions as {
			"entries:write": {"patterns": ["^POST:/api/entry$"]},
			"entries:delete": {"patterns": ["^DELETE:/api/entry/key\\?v=(.*)"]},
		}
}

test_multiple_roles_viewer_prod_and_secrets_reader if {
	authz.allow with input as {"payload": {"roles": ["viewer_prod", "secrets_reader"]}, "action": "GET:/api/entry/secret-value?v=production/app/password"}
		with data.roles as {
			"viewer_prod": {"permissions": ["entries:read:key"]},
			"secrets_reader": {"permissions": ["secrets:read:value"]},
		}
		with data.permissions as {
			"entries:read:key": {"patterns": ["^GET:/api/entry/key\\?v=(.*)"]},
			"secrets:read:value": {"patterns": ["^GET:/api/entry/secret-value\\?v=(.*)"]},
		}
}

# ==============================================================================
# EDGE CASES AND SECURITY TESTS
# Tests for edge cases and potential security issues
# ==============================================================================

test_empty_roles_cannot_access_api if {
	not authz.allow with input as {"payload": {"roles": []}, "action": "GET:/api/entry/key?v=development/app/config"}
		with data.roles as {"viewer": {"permissions": ["entries:read:key"]}}
		with data.permissions as {"entries:read:key": {"patterns": ["^GET:/api/entry/key\\?v=(.*)"]}}
}

test_unknown_role_cannot_access_api if {
	not authz.allow with input as {"payload": {"roles": ["unknown_role"]}, "action": "GET:/api/entry/key?v=development/app/config"}
		with data.roles as {"viewer": {"permissions": ["entries:read:key"]}}
		with data.permissions as {"entries:read:key": {"patterns": ["^GET:/api/entry/key\\?v=(.*)"]}}
}

test_case_sensitive_action_method if {
	not authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "post:/api/entry"}
		with data.roles as {"editor": {"permissions": ["entries:write"]}}
		with data.permissions as {"entries:write": {"patterns": ["^POST:/api/entry$"]}}
}

# ==============================================================================
# PERMISSION INHERITANCE TESTS
# Tests to ensure permission patterns work correctly
# ==============================================================================

test_permission_with_query_params if {
	authz.allow with input as {"payload": {"roles": ["viewer_prod"]}, "action": "GET:/api/entry/export?prefix=production&format=json"}
		with data.roles as {"viewer_prod": {"permissions": ["entries:read:export"]}}
		with data.permissions as {"entries:read:export": {"patterns": ["^GET:/api/entry/export\\?(.*)"]}}
}

test_permission_with_complex_path if {
	authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "GET:/api/box/myservice/production/task-definition.json/build"}
		with data.roles as {"editor": {"permissions": ["templates:read:build"]}}
		with data.permissions as {"templates:read:build": {"patterns": ["^GET:/api/box/(.*)/(.*)/(.*)/build$"]}}
}

test_permission_regex_match_exact if {
	authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "POST:/api/entry"}
		with data.roles as {"editor": {"permissions": ["entries:write"]}}
		with data.permissions as {"entries:write": {"patterns": ["^POST:/api/entry$"]}}
}

test_permission_regex_no_match_with_extra_path if {
	not authz.allow with input as {"payload": {"roles": ["editor"]}, "action": "POST:/api/entry/extra"}
		with data.roles as {"editor": {"permissions": ["entries:write"]}}
		with data.permissions as {"entries:write": {"patterns": ["^POST:/api/entry$"]}}
}

# ==============================================================================
# ENTRYPUSHD ROLE TESTS (Fase 2.1.a)
# Service account: read-only access to entry prefix listings
# ==============================================================================

test_entrypushd_can_read_entries_prefix if {
	authz.allow with input as {"payload": {"roles": ["entrypushd"]}, "action": "GET:/api/entry/prefix?v=passbox/banking/"}
		with data.roles as {"entrypushd": {"permissions": ["entries:read:prefix"]}}
		with data.permissions as {"entries:read:prefix": {"patterns": ["^GET:/api/entry/prefix\\?v=(.*)"]}}
}

test_entrypushd_cannot_write_entries if {
	not authz.allow with input as {"payload": {"roles": ["entrypushd"]}, "action": "POST:/api/entry"}
		with data.roles as {"entrypushd": {"permissions": ["entries:read:prefix"]}}
		with data.permissions as {"entries:read:prefix": {"patterns": ["^GET:/api/entry/prefix\\?v=(.*)"]}}
}

test_entrypushd_cannot_read_secret_values if {
	not authz.allow with input as {"payload": {"roles": ["entrypushd"]}, "action": "GET:/api/entry/secret-value?v=passbox/foo"}
		with data.roles as {"entrypushd": {"permissions": ["entries:read:prefix"]}}
		with data.permissions as {"entries:read:prefix": {"patterns": ["^GET:/api/entry/prefix\\?v=(.*)"]}}
}

# ==============================================================================
# UI HINTS + SELF-SERVICE TESTS
# ==============================================================================

test_permissions_for_viewer if {
	perms := authz.permissions with input as {"payload": {"roles": ["viewer"]}}
		with data.roles as {"viewer": {"permissions": ["tracking:read", "entries:read:key:non_production"]}}
	perms == {"tracking:read", "entries:read:key:non_production"}
}

test_allowed_resources_for_viewer if {
	res := authz.allowed_resources with input as {"payload": {"roles": ["viewer"]}}
		with data.roles as {"viewer": {"permissions": ["tracking:read"]}}
		with data.permissions as {"tracking:read": {"patterns": ["^GET:/api/track/(.*)$"]}}
	res == {"^GET:/api/track/(.*)$"}
}

test_self_service_permissions_allowed_for_authenticated if {
	authz.allow with input as {"payload": {"roles": ["viewer"]}, "action": "GET:/api/me/permissions"}
}

test_self_service_permissions_denied_for_anonymous if {
	not authz.allow with input as {"payload": {"roles": []}, "action": "GET:/api/me/permissions"}
		with data.whitelist as []
		with data.roles as {}
		with data.permissions as {}
}

# ==============================================================================
# ENTRYPUSHD VAULT PERMISSION TESTS (Fase 2.1.d)
# secrets:read:value:vault — resolve plaintext ONLY for passbox/ keys
# ==============================================================================

vault_perm := {"secrets:read:value:vault": {"patterns": ["^GET:/api/entry/resolve\\?v=passbox(/|%2[Ff])(.*)"]}}

vault_role := {"entrypushd": {"permissions": ["secrets:read:value:vault"]}}

test_entrypushd_can_resolve_passbox if {
	authz.allow with input as {
		"action": "GET:/api/entry/resolve?v=passbox/banking/api",
		"payload": {"roles": ["entrypushd"]},
	}
		with data.roles as vault_role
		with data.permissions as vault_perm
}

# The entrypushd client url-encodes slashes (%2F); the action carries the
# raw query, so the pattern must accept the encoded form too.
test_entrypushd_can_resolve_passbox_url_encoded if {
	authz.allow with input as {
		"action": "GET:/api/entry/resolve?v=passbox%2Fbanking%2Fapi",
		"payload": {"roles": ["entrypushd"]},
	}
		with data.roles as vault_role
		with data.permissions as vault_perm
}

test_entrypushd_cannot_resolve_production if {
	not authz.allow with input as {
		"action": "GET:/api/entry/resolve?v=production/banking/api",
		"payload": {"roles": ["entrypushd"]},
	}
		with data.roles as vault_role
		with data.permissions as vault_perm
}

test_entrypushd_cannot_resolve_production_url_encoded if {
	not authz.allow with input as {
		"action": "GET:/api/entry/resolve?v=production%2Fbanking%2Fapi",
		"payload": {"roles": ["entrypushd"]},
	}
		with data.roles as vault_role
		with data.permissions as vault_perm
}

test_entrypushd_vault_perm_does_not_match_secret_value_alias if {
	not authz.allow with input as {
		"action": "GET:/api/entry/secret-value?v=passbox/banking/api",
		"payload": {"roles": ["entrypushd"]},
	}
		with data.roles as vault_role
		with data.permissions as vault_perm
}

# ==============================================================================
# ENTRIES:READ:PREFIX + &leaves FLAG TESTS (Task B)
# entrypushd sends ?leaves=true&v=... (alphabetical order via resty).
# The updated pattern ^GET:/api/entry/prefix\?(.*&)?v=(.*) must accept both
# the plain ?v=... form and the ?leaves=true&v=... form.
# ==============================================================================

# entries:read:prefix debe autorizar el flag &leaves aunque el query encodeado
# ponga "leaves" antes de "v" (resty ordena los params alfabéticamente).
test_entries_read_prefix_allows_leaves_flag if {
	authz.allow with input as {
		"action": "GET:/api/entry/prefix?leaves=true&v=passbox/av",
		"payload": {"roles": ["entrypushd"]},
	}
		with data.roles as {"entrypushd": {"permissions": ["entries:read:prefix"]}}
		with data.permissions as {"entries:read:prefix": {"patterns": ["^GET:/api/entry/prefix\\?(.*&)?v=(.*)"]}}
}

test_entries_read_prefix_still_allows_plain if {
	authz.allow with input as {
		"action": "GET:/api/entry/prefix?v=passbox/av",
		"payload": {"roles": ["entrypushd"]},
	}
		with data.roles as {"entrypushd": {"permissions": ["entries:read:prefix"]}}
		with data.permissions as {"entries:read:prefix": {"patterns": ["^GET:/api/entry/prefix\\?(.*&)?v=(.*)"]}}
}
