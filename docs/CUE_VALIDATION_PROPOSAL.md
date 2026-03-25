# Propuesta: Validación de Templates con BoxSpec

## Resumen Ejecutivo

Esta propuesta describe la implementación de un módulo de validación semántica para templates (Box) en nbox. La arquitectura sigue Clean Architecture con **Self-Describing Schemas** y soporte para **Hot-Reload** sin recompilación.

---

## 1. Arquitectura Final

### 1.1 Principios de Diseño

| Principio | Implementación |
|-----------|----------------|
| **Self-Describing Schemas** | Cada `.cue` contiene `#Meta` con su ID, nombre y patrones de matching |
| **Hot-Reload** | Estrategia de capas `fs.FS`: External (local) > Embedded (default) |
| **Clean Architecture** | Domain (ports) → Adapters (implementations) |
| **Single Source of Truth** | El archivo `.cue` define TODO sobre el schema |

### 1.2 Estructura de Directorios

```
internal/
├── domain/
│   └── boxspec/
│       ├── model.go          # SpecDefinition, ValidationResult, ValidationError
│       ├── ports.go          # SpecStore, SpecEngine interfaces
│       └── registry.go       # SpecRegistry (central registry)
│
├── adapters/
│   └── boxspec/
│       ├── fs_store.go       # FSStore implements SpecStore
│       ├── cue_engine.go     # CueEngine implements SpecEngine
│       └── layered_fs.go     # LayeredFS (external > embed fallback)
│
└── entrypoints/
    └── api/
        └── handlers/
            └── boxspec.go    # HTTP handlers

assets/
└── specs/                    # Embedded CUE schemas (default)
    ├── aws/
    │   └── ecs_task_definition.cue
    └── kubernetes/
        └── deployment.cue

/etc/nbox/specs/              # External CUE schemas (hot-reload, optional)
```

### 1.3 Diagrama de Flujo

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Schema Loading Flow                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Startup / Reload Signal                                                 │
│         │                                                                │
│         ▼                                                                │
│  ┌─────────────────┐                                                     │
│  │  LayeredFS      │  1. Check External FS (/etc/nbox/specs/)            │
│  │  (fs.FS)        │  2. Fallback to Embedded FS (assets/specs/)         │
│  └────────┬────────┘                                                     │
│           │                                                              │
│           ▼                                                              │
│  ┌─────────────────┐                                                     │
│  │  FSRepository   │  Walk all .cue files                                │
│  │  LoadAll()      │  Extract #Meta from each file                       │
│  └────────┬────────┘  Build SpecDefinition with RawContent               │
│           │                                                              │
│           ▼                                                              │
│  ┌─────────────────┐                                                     │
│  │  SpecRegistry   │  Store in memory map (ID → Spec)                    │
│  │  (Registry)     │  Build pattern index for fast lookup                │
│  └─────────────────┘                                                     │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                        Validation Flow                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Request: Validate(filename, content)                                    │
│         │                                                                │
│         ▼                                                                │
│  ┌─────────────────┐                                                     │
│  │  SpecRegistry   │  1. Resolve spec by filename pattern                │
│  │  Resolve()      │  2. If no match → SpecNone (skip validation)        │
│  └────────┬────────┘                                                     │
│           │                                                              │
│           ▼                                                              │
│  ┌─────────────────┐                                                     │
│  │  CueValidator   │  1. Get/compile schema (cached)                     │
│  │  Validate()     │  2. Parse data (JSON/YAML)                          │
│  └────────┬────────┘  3. Unify(#Schema, data)                            │
│           │           4. Return ValidationResult                         │
│           ▼                                                              │
│  ┌─────────────────┐                                                     │
│  │  Result         │  { valid: bool, errors: [...] }                     │
│  └─────────────────┘                                                     │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Domain Layer

### 2.1 Model (model.go)

```go
// internal/domain/boxspec/model.go

package boxspec

// SpecDefinition represents a validation rule loaded from a CUE file.
type SpecDefinition struct {
    ID         string   `json:"id"`         // Unique identifier (e.g., "aws:ecs:task-def-v1")
    Name       string   `json:"name"`       // Human-readable name
    Version    string   `json:"version"`    // Schema version
    MatchFiles []string `json:"matchFiles"` // Glob patterns (e.g., "taskdef*.json")
    RawContent string   `json:"-"`          // CUE source (not exposed in JSON)
}

// ValidationResult contains the result of a validation.
type ValidationResult struct {
    Valid  bool              `json:"valid"`
    Errors []ValidationError `json:"errors,omitempty"`
}

// ValidationError represents a single validation error with path context.
type ValidationError struct {
    Path    string `json:"path"`    // e.g., "containerDefinitions.0.memory"
    Message string `json:"message"`
}

// NewValidationResult creates a successful validation result.
func NewValidationResult() ValidationResult {
    return ValidationResult{Valid: true}
}

// Failed creates a failed validation result with errors.
func Failed(errors ...ValidationError) ValidationResult {
    return ValidationResult{Valid: false, Errors: errors}
}
```

### 2.2 Ports (ports.go)

```go
// internal/domain/boxspec/ports.go

package boxspec

import "context"

// SpecStore loads spec definitions from storage sources (FS, S3, Embed).
type SpecStore interface {
    // LoadAll scans the source and returns all valid specs found.
    LoadAll(ctx context.Context) ([]SpecDefinition, error)
}

// SpecEngine executes validation against loaded specs (CUE, JSON Schema, etc.).
type SpecEngine interface {
    // Validate checks data against a specific spec definition.
    Validate(ctx context.Context, spec SpecDefinition, data []byte, format string) (ValidationResult, error)

    // InvalidateCache clears any cached compiled schemas (called on reload).
    InvalidateCache()
}
```

### 2.3 SpecRegistry (registry.go)

```go
// internal/domain/boxspec/registry.go

package boxspec

import (
    "context"
    "fmt"
    "path/filepath"
    "strings"
    "sync"
)

// SpecRegistry acts as the central registry for spec definitions.
// Keeps specs in memory for fast O(1) access.
type SpecRegistry struct {
    store        SpecStore
    engine       SpecEngine
    specs        map[string]SpecDefinition // ID -> Spec
    patternIndex map[string]string         // pattern -> specID (pre-computed)
    mu           sync.RWMutex
}

// NewSpecRegistry creates a new spec registry.
func NewSpecRegistry(store SpecStore, engine SpecEngine) *SpecRegistry {
    return &SpecRegistry{
        store:        store,
        engine:       engine,
        specs:        make(map[string]SpecDefinition),
        patternIndex: make(map[string]string),
    }
}

// Reload loads (or reloads) all definitions from the store.
// Call this at startup and when hot-reload is triggered.
func (r *SpecRegistry) Reload(ctx context.Context) error {
    loaded, err := r.store.LoadAll(ctx)
    if err != nil {
        return fmt.Errorf("failed to load specs: %w", err)
    }

    r.mu.Lock()
    defer r.mu.Unlock()

    // Rebuild maps
    r.specs = make(map[string]SpecDefinition, len(loaded))
    r.patternIndex = make(map[string]string)

    for _, spec := range loaded {
        r.specs[spec.ID] = spec
        // Build pattern index for fast lookup
        for _, pattern := range spec.MatchFiles {
            r.patternIndex[pattern] = spec.ID
        }
    }

    // Invalidate engine cache
    r.engine.InvalidateCache()

    return nil
}

// GetByID retrieves a spec by its unique ID.
func (r *SpecRegistry) GetByID(id string) (SpecDefinition, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    if spec, ok := r.specs[id]; ok {
        return spec, nil
    }
    return SpecDefinition{}, fmt.Errorf("spec not found: %s", id)
}

// Resolve finds the best matching spec based on filename.
func (r *SpecRegistry) Resolve(filename string) (SpecDefinition, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    base := strings.ToLower(filepath.Base(filename))

    // Check pattern index
    for pattern, specID := range r.patternIndex {
        if matched, _ := filepath.Match(pattern, base); matched {
            if spec, ok := r.specs[specID]; ok {
                return spec, true
            }
        }
    }
    return SpecDefinition{}, false
}

// List returns all available specs (for API).
func (r *SpecRegistry) List() []SpecDefinition {
    r.mu.RLock()
    defer r.mu.RUnlock()

    list := make([]SpecDefinition, 0, len(r.specs))
    for _, spec := range r.specs {
        list = append(list, spec)
    }
    return list
}

// Validate executes validation by delegating to the engine.
func (r *SpecRegistry) Validate(ctx context.Context, specID string, data []byte, format string) (ValidationResult, error) {
    spec, err := r.GetByID(specID)
    if err != nil {
        return ValidationResult{Valid: false}, err
    }
    return r.engine.Validate(ctx, spec, data, format)
}

// ValidateByFilename resolves spec from filename and validates.
func (r *SpecRegistry) ValidateByFilename(ctx context.Context, filename string, data []byte, format string) (ValidationResult, error) {
    spec, found := r.Resolve(filename)
    if !found {
        // No matching spec - skip semantic validation
        return NewValidationResult(), nil
    }
    return r.engine.Validate(ctx, spec, data, format)
}
```

---

## 3. Adapter Layer

### 3.1 Layered FS (layered_fs.go)

```go
// internal/adapters/boxspec/layered_fs.go

package boxspec

import (
    "io/fs"
)

// LayeredFS implements fs.FS with fallback layers.
// External (hot-reload) takes priority over Embedded (default).
type LayeredFS struct {
    layers []fs.FS
}

// NewLayeredFS creates a layered filesystem.
// Layers are checked in order; first match wins.
func NewLayeredFS(layers ...fs.FS) *LayeredFS {
    return &LayeredFS{layers: layers}
}

// Open implements fs.FS.
func (l *LayeredFS) Open(name string) (fs.File, error) {
    for _, layer := range l.layers {
        if layer == nil {
            continue
        }
        f, err := layer.Open(name)
        if err == nil {
            return f, nil
        }
    }
    return nil, fs.ErrNotExist
}

// ReadDir implements fs.ReadDirFS for directory listing.
func (l *LayeredFS) ReadDir(name string) ([]fs.DirEntry, error) {
    seen := make(map[string]bool)
    var result []fs.DirEntry

    for _, layer := range l.layers {
        if layer == nil {
            continue
        }
        if rdfs, ok := layer.(fs.ReadDirFS); ok {
            entries, err := rdfs.ReadDir(name)
            if err != nil {
                continue
            }
            for _, entry := range entries {
                if !seen[entry.Name()] {
                    seen[entry.Name()] = true
                    result = append(result, entry)
                }
            }
        }
    }
    return result, nil
}
```

### 3.2 FS Store (fs_store.go)

```go
// internal/adapters/boxspec/fs_store.go

package boxspec

import (
    "context"
    "fmt"
    "io/fs"
    "path/filepath"

    "cuelang.org/go/cue"
    "cuelang.org/go/cue/cuecontext"

    "nbox/internal/domain/boxspec"
)

// FSStore loads SpecDefinitions from a filesystem.
// Implements boxspec.SpecStore.
type FSStore struct {
    fileSystem fs.FS
}

// NewFSStore creates a new filesystem-based store.
func NewFSStore(fileSystem fs.FS) *FSStore {
    return &FSStore{fileSystem: fileSystem}
}

// LoadAll scans the filesystem and extracts specs from .cue files.
func (s *FSStore) LoadAll(ctx context.Context) ([]boxspec.SpecDefinition, error) {
    var specs []boxspec.SpecDefinition
    cueCtx := cuecontext.New()

    err := fs.WalkDir(s.fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }
        if d.IsDir() || filepath.Ext(path) != ".cue" {
            return nil
        }

        bytes, err := fs.ReadFile(s.fileSystem, path)
        if err != nil {
            return fmt.Errorf("failed to read %s: %w", path, err)
        }
        content := string(bytes)

        // Compile to extract #Meta
        val := cueCtx.CompileString(content)
        if val.Err() != nil {
            // Log but don't stop loading other files
            return fmt.Errorf("invalid CUE syntax in %s: %w", path, val.Err())
        }

        // Look for #Meta block
        metaVal := val.LookupPath(cue.ParsePath("#Meta"))
        if !metaVal.Exists() {
            // Ignore auxiliary files without metadata
            return nil
        }

        // Decode metadata
        var meta struct {
            ID         string   `json:"id"`
            Name       string   `json:"name"`
            Version    string   `json:"version"`
            MatchFiles []string `json:"matchFiles"`
        }
        if err := metaVal.Decode(&meta); err != nil {
            return fmt.Errorf("failed to decode #Meta in %s: %w", path, err)
        }

        specs = append(specs, boxspec.SpecDefinition{
            ID:         meta.ID,
            Name:       meta.Name,
            Version:    meta.Version,
            MatchFiles: meta.MatchFiles,
            RawContent: content,
        })
        return nil
    })

    return specs, err
}
```

### 3.3 CUE Engine with Caching (cue_engine.go)

```go
// internal/adapters/boxspec/cue_engine.go

package boxspec

import (
    "context"
    "fmt"
    "strings"
    "sync"

    "cuelang.org/go/cue"
    "cuelang.org/go/cue/cuecontext"
    "cuelang.org/go/cue/errors"
    cuejson "cuelang.org/go/encoding/json"
    cueyaml "cuelang.org/go/encoding/yaml"

    "nbox/internal/domain/boxspec"
)

// CueEngine implements boxspec.SpecEngine using CUE language.
type CueEngine struct {
    ctx   *cue.Context
    cache map[string]cue.Value // specID -> compiled #Schema
    mu    sync.RWMutex
}

// NewCueEngine creates a new CUE-based engine.
func NewCueEngine() *CueEngine {
    return &CueEngine{
        ctx:   cuecontext.New(),
        cache: make(map[string]cue.Value),
    }
}

// Validate checks data against a CUE specification.
func (e *CueEngine) Validate(ctx context.Context, spec boxspec.SpecDefinition, data []byte, format string) (boxspec.ValidationResult, error) {
    // 1. Get or compile schema (cached)
    schema, err := e.getOrCompile(spec)
    if err != nil {
        return boxspec.ValidationResult{}, err
    }

    // 2. Parse input data
    dataVal, parseErr := e.parseData(data, format)
    if parseErr != nil {
        return boxspec.Failed(boxspec.ValidationError{
            Path:    "parse",
            Message: fmt.Sprintf("failed to parse %s: %v", format, parseErr),
        }), nil
    }

    // 3. Unify schema with data
    unified := schema.Unify(dataVal)

    // 4. Validate
    if err := unified.Validate(); err != nil {
        return e.parseErrors(err), nil
    }

    return boxspec.NewValidationResult(), nil
}

// InvalidateCache clears the compiled schema cache.
func (e *CueEngine) InvalidateCache() {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.cache = make(map[string]cue.Value)
    // Recreate context to free memory
    e.ctx = cuecontext.New()
}

// getOrCompile retrieves a cached schema or compiles it.
func (e *CueEngine) getOrCompile(spec boxspec.SpecDefinition) (cue.Value, error) {
    // Fast path: check cache
    e.mu.RLock()
    if cached, ok := e.cache[spec.ID]; ok {
        e.mu.RUnlock()
        return cached, nil
    }
    e.mu.RUnlock()

    // Slow path: compile and cache
    e.mu.Lock()
    defer e.mu.Unlock()

    // Double-check after acquiring write lock
    if cached, ok := e.cache[spec.ID]; ok {
        return cached, nil
    }

    // Compile the spec
    val := e.ctx.CompileString(spec.RawContent)
    if val.Err() != nil {
        return cue.Value{}, fmt.Errorf("invalid spec definition: %w", val.Err())
    }

    // Extract #Schema
    schema := val.LookupPath(cue.ParsePath("#Schema"))
    if !schema.Exists() {
        return cue.Value{}, fmt.Errorf("spec %s missing #Schema definition", spec.ID)
    }

    e.cache[spec.ID] = schema
    return schema, nil
}

// parseData parses JSON or YAML into a CUE value.
func (e *CueEngine) parseData(data []byte, format string) (cue.Value, error) {
    switch strings.ToLower(format) {
    case "json":
        expr, err := cuejson.Extract("input.json", data)
        if err != nil {
            return cue.Value{}, err
        }
        return e.ctx.BuildExpr(expr), nil

    case "yaml", "yml":
        file, err := cueyaml.Extract("input.yaml", data)
        if err != nil {
            return cue.Value{}, err
        }
        return e.ctx.BuildFile(file), nil

    default:
        return cue.Value{}, fmt.Errorf("unsupported format: %s", format)
    }
}

// parseErrors converts CUE errors to ValidationErrors.
func (e *CueEngine) parseErrors(err error) boxspec.ValidationResult {
    var validationErrors []boxspec.ValidationError

    for _, e := range errors.Errors(err) {
        path := strings.Join(e.Path(), ".")
        msg := e.Message()
        // Clean up message
        msg = strings.ReplaceAll(msg, "#Schema.", "")

        validationErrors = append(validationErrors, boxspec.ValidationError{
            Path:    path,
            Message: msg,
        })
    }

    return boxspec.ValidationResult{
        Valid:  false,
        Errors: validationErrors,
    }
}

// Ensure CueEngine implements boxspec.SpecEngine
var _ boxspec.SpecEngine = (*CueEngine)(nil)
```

---

## 4. Self-Describing CUE Schemas

### 4.1 Schema Structure

Each `.cue` file must contain:
- `#Meta`: Metadata for auto-registration
- `#Schema`: The actual validation rules

```cue
package specs

// #Meta: Metadata for dynamic registration in nbox
#Meta: {
    id:         string        // Unique identifier
    name:       string        // Human-readable name
    version:    string        // Schema version
    matchFiles: [...string]   // Glob patterns for filename matching
}

// #Schema: The validation rules
#Schema: {
    // ... validation constraints
}
```

### 4.2 AWS ECS Task Definition

```cue
// assets/specs/aws/ecs_task_definition.cue

package aws

#Meta: {
    id:         "aws:ecs:task-definition"
    name:       "AWS ECS Task Definition"
    version:    "1.0"
    matchFiles: ["taskdef*.json", "*-task.json", "task-definition*.json"]
}

#Schema: {
    family!: string & =~"^[a-zA-Z0-9_-]+$"

    networkMode: *"awsvpc" | "bridge" | "host" | "none"

    // CPU and Memory must be numeric strings for Fargate
    cpu?:    string & =~"^[0-9]+$"
    memory?: string & =~"^[0-9]+$"

    taskRoleArn?:      string & =~"^arn:aws:iam::"
    executionRoleArn?: string & =~"^arn:aws:iam::"

    containerDefinitions!: [...#Container] & [_, ...]  // At least one

    requiresCompatibilities?: [...("EC2" | "FARGATE")]

    // Cross-field: Fargate requires cpu, memory, and awsvpc
    if requiresCompatibilities != _|_ {
        if list.Contains(requiresCompatibilities, "FARGATE") {
            cpu!:    string
            memory!: string
            networkMode: "awsvpc"
        }
    }
}

#Container: {
    name!:      string & =~"^[a-zA-Z0-9_-]+$"
    image!:     string
    essential:  bool | *true
    cpu?:       int & >=0
    memory?:    int & >0

    portMappings?: [...{
        containerPort!: int & >0 & <=65535
        hostPort?:      int & >0 & <=65535
        protocol:       *"tcp" | "udp"
    }]

    environment?: [...{
        name!:  string
        value!: string
    }]

    secrets?: [...{
        name!:      string
        valueFrom!: string & =~"^arn:aws:(secretsmanager|ssm):"
    }]

    logConfiguration?: {
        logDriver!: "awslogs" | "splunk" | "fluentd" | "json-file" | "syslog"
        options?: {...}
    }

    // If essential, must have logConfiguration
    if essential == true {
        logConfiguration!: _
    }
}
```

### 4.3 Kubernetes Deployment

```cue
// assets/specs/kubernetes/deployment.cue

package kubernetes

#Meta: {
    id:         "k8s:apps:deployment"
    name:       "Kubernetes Deployment"
    version:    "1.0"
    matchFiles: ["deployment*.yaml", "deployment*.yml", "deploy*.yaml"]
}

#Schema: {
    apiVersion!: "apps/v1"
    kind!:       "Deployment"

    metadata!: {
        name!:        string & =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
        namespace?:   string | *"default"
        labels?:      [string]: string
        annotations?: [string]: string
    }

    spec!: {
        replicas?: int & >=0 | *1

        selector!: {
            matchLabels!: [string]: string & {[_]: _}  // At least one
        }

        template!: {
            metadata!: {
                labels!: [string]: string
            }
            spec!: {
                containers!: [...#Container] & [_, ...]
                initContainers?: [...#Container]
                serviceAccountName?: string
                restartPolicy?: *"Always" | "OnFailure" | "Never"
            }
        }

        // selector.matchLabels must match template.metadata.labels
        selector: matchLabels: template.metadata.labels
    }
}

#Container: {
    name!:  string & =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
    image!: string

    ports?: [...{
        containerPort!: int & >0 & <=65535
        protocol?:      *"TCP" | "UDP" | "SCTP"
        name?:          string
    }]

    env?: [...{
        name!:  string
        value?: string
        valueFrom?: {
            secretKeyRef?: {name!: string, key!: string}
            configMapKeyRef?: {name!: string, key!: string}
        }
    }]

    resources?: {
        limits?:   {cpu?: string, memory?: string}
        requests?: {cpu?: string, memory?: string}
    }

    livenessProbe?:  #Probe
    readinessProbe?: #Probe
}

#Probe: {
    httpGet?:   {path!: string, port!: int | string}
    tcpSocket?: {port!: int | string}
    exec?:      {command!: [...string]}
    initialDelaySeconds?: int | *0
    periodSeconds?:       int | *10
    timeoutSeconds?:      int | *1
    failureThreshold?:    int | *3
}
```

---

## 5. Wiring (Dependency Injection)

```go
// cmd/api/main.go or internal/di/provider.go

package main

import (
    "context"
    "embed"
    "io/fs"
    "log"
    "os"

    adapterboxspec "nbox/internal/adapters/boxspec"
    "nbox/internal/domain/boxspec"
)

//go:embed assets/specs/**/*.cue
var embeddedSpecs embed.FS

func ProvideBoxSpecRegistry() *boxspec.SpecRegistry {
    // 1. Build layered filesystem
    var layers []fs.FS

    // External FS (hot-reload) - highest priority
    externalPath := os.Getenv("NBOX_SPECS_PATH")
    if externalPath == "" {
        externalPath = "/etc/nbox/specs"
    }
    if info, err := os.Stat(externalPath); err == nil && info.IsDir() {
        layers = append(layers, os.DirFS(externalPath))
    }

    // Embedded FS (default) - fallback
    embeddedSubFS, _ := fs.Sub(embeddedSpecs, "assets/specs")
    layers = append(layers, embeddedSubFS)

    layeredFS := adapterboxspec.NewLayeredFS(layers...)

    // 2. Create adapters
    store := adapterboxspec.NewFSStore(layeredFS)
    engine := adapterboxspec.NewCueEngine()

    // 3. Create registry
    registry := boxspec.NewSpecRegistry(store, engine)

    // 4. Initial load
    if err := registry.Reload(context.Background()); err != nil {
        log.Printf("Warning: failed to load validation specs: %v", err)
    }

    return registry
}
```

---

## 6. API Endpoints

### 6.1 Handler

```go
// internal/entrypoints/api/handlers/boxspec.go

package handlers

import (
    "encoding/json"
    "net/http"

    "nbox/internal/domain/boxspec"
)

type BoxSpecHandler struct {
    registry *boxspec.SpecRegistry
}

func NewBoxSpecHandler(registry *boxspec.SpecRegistry) *BoxSpecHandler {
    return &BoxSpecHandler{registry: registry}
}

// List godoc
// @Summary     List available validation specs
// @Description Get list of schemas available for template validation
// @Tags        boxspec
// @Produce     json
// @Success     200 {array} boxspec.SpecDefinition
// @Router      /api/v1/boxspec/specs [get]
func (h *BoxSpecHandler) List(w http.ResponseWriter, r *http.Request) {
    specs := h.registry.List()
    json.NewEncoder(w).Encode(specs)
}

// Reload godoc
// @Summary     Reload validation specs
// @Description Hot-reload specs from filesystem without restart
// @Tags        boxspec
// @Produce     json
// @Success     200 {object} map[string]int "count of loaded specs"
// @Router      /api/v1/boxspec/reload [post]
func (h *BoxSpecHandler) Reload(w http.ResponseWriter, r *http.Request) {
    if err := h.registry.Reload(r.Context()); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    specs := h.registry.List()
    json.NewEncoder(w).Encode(map[string]int{"loaded": len(specs)})
}
```

### 6.2 Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/boxspec/specs` | List all available specs |
| POST | `/api/v1/boxspec/reload` | Hot-reload specs from filesystem |

### 6.3 Response Examples

**GET /api/v1/boxspec/specs**
```json
[
  {
    "id": "aws:ecs:task-definition",
    "name": "AWS ECS Task Definition",
    "version": "1.0",
    "matchFiles": ["taskdef*.json", "*-task.json"]
  },
  {
    "id": "k8s:apps:deployment",
    "name": "Kubernetes Deployment",
    "version": "1.0",
    "matchFiles": ["deployment*.yaml", "deploy*.yaml"]
  }
]
```

**POST /api/v1/boxspec/reload**
```json
{
  "loaded": 5
}
```

---

## 7. Deployment Configuration

### 7.1 Kubernetes (Hot-Reload via ConfigMap)

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nbox-specs
data:
  custom-spec.cue: |
    package custom

    #Meta: {
        id:         "custom:my-app"
        name:       "My App Config"
        version:    "1.0"
        matchFiles: ["myapp*.json"]
    }

    #Schema: {
        appName!: string
        port!:    int & >0 & <=65535
    }

---
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nbox
spec:
  template:
    spec:
      containers:
      - name: nbox
        env:
        - name: NBOX_SPECS_PATH
          value: /etc/nbox/specs
        volumeMounts:
        - name: specs
          mountPath: /etc/nbox/specs
      volumes:
      - name: specs
        configMap:
          name: nbox-specs
```

### 7.2 Docker Compose (Local Development)

```yaml
version: '3.8'
services:
  nbox:
    image: nbox:latest
    environment:
      - NBOX_SPECS_PATH=/specs
    volumes:
      - ./local-specs:/specs:ro
```

---

## 8. Roadmap

### Phase 1: Domain Layer
- [x] Create `model.go` (SpecDefinition, ValidationResult)
- [x] Create `ports.go` (SpecStore, SpecEngine interfaces)
- [ ] Create `registry.go` (SpecRegistry with caching)
- [ ] Unit tests

### Phase 2: Adapter Layer
- [ ] Add dependency `cuelang.org/go`
- [ ] Create `layered_fs.go`
- [ ] Create `fs_store.go` (FSStore implements SpecStore)
- [ ] Create `cue_engine.go` (CueEngine implements SpecEngine)
- [ ] Integration tests

### Phase 3: Schemas
- [ ] AWS ECS Task Definition
- [ ] Kubernetes Deployment
- [ ] Kubernetes Service
- [ ] Kubernetes ConfigMap

### Phase 4: API & Integration
- [ ] BoxSpec handlers
- [ ] Hot-reload endpoint
- [ ] Integrate with Box creation flow
- [ ] OpenAPI documentation

---

## 9. Benefits Summary

| Aspect | Benefit |
|--------|---------|
| **Hot-Reload** | Change validation rules without redeploying |
| **Self-Describing** | Single source of truth in `.cue` files |
| **Caching** | Compiled schemas cached for performance |
| **Layered FS** | External overrides embedded defaults |
| **Testability** | Mock SpecStore/SpecEngine interfaces |
| **Extensibility** | Add new specs by dropping `.cue` files |

---

## 10. References

- [CUE Language](https://cuelang.org/)
- [CUE Go API](https://cuelang.org/docs/howto/validate-json-using-go-api/)
- [Cuetorials](https://cuetorials.com/first-steps/validate-configuration/)
- [Go fs.FS](https://pkg.go.dev/io/fs)