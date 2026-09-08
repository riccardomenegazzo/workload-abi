# Contributing

Workload ABI is intentionally experimental. Contributions that challenge the model are as valuable as features.

## Development

Requirements: Go 1.23+, Docker for integration work, and Docker Compose v2 for target compatibility tests.

```bash
make check
make build
```

Run the end-to-end demo:

```bash
make demo
```

## Pull requests

Please keep changes focused and include tests for semantic or compatibility behavior. If a change adds a new observation surface, document:

1. what evidence is collected;
2. how it is normalized;
3. known sources of nondeterminism;
4. why a difference is operationally meaningful.

Recorder-specific data should not leak into the generic RuntimeGraph unless it represents a portable operational concept.

## Compatibility rules

New solver rules must be evidence-backed and explainable. A rule should point to both an observed candidate fact and a concrete target constraint; avoid heuristic `breaking` labels that cannot be justified to a user.
