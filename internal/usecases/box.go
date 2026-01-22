package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nbox/internal/domain"
	"nbox/internal/domain/models"
)

type BoxUseCase struct {
	templateAdapter domain.TemplateAdapter
	entryAdapter    domain.EntryManager
	pathUseCase     *PathUseCase
}

func NewBox(boxOperation domain.TemplateAdapter, entryOperations domain.EntryManager, pathUseCase *PathUseCase) *BoxUseCase {
	return &BoxUseCase{
		templateAdapter: boxOperation,
		entryAdapter:    entryOperations,
		pathUseCase:     pathUseCase,
	}
}

func (b *BoxUseCase) BuildBox(ctx context.Context, service, stage, template string, args map[string]string) (string, error) {
	var schemaEnum models.SchemaType

	schema, _ := schemaEnum.GetSchemaFromFilename(template)

	box, err := b.templateAdapter.RetrieveBox(ctx, service, stage, template)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve box: %w", err)
	}

	tmpl := b.VarsBuilder(string(box), service, stage, template, args)
	proc := NewProcessor(tmpl)
	prefixes := proc.GetPrefixes()

	tree := map[string]string{}

	for _, k := range prefixes {
		entries, _ := b.entryAdapter.List(ctx, k)
		for _, entry := range entries {
			if k == strings.TrimSpace(entry.Path) {
				p := b.pathUseCase.Concat(k, entry.Key)
				tree[p] = b.transformBySchema(schema, entry.Value)
			}
		}
	}

	return proc.Replace(tree), nil
}

func (b *BoxUseCase) transformBySchema(schemeType models.SchemaType, value string) string {
	switch schemeType {
	case models.JSON:
		// return strings.ReplaceAll(value, `"`, `\"`)
		escaped, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return strings.Trim(string(escaped), `"`)
	default:
		return value
	}
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
