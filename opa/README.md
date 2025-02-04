
```shell

# rub dev
 ./opa run --server --log-level debug --config-file=config.yaml ./data --disable-telemetry --log-format=json-pretty --set=decision_logs.console=true
 ./opa run  ./data --server --log-level debug --disable-telemetry --set=decision_logs.console=true


# Build a new bundle for the new policy. 
./opa build data/policies.rego 


# evaluar
./opa eval -i test.json -d authz "authz.allow"

# test
./opa test . -v

# format
./opa fmt --write ./authz
```

https://play.openpolicyagent.org/p/CJIq9dnzfC
https://docs.styra.com/opa/rego-cheat-sheet
https://docs.styra.com/opa/rego-style-guide#use-opa-fmt


```shell
opa eval --data policies/  --data data.json --input input.json "data.policies.rbac.allow" --explain=full

opa eval --data policies/policy.rego --data roles.json --data users.json --input input.json "data.policies.rbac.allow"



opa fmt --write policies
```