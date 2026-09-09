# Contributing to Workload ABI

Workload ABI is an experimental open-source specification and implementation. Contributions that improve correctness, portability, evidence quality, compatibility semantics, or reproducibility are welcome.

## Development requirements

- Go 1.23+
- Docker Engine or Docker Desktop
- Docker Compose v2 for Compose target tests
- `kubectl` only when testing Kubernetes YAML target normalization

## Local checks

Run the same core gates used by CI:

```bash
gofmt -w ./cmd ./internal
go vet ./...
go test -race ./...
go build -trimpath -o ./bin/wabi ./cmd/wabi
```

For Docker integration work, reproduce the included demo:

```bash
docker build -t wabi-demo:v1 ./examples/demo/v1
docker build -t wabi-demo:v2 ./examples/demo/v2

./bin/wabi compare \
  --observe 500ms \
  --scenario ./examples/demo/scenario.json \
  --target ./examples/demo/compose.yaml \
  --service app \
  wabi-demo:v1 wabi-demo:v2
```

## Design rules

### Evidence must be explainable

Do not introduce a compatibility verdict that cannot point back to concrete evidence or an explicit policy/target constraint.

### Keep collection separate from semantics

Deep recorders such as eBPF, Falco, Tracee, or platform-specific providers should normalize into the public snapshot model instead of embedding product-specific behavior into the diff engine.

### Avoid incidental nondeterminism

A field should participate in the operational fingerprint only when it represents a stable compatibility-relevant property. Timestamps, diagnostic text, and noisy samples should not accidentally change the identity of the operational ABI.

### Preserve equivalent experiments

Changes to scenario execution must maintain the rule that baseline and candidate receive equivalent stimuli.

### Public schema changes are API changes

When changing serialized snapshot, comparison, scenario, policy, or attestation structures, update the appropriate schema under `schemas/`, tests, and documentation. Do not silently change the meaning of a published schema version.

## Pull requests

A strong PR should include:

- a concise problem statement;
- tests that fail without the change;
- documentation when user-visible behavior changes;
- no unrelated refactors;
- evidence that `go vet`, race tests, and build pass.

For new compatibility rules, include at least one positive and one negative case whenever practical.

## Adding a new target adapter

Target adapters should:

1. parse or normalize the target configuration;
2. expose only constraints relevant to operational compatibility;
3. produce explainable conflicts;
4. avoid performing mutations against the target environment.

## Adding a new recorder

A recorder should be treated as an evidence provider. It should not decide whether evidence is breaking. That belongs to diff/compatibility/policy layers.

## License

By contributing, you agree that your contribution is licensed under Apache-2.0, consistent with the repository license.
