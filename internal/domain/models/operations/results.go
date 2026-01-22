package operations

import (
	"encoding/json"
	"nbox/internal/domain/models"
)

type OperationType string

const (
	Created   OperationType = "created"
	Updated   OperationType = "updated"
	Unchanged OperationType = "unchanged" // Útil para operaciones idempotentes
	Failed    OperationType = "failed"    // Útil si quieres registrar fallos explícitos
)

func (ot OperationType) IsValid() bool {
	switch ot {
	case Created, Updated, Unchanged, Failed:
		return true
	}
	return false
}

func (ot OperationType) String() string {
	return string(ot)
}

type Metadata struct {
	Secure bool `json:"secure"`
}

type Result struct {
	Key    string        `json:"key"`
	Action OperationType `json:"action"`
	Err    error         `json:"-"`
	Output *models.Entry `json:"-"`
}

func (r Result) MarshalJSON() ([]byte, error) {
	type Alias Result

	aux := struct {
		Alias
		ErrorMessage string `json:"error,omitempty"`
	}{
		Alias: Alias(r),
	}

	if r.Err != nil {
		aux.ErrorMessage = r.Err.Error()
	}

	return json.Marshal(aux)
}

type Results map[string]Result

func (r Results) Add(key string, action OperationType, err error) {
	r[key] = Result{
		Key:    key,
		Action: action,
		Err:    err,
	}
}

// Helper para facilitar la creación de resultados con Output
func (r Results) AddWithOutput(key string, action OperationType, err error, output *models.Entry) {
	r[key] = Result{
		Key:    key,
		Action: action,
		Err:    err,
		Output: output,
	}
}

func (r Results) HasErrors() bool {
	for _, res := range r {
		if res.Err != nil {
			return true
		}
	}
	return false
}
