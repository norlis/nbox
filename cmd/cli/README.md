```shell



  # Desde archivo
  go run cmd/cli/main.go seed prefix-example.json

  # JSON inline - Objeto único
  go run cmd/cli/main.go seed '{"prefix":"test","typeDefault":"dynamodb","typeAllowed":[]}'

  # JSON inline - Array
  go run cmd/cli/main.go seed '[{"prefix":"dev","typeDefault":"dynamodb","typeAllowed":[]}]'

  # Desde pipe
  cat prefix-example.json | go run cmd/cli/main.go seed -
  echo '{"prefix":"test","typeDefault":"dynamodb","typeAllowed":[]}' | go run cmd/cli/main.go seed -

```