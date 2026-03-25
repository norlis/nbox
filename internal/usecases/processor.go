package usecases

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

type Processor struct {
	tmpl       string
	subPattern string
	vars       []string
}

const (
	ExpressionSingle = `{([^{}]*)}`
	ExpressionDouble = `{{(.*?)}}` // ExpressionDouble double curly braces
)

var regexDouble = regexp.MustCompile(ExpressionDouble)

func NewProcessor(tmpl string) *Processor {
	processor := &Processor{
		tmpl:       tmpl,
		subPattern: ExpressionDouble,
	}
	processor.vars = processor.populateVars()
	return processor
}

func (p *Processor) populateVars() []string {
	matches := regexDouble.FindAllStringSubmatch(p.tmpl, -1)
	vars := make([]string, 0, len(matches))
	for _, s := range matches {
		vars = append(vars, s[1])
	}
	return vars
}

func (p *Processor) GetVars() []string {
	return p.vars
}

func (p *Processor) GetKeys() []string {
	vars := make([]string, 0, len(p.vars))
	for _, k := range p.vars {
		vars = append(vars, strings.TrimSpace(k))
	}
	return vars
}

func (p *Processor) GetPrefixes() []string {
	prefixes := map[string]bool{}
	var k []string
	for _, v := range p.vars {
		cleaned := strings.TrimSpace(v)
		prefix := path.Dir(cleaned)
		if prefix == "." {
			prefix = ""
		}
		if _, ok := prefixes[prefix]; !ok {
			k = append(k, prefix)
			prefixes[prefix] = true
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

// GetMissingVars retorna una lista de variables requeridas por el template
// que no están presents en el mapa de valores proporcionado.
// solo se aplica para variables q no existan
// si una variable tiene valor vacio se toma como existente.
func (p *Processor) GetMissingVars(values map[string]string) []string {
	var missing []string
	for _, v := range p.GetKeys() {
		if _, ok := values[v]; !ok {
			missing = append(missing, v)
		}
	}
	return missing
}
