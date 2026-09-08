# Compatibility model

Workload ABI models compatibility as a relationship, not a property of an image in isolation:

```text
Compatibility = f(Baseline behavior, Candidate behavior, Target constraints)
```

A newly observed behavior can be harmless in one environment and breaking in another.

## Current Docker Compose rules

The v0.1 solver consumes normalized output from:

```bash
docker compose -f compose.yaml config --format json
```

It currently proves conflicts for:

### Read-only root filesystem

If the candidate introduces a writable-layer mutation outside configured `tmpfs` mounts and the service has `read_only: true`, the result is breaking.

### Memory limit

If observed candidate peak memory exceeds the Compose `mem_limit`, the result is breaking.

### Stop grace period

If observed graceful shutdown duration exceeds `stop_grace_period`, the result is breaking because the runtime may force-kill the workload before shutdown completes.

### Disabled networking

If the candidate introduces outbound network behavior while the service uses `network_mode: none`, the result is breaking.

### Dropped capabilities

If the candidate observes a new effective Linux capability while the target explicitly drops all capabilities, the solver reports a breaking privilege conflict.

## Status semantics

- `compatible`: no modeled conflict was proven.
- `degraded`: significant runtime changes exist, but the target model does not prove a direct break.
- `breaking`: at least one high/critical target contradiction was proven.
- `unknown`: reserved for future solvers that cannot obtain enough evidence.

The project intentionally avoids claiming that `compatible` means universally safe; it means compatible with the constraints currently modeled and exercised.
