package prefix

import "go.uber.org/fx"

// Module provides prefix domain components.
// prefix.Store is wired by the caller (cmd/nbox/main.go) via prefixstore.NewConfigBacked,
// which reads from the shared config snapshot — no separate DynamoDB table needed.
var Module = fx.Module("prefix",
	fx.Provide(NewHandler),
)
