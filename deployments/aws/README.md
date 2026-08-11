# NBOX Infrastructure Deployment

Template SAM/CloudFormation para desplegar la infraestructura de NBOX (DynamoDB + S3).

## 📦 Recursos

- **EntriesTable** - Tabla principal de entries (Path+Key)
- **TrackingEntriesTable** - Tabla de auditoría (Key+Timestamp)
- **BoxTable** - Metadata de boxes (Service+Stage)
- **ConfigTable** - Config dinámica: auth (basic_auth/app_role/arn_map) + prefix-config (kind+id)
- **BoxBucket** - Bucket S3 para templates

## 🚀 Deploy

```bash
# Development
sam validate --lint && sam build && sam deploy --config-env dev

# Production
sam validate --lint && sam build && sam deploy --config-env prod
```

## 🌱 Seed inicial (CLI)

Después del deploy, poblar la config vía el CLI. Prefix-config ahora vive en la tabla
config compartida (`kind=prefix_config`), gestionada con `nbox-cli prefix`.

```bash
export AWS_REGION=us-east-1 NBOX_CONFIG_TABLE_NAME=nbox-config-production

# Ver los prefijos actuales
go run ./cmd/cli prefix list

# Prefijos por entorno (routing de storage)
go run ./cmd/cli prefix upsert --prefix=development --type=dynamodb --secure=parameterstore_secure --allowed=parameterstore,parameterstore_secure
go run ./cmd/cli prefix upsert --prefix=qa          --type=dynamodb --secure=parameterstore_secure --allowed=parameterstore,parameterstore_secure
go run ./cmd/cli prefix upsert --prefix=beta        --type=dynamodb --secure=parameterstore_secure --allowed=parameterstore,parameterstore_secure
go run ./cmd/cli prefix upsert --prefix=sandbox     --type=dynamodb --secure=parameterstore_secure --allowed=parameterstore,parameterstore_secure
go run ./cmd/cli prefix upsert --prefix=production  --type=dynamodb --secure=parameterstore_secure --allowed=parameterstore,parameterstore_secure
go run ./cmd/cli prefix upsert --prefix=global      --type=dynamodb --secure=parameterstore_secure --allowed=parameterstore,parameterstore_secure

# Vault (passbox): fuerza secure
go run ./cmd/cli prefix upsert --prefix=passbox --type=parameterstore_secure --secure=parameterstore_secure

# Prefijos específicos (parameterstore por defecto)
go run ./cmd/cli prefix upsert --prefix=global/av_v2_receiver  --type=parameterstore --secure=parameterstore_secure --allowed=parameterstore_secure
go run ./cmd/cli prefix upsert --prefix=global/serverless      --type=parameterstore --secure=parameterstore_secure --allowed=parameterstore_secure
go run ./cmd/cli prefix upsert --prefix=production/serverless  --type=parameterstore --secure=parameterstore_secure --allowed=parameterstore_secure
go run ./cmd/cli prefix upsert --prefix=development/serverless --type=parameterstore --secure=parameterstore_secure --allowed=parameterstore_secure
go run ./cmd/cli prefix upsert --prefix=development/av2        --type=parameterstore_secure --secure=parameterstore_secure
```

### Usuarios (Basic Auth) y AppRoles (M2M)

```bash
# password aleatorio de ejemplo
SECRET=$(LC_ALL=C tr -dc A-Za-z0-9_ < /dev/urandom | head -c 8 | xargs)

# crear usuario admin
go run ./cmd/cli config --table "${NBOX_CONFIG_TABLE_NAME}" user upsert \
  --username admin --roles admin --password "${SECRET}"

# crear un AppRole (M2M) — todas las opciones:
#   --name    nombre legible (requerido)
#   --roles   roles OPA (lista)              p. ej. entrypushd
#   --cidrs   CIDRs permitidos (opcional)    restringe la IP de origen
#   --cost    bcrypt cost 4-31 (default 10)
go run ./cmd/cli config --table "${NBOX_CONFIG_TABLE_NAME}" approle generate \
  --name watcher \
  --roles entrypushd \
  --cidrs 10.0.0.0/8,172.16.0.0/12 \
  --cost 12
# imprime role_id + secret_id (el secret_id NO se vuelve a mostrar — distribuilo seguro)

# listar / borrar approles
go run ./cmd/cli config --table "${NBOX_CONFIG_TABLE_NAME}" approle list
go run ./cmd/cli config --table "${NBOX_CONFIG_TABLE_NAME}" approle rm <role_id>
```

## ⚙️ Configuración

Las variables de entorno están en `samconfig.yaml`:

```yaml
dev:
  deploy:
    parameters:
      parameter_overrides:
        Environment=development

prod:
  deploy:
    parameters:
      parameter_overrides:
        Environment=production
```

## 📊 Diferencias por Ambiente

| Feature | Development | Production |
|---------|------------|------------|
| Deletion Protection | ❌ | ✅ |
| Point-in-Time Recovery | ❌ | ✅ |
| GlobalTable Replication | Single region | Single region* |

*Para multi-región en prod, agregar más regiones en `Replicas[]`

## 🔍 Verificación

```bash
# Ver outputs
aws cloudformation describe-stacks \
  --stack-name nbox-infra-dev \
  --query 'Stacks[0].Outputs'

# Listar tablas
aws dynamodb list-tables | grep nbox

# Ver configuraciones de prefijos (ahora kind=prefix_config en la tabla config)
go run ./cmd/cli prefix list --table nbox-config-production --json
```