package usecases

import (
	"reflect"
	"testing"
)

func TestPathUseCase_Prefixes(t *testing.T) {
	t.Parallel()

	useCase := NewPathUseCase()

	results := useCase.Prefixes("namespace/env/name")

	if !reflect.DeepEqual(results, []string{"namespace", "namespace/env"}) {
		t.Errorf(`Expected ["namespace", "env"] got: %v`, results)
	}
}

func TestPathUseCase_PathWithoutKey(t *testing.T) {
	t.Parallel()

	useCase := NewPathUseCase()

	result := useCase.PathWithoutKey("namespace/env/name")

	if result != "namespace/env" {
		t.Errorf(`Expected "namespace/env" got: %v`, result)
	}
}

func TestPathUseCase_BaseKey(t *testing.T) {
	t.Parallel()

	useCase := NewPathUseCase()

	result := useCase.BaseKey("namespace/env/name")

	if result != "name" {
		t.Errorf(`Expected "name" got: %v`, result)
	}
}

func TestPathUseCase(t *testing.T) {
	t.Parallel()

	useCase := NewPathUseCase()

	results := make([][]string, 0, 2)

	for _, prefix := range useCase.Prefixes("namespace/env/name") {
		path := useCase.PathWithoutKey(prefix)
		key := useCase.BaseKey(prefix) + "/"

		results = append(results, []string{path, key})
	}

	if !reflect.DeepEqual(results[0], []string{"", "namespace/"}) && !reflect.DeepEqual(results[1], []string{"namespace", "env/"}) {
		t.Errorf(`Expected [["", "namespace/"], ["namespace", "env/"]] got: %v`, results)
	}
}
