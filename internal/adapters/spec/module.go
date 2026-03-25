package spec

import (
	"context"
	"io/fs"
	"os"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"nbox/assets/specs"
	"nbox/internal/application"
	"nbox/internal/domain/boxspec"
)

// Module exports boxspec dependencies for fx.
var Module = fx.Module("boxspec",
	fx.Provide(
		ProvideLayeredFS,
		fx.Annotate(
			ProvideFSStore,
			fx.As(new(boxspec.SpecStore)),
		),
		NewCueEngine,
		NewSpecRegistry,
	),
	fx.Invoke(RegisterLifecycle),
)

// LayeredFSParams for fx dependency injection.
type LayeredFSParams struct {
	fx.In
	Config *application.Config
}

// ProvideLayeredFS creates the layered filesystem with DI.
func ProvideLayeredFS(p LayeredFSParams) fs.FS {
	var layers []fs.FS

	// External FS (hot-reload) - highest priority
	externalPath := p.Config.SpecsPath // from config
	if externalPath == "" {
		externalPath = "/etc/nbox/specs"
	}
	if info, err := os.Stat(externalPath); err == nil && info.IsDir() {
		layers = append(layers, os.DirFS(externalPath))
	}

	// Embedded FS (default) - fallback
	// embeddedSubFS, _ := fs.Sub(embeddedSpecs, "schemas")
	layers = append(layers, specs.FS)

	return NewLayeredFS(layers...)
}

// FSStoreParams for fx dependency injection.
type FSStoreParams struct {
	fx.In
	FileSystem fs.FS
	Logger     *zap.Logger
}

// ProvideFSStore creates the FSStore with DI.
func ProvideFSStore(p FSStoreParams) *FSStore {
	return NewFSStore(p.FileSystem, p.Logger)
}

// SpecRegistryParams for fx dependency injection.
type SpecRegistryParams struct {
	fx.In
	Store  boxspec.SpecStore
	Engine boxspec.SpecEngine
}

// NewSpecRegistry creates the registry with DI.
func NewSpecRegistry(p SpecRegistryParams) *boxspec.SpecRegistry {
	return boxspec.NewSpecRegistry(p.Store, p.Engine)
}

// LifecycleParams for lifecycle hooks.
type LifecycleParams struct {
	fx.In
	Lifecycle fx.Lifecycle
	Registry  *boxspec.SpecRegistry
	Logger    *zap.Logger
}

// RegisterLifecycle registers startup/shutdown hooks.
func RegisterLifecycle(p LifecycleParams) {
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			p.Logger.Info("Loading boxspec definitions...")
			if err := p.Registry.Reload(ctx); err != nil {
				p.Logger.Warn("Failed to load boxspec definitions", zap.Error(err))
				// Non-fatal: app can start without specs
			}
			p.Logger.Info("BoxSpec ready", zap.Int("specs_loaded", len(p.Registry.List())))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			p.Logger.Info("BoxSpec shutdown")
			return nil
		},
	})
}
