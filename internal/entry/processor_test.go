package entry

import (
	"reflect"
	"testing"
)

func TestProcessor_Prefixes(t *testing.T) {
	t.Parallel()

	p := NewProcessor()

	results := p.Prefixes("namespace/env/name")

	if !reflect.DeepEqual(results, []string{"namespace", "namespace/env"}) {
		t.Errorf(`Expected ["namespace", "env"] got: %v`, results)
	}
}

func TestProcessor_PathWithoutKey(t *testing.T) {
	t.Parallel()

	p := NewProcessor()

	result := p.PathWithoutKey("namespace/env/name")

	if result != "namespace/env" {
		t.Errorf(`Expected "namespace/env" got: %v`, result)
	}
}

func TestProcessor_BaseKey(t *testing.T) {
	t.Parallel()

	p := NewProcessor()

	result := p.BaseKey("namespace/env/name")

	if result != "name" {
		t.Errorf(`Expected "name" got: %v`, result)
	}
}

func TestProcessor_Concat(t *testing.T) {
	t.Parallel()

	p := NewProcessor()

	results := make([][]string, 0, 2)

	for _, prefix := range p.Prefixes("namespace/env/name") {
		path := p.PathWithoutKey(prefix)
		key := p.BaseKey(prefix) + "/"

		results = append(results, []string{path, key})
	}

	if !reflect.DeepEqual(results[0], []string{"", "namespace/"}) && !reflect.DeepEqual(results[1], []string{"namespace", "env/"}) {
		t.Errorf(`Expected [["", "namespace/"], ["namespace", "env/"]] got: %v`, results)
	}
}
