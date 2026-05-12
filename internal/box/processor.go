package box

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Processor parses and resolves {{ var }} template expressions.
type Processor struct {
	tmpl string
	vars []string
}

const (
	ExpressionDouble = `{{(.*?)}}`
)

var regexDouble = regexp.MustCompile(ExpressionDouble)

func NewTemplateProcessor(tmpl string) *Processor {
	p := &Processor{tmpl: tmpl}
	p.vars = p.populateVars()
	return p
}

func (p *Processor) populateVars() []string {
	matches := regexDouble.FindAllStringSubmatch(p.tmpl, -1)
	vars := make([]string, 0, len(matches))
	for _, s := range matches {
		vars = append(vars, s[1])
	}
	return vars
}

func (p *Processor) GetVars() []string { return p.vars }

func (p *Processor) GetKeys() []string {
	keys := make([]string, 0, len(p.vars))
	for _, k := range p.vars {
		keys = append(keys, strings.TrimSpace(k))
	}
	return keys
}

func (p *Processor) GetPrefixes() []string {
	prefixes := make(map[string]struct{})
	var k []string
	for _, v := range p.vars {
		cleaned := strings.TrimSpace(v)
		prefix := path.Dir(cleaned)
		if prefix == "." {
			prefix = ""
		}
		if _, ok := prefixes[prefix]; !ok {
			k = append(k, prefix)
			prefixes[prefix] = struct{}{}
		}
	}
	return k
}

func (p *Processor) Replace(values map[string]string) string {
	oldnew := make([]string, 0, len(p.vars)*2)
	for _, v := range p.vars {
		cleaned := strings.TrimSpace(v)
		oldnew = append(oldnew, fmt.Sprintf(`{{%s}}`, v), values[cleaned])
	}
	return strings.NewReplacer(oldnew...).Replace(p.tmpl)
}

// GetMissingVars returns variables required by the template that are absent from values.
func (p *Processor) GetMissingVars(values map[string]string) []string {
	var missing []string
	for _, v := range p.GetKeys() {
		if _, ok := values[v]; !ok {
			missing = append(missing, v)
		}
	}
	return missing
}
