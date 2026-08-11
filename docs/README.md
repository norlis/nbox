# Documentación OpenAPI (Swagger)

La API de `nbox` se documenta con [swaggo/swag](https://github.com/swaggo/swag):
las anotaciones viven en los handlers (comentarios `// @...`) y se compilan a
los archivos de este directorio.

## Archivos generados (no editar a mano)

| Archivo | Qué es |
|---|---|
| `docs.go` | Spec embebida en el binario (la sirve el Swagger UI). |
| `swagger.json` / `swagger.yaml` | Spec OpenAPI exportada. |

UI interactiva en runtime: **`GET /swagger/`** (puerto `7337`).

## Regenerar

```shell
make docs        # = swag init ... (instala swag con `make tools` si falta)
```

Corré `make docs` cada vez que cambies anotaciones o agregues/quites endpoints,
y commiteá los archivos regenerados.

## Anotaciones en los handlers

Las anotaciones van como comentarios sobre cada handler. Ejemplo real
(`internal/box/handler.go`, `UpsertBoxV2`):

```go
// UpsertBoxV2
// @Summary Upsert template
// @Description insert or update a specific template on s3
// @Tags templates
// @Accept json
// @Produce json
// @Security BasicAuth
// @Security BearerAuth
// @Param service path string true "service name"
// @Param stage path string true "stage name"
// @Param template path string true "template name"
// @Param data body Input true "Template content (Base64 encoded)"
// @Success 200 {object} []string "List of updated paths"
// @Failure 400 {object} problem.Detail "Bad Request"
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/box/{service}/{stage}/{template} [post].
func (h *Handler) UpsertBoxV2(w http.ResponseWriter, r *http.Request) { ... }
```

### Referencia rápida

| Anotación | Para qué |
|---|---|
| `@Summary` / `@Description` | Resumen breve y explicación detallada del endpoint. |
| `@Tags` | Agrupa endpoints (ej. `templates`, `entries`). |
| `@Accept` / `@Produce` | Content-Type de entrada y salida (ej. `json`). |
| `@Security` | Esquema de auth requerido (`BasicAuth`, `BearerAuth`). |
| `@Param` | Parámetro: `<nombre> <in> <tipo> <required> "<desc>"` (`in` = `path`, `query`, `body`, `header`). |
| `@Success` / `@Failure` | Respuestas por código (`{object} Tipo "desc"`). |
| `@Router` | Ruta + método HTTP, ej. `/api/box/{service}/{stage}/{template} [post]`. |

Más detalle del formato: [swag — the swag formatter](https://github.com/swaggo/swag?tab=readme-ov-file#the-swag-formatter).
