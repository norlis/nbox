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

```
