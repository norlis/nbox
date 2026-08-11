# Convertir JSON Schema a CUE

Guia paso a paso para convertir un JSON Schema a CUE.

---

## Paso 1: Instalar CUE

```bash
# macOS
brew install cue

# Go
go install cuelang.org/go/cmd/cue@latest

# Verificar instalacion
cue version
```

---

## Paso 2: Descargar el JSON Schema

```bash
# Ejemplo: ECS Task Definition
curl -o task-definition-schema.json \
  https://ecs-intellisense.s3-us-west-2.amazonaws.com/task-definition/schema.json
```

---

## Paso 3: Importar a CUE

```bash
# cue import jsonschema task-definition-schema.json
cue import jsonschema -l '#Schema:' task-definition-schema.json

# eliminar comentarios 
sed -i '' '/^[[:space:]]*\/\//d' task-definition-schema.cue 
```

Esto genera un archivo `task-definition-schema.cue`.

---

## Paso 4: Revisar y ajustar

Abrir el archivo generado y hacer ajustes:

### 4.1 Agregar metadatos `#Meta`

```cue
#Meta: {
    id:         "aws:ecs:task-definition"
    name:       "AWS ECS Task Definition"
    version:    "1.0"
    matchFiles: ["taskdef*.json", "*-task.json"]
}
```



---

## Paso 5: Validar el schema

```bash
# Verificar que el CUE es valido
cue vet ecs_task_definition.cue

# Probar con un archivo JSON real
cue vet ecs_task_definition.cue taskdef.json  -d '#Schema'

bin/cue vet assets/specs/aws/task-definition-schema.cue examples/example-task-definition.json -d '#Schema'

```


## Referencias

- [CUE + JSON Schema](https://cuelang.org/docs/concept/how-cue-works-with-json-schema/)
- [CUE import command](https://cuelang.org/docs/reference/command/cue-help-import/)
- [CUE Language Spec](https://cuelang.org/docs/reference/spec/)
- [AWS::ECS::TaskDefinition](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-ecs-taskdefinition.html)