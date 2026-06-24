# NBOX Infrastructure Deployment

Template SAM/CloudFormation para desplegar la infraestructura de NBOX (DynamoDB + S3).

## 📦 Recursos

- **EntriesTable** - Tabla principal de entries (Path+Key)
- **TrackingEntriesTable** - Tabla de auditoría (Key+Timestamp)
- **BoxTable** - Metadata de boxes (Service+Stage)
- **PrefixConfigTable** - Configuración de prefijos v2 (Prefix)
- **BoxBucket** - Bucket S3 para templates

## 🚀 Deploy

```bash
# Development
sam validate --lint && sam build && sam deploy --config-env dev

# Production
sam validate --lint && sam build && sam deploy --config-env prod
```

## 🌱 Seed Prefix Configurations

Después del deploy, poblar PrefixConfigTable:

```bash
# Poblar configuraciones por defecto
./seed-prefix-config.sh

# Ver qué se insertaría (dry-run)
./seed-prefix-config.sh --dry-run

# Limpiar y re-seed
./seed-prefix-config.sh --clean

# Agregar configuración personalizada
./add-prefix-config.sh passbox parameterstore_secure --tag isVault=true
```

Ver [SEED_USAGE.md](./SEED_USAGE.md) para más detalles.

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

# Ver configuraciones de prefijos
aws dynamodb scan --table-name nbox-prefix-config-table \
  | jq '.Items[] | {Prefix: .Prefix.S, TypeDefault: .TypeDefault.S}'
```