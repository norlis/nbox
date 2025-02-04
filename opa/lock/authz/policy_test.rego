package authz_test

import data.authz

test_delete_allowed if {
	authz.allow with input as {"path": ["/users"], "method": "DELETE", "payload": {"username": "bob"}}
}
