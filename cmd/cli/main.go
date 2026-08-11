package main

import (
	"nbox/cmd/cli/commands"
)

// main go run cmd/cli/main.go --region us-east-1 seed prefix-example.json.
func main() {
	commands.Execute()
}
