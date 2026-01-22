package exporter

import "nbox/internal/domain/models"

// Exporter interfaz para exportadores de diferentes formatos.
type Exporter interface {
	Export(entries []models.Entry) ([]byte, error)
}
