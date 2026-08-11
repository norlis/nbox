package approle

import "errors"

var (
	ErrInvalidCredential = errors.New("approle: missing role_id or secret_id")
	ErrRoleNotFound      = errors.New("approle: role not found")
	ErrRoleDisabled      = errors.New("approle: role disabled")
	ErrInvalidSecret     = errors.New("approle: invalid secret_id")
	ErrSourceNotAllowed  = errors.New("approle: source IP not in role's allowed CIDRs")
)
