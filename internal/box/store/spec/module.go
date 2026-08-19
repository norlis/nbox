package spec

import (
	"context"
	"io/fs"
	"log/slog"
	"os"

	"github.com/norlis/httpgate/logging"
	"go.uber.org/fx"
	"nbox/assets/specs"
	"nbox/internal/nbox"
)

// Module exporta las dependencies de boxspec para fx.
var Module = fx.Module("box.spec",
	fx.Provide(
		ProvideLayeredFS,
		fx.Annotate(
			ProvideFSStore,
			fx.As(new(SpecStore)),
		),
		NewCueEngine,
		NewSpecRegistryFromParams,
	),
	fx.Invoke(RegisterLifecycle),
)

// LayeredFSParams for fx dependency injection.
type LayeredFSParams struct {
	fx.In
	Config *nbox.Config
}

// ProvideLayeredFS creates the layered filesystem with DI.
func ProvideLayeredFS(p LayeredFSParams) fs.FS {
	var layers []fs.FS

	externalPath := p.Config.SpecsPath
	if externalPath == "" {
		externalPath = "/etc/nbox/specs"
	}
	if info, err := os.Stat(externalPath); err == nil && info.IsDir() {
		layers = append(layers, os.DirFS(externalPath))
	}

	layers = append(layers, specs.FS)

	return NewLayeredFS(layers...)
}

// FSStoreParams for fx dependency injection.
type FSStoreParams struct {
	fx.In
	FileSystem fs.FS
	Logger     *slog.Logger
}

// ProvideFSStore creates the FSStore with DI.
func ProvideFSStore(p FSStoreParams) *FSStore {
	return NewFSStore(p.FileSystem, p.Logger)
}

// SpecRegistryParams for fx dependency injection.
type SpecRegistryParams struct {
	fx.In
	Store  SpecStore
	Engine SpecEngine
}

// NewSpecRegistryFromParams creates the registry with DI.
func NewSpecRegistryFromParams(p SpecRegistryParams) *SpecRegistry {
	return NewSpecRegistry(p.Store, p.Engine)
}

// LifecycleParams for lifecycle hooks.
type LifecycleParams struct {
	fx.In
	Lifecycle fx.Lifecycle
	Registry  *SpecRegistry
	Logger    *slog.Logger
}

// RegisterLifecycle registers startup/shutdown hooks.
func RegisterLifecycle(p LifecycleParams) {
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := p.Registry.Reload(ctx); err != nil {
				p.Logger.ErrorContext(ctx, "boxspec load failed", logging.Err(err))
			}
			return nil
		},
		OnStop: func(_ context.Context) error {
			p.Logger.Info("boxspec shutdown")
			return nil
		},
	})
}
