```shell

  # Single prefix upsert (merge: only provided flags change)
  go run cmd/cli/main.go prefix upsert \
    --prefix global/serverless \
    --type parameterstore \
    --table <cfg-table> \
    --region us-east-1

  # List all prefix configs as JSON
  go run cmd/cli/main.go prefix list --json --table <cfg-table>

  # Remove a prefix config (interactive prompt unless --force)
  go run cmd/cli/main.go prefix rm global/serverless --force --table <cfg-table>

  # Local dev — generate env vars without DynamoDB (--emit-env prints export lines to stdout)
  go run cmd/cli/main.go config user upsert --username admin --password 'secret' --roles admin --emit-env
  go run cmd/cli/main.go config approle generate --name watcher --roles entrypushd --emit-env
  go run cmd/cli/main.go config approle rotate-secret --emit-env
  go run cmd/cli/main.go config aws-sts upsert --arn arn:aws:iam::123:role/foo --roles entrypushd --emit-env

```
