package auth

import "go.uber.org/fx"

// Module provides auth domain business logic.
// auth.Store (authstore.NewInMemory) is wired by the caller due to import cycles.
// HTTP handler (TokenHandler) is registered by the transport module.
var Module = fx.Module("auth")
