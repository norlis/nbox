package box

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nbox/internal/entry"
)

type buildConfig struct {
	Strict bool
}

// BuildOption configures BuildBox behaviour.
type BuildOption func(*buildConfig)

func WithBuildTemplateStrict() BuildOption {
	return func(c *buildConfig) { c.Strict = true }
}

// Renderer builds box templates by resolving {{ var }} placeholders with stored entries.
type Renderer struct {
	templateAdapter Store
	entryAdapter    entry.Manager
	pathProcessor   *entry.Processor
	resolver        *StrategyResolver
}

func NewRenderer(
	boxStore Store,
	entryManager entry.Manager,
	pathProcessor *entry.Processor,
	resolver *StrategyResolver,
) *Renderer {
	return &Renderer{
		templateAdapter: boxStore,
		entryAdapter:    entryManager,
		pathProcessor:   pathProcessor,
		resolver:        resolver,
	}
}

func (r *Renderer) Detail(ctx context.Context, service, stage string) (*Stage, error) {
	result, err := r.templateAdapter.Detail(ctx, service, stage)
	if err != nil {
		return nil, fmt.Errorf("failed to get detail: %w", err)
	}
	return result, nil
}

func (r *Renderer) BuildBox(ctx context.Context, service, stage, template string, args map[string]string, opts ...BuildOption) (string, error) {
	cfg := &buildConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	strategy := r.resolver.Resolve(template)

	raw, err := r.templateAdapter.RetrieveBox(ctx, service, stage, template)
	if err != nil {
		return "", errors.Join(ErrTemplateNotFound, fmt.Errorf("failed to retrieve template %s/%s/%s: %w", service, stage, template, err))
	}

	tmpl := r.VarsBuilder(string(raw), service, stage, template, args)
	proc := NewTemplateProcessor(tmpl)
	keys := proc.GetKeys()
	tree := map[string]string{}

	results, err := r.entryAdapter.RetrieveMany(ctx, keys)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve variables values: %w", err)
	}
	for _, e := range results {
		tree[e.Key] = strategy.Transform(e.Value)
	}

	if cfg.Strict {
		if missing := proc.GetMissingVars(tree); len(missing) > 0 {
			return "", fmt.Errorf("%w: %v", ErrMissingVariables, missing)
		}
	}

	content := proc.Replace(tree)

	if cfg.Strict {
		if vErr := strategy.Validate(content); vErr != nil {
			return "", fmt.Errorf("%w: invalid %s syntax: %w", ErrInvalidSyntax, strategy.SchemaType(), vErr)
		}
	}

	return content, nil
}

func (r *Renderer) VarsBuilder(tmpl, service, stage, template string, args map[string]string) string {
	oldnew := make([]string, 0, 6+(len(args)*2))
	oldnew = append(oldnew,
		":service", service,
		":stage", stage,
		":template", template,
	)
	for k, v := range args {
		oldnew = append(oldnew, ":"+strings.TrimSpace(k), v)
	}
	return strings.NewReplacer(oldnew...).Replace(tmpl)
}

func (r *Renderer) ListVars(ctx context.Context, service, stage, template string) []string {
	raw, err := r.templateAdapter.RetrieveBox(ctx, service, stage, template)
	if err != nil {
		return []string{}
	}
	proc := NewTemplateProcessor(string(raw))
	return proc.GetVars()
}
