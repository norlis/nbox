package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nbox/internal/domain"
	"nbox/internal/domain/strategies"
)

type buildConfig struct {
	Strict bool
}

type BuildOption func(*buildConfig)

func WithBuildTemplateStrict() BuildOption {
	return func(c *buildConfig) {
		c.Strict = true
	}
}

type BoxUseCase struct {
	templateAdapter domain.TemplateAdapter
	entryAdapter    domain.EntryManager
	pathUseCase     *PathUseCase
	resolver        *strategies.StrategyResolver
}

func NewBox(
	boxOperation domain.TemplateAdapter,
	entryOperations domain.EntryManager,
	pathUseCase *PathUseCase,
	resolver *strategies.StrategyResolver,
) *BoxUseCase {
	return &BoxUseCase{
		templateAdapter: boxOperation,
		entryAdapter:    entryOperations,
		pathUseCase:     pathUseCase,
		resolver:        resolver,
	}
}

func (b *BoxUseCase) BuildBox(ctx context.Context, service, stage, template string, args map[string]string, opts ...BuildOption) (string, error) {
	cfg := &buildConfig{
		Strict: false,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	strategy := b.resolver.Resolve(template)

	box, err := b.templateAdapter.RetrieveBox(ctx, service, stage, template)
	if err != nil {
		return "", errors.Join(domain.ErrTemplateNotFound, fmt.Errorf("failed to retrieve template %s/%s/%s: %w", service, stage, template, err))
	}

	tmpl := b.VarsBuilder(string(box), service, stage, template, args)
	proc := NewProcessor(tmpl)
	keys := proc.GetKeys()
	tree := map[string]string{}

	results, err := b.entryAdapter.RetrieveMany(ctx, keys)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve variables values: %w", err)
	}
	for _, entry := range results {
		tree[entry.Key] = strategy.Transform(entry.Value)
		// tree[entry.Key] = b.transformBySchema(schema, entry.Value)
	}

	if cfg.Strict {
		missing := proc.GetMissingVars(tree)
		if len(missing) > 0 {
			return "", fmt.Errorf("%w: %v", domain.ErrMissingVariables, missing)
		}
	}

	content := proc.Replace(tree)

	if cfg.Strict {
		if err = strategy.Validate(content); err != nil {
			return "", fmt.Errorf("%w: invalid %s syntax: %w", domain.ErrInvalidSyntax, strategy.SchemaType(), err)
		}
	}

	return content, nil
}

func (b *BoxUseCase) VarsBuilder(tmpl, service, stage, template string, args map[string]string) string {
	oldnew := []string{
		":service", service,
		":stage", stage,
		":template", template,
	}

	for k, v := range args {
		oldnew = append(oldnew, ":"+strings.TrimSpace(k), v)
	}

	return strings.NewReplacer(oldnew...).Replace(tmpl)
}

func (b *BoxUseCase) ListVars(ctx context.Context, service, stage, template string) []string {
	box, err := b.templateAdapter.RetrieveBox(ctx, service, stage, template)
	if err != nil {
		return []string{}
	}
	proc := NewProcessor(string(box))
	return proc.GetVars()
}
