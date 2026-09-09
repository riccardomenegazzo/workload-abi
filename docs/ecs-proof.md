# Amazon ECS task-definition compatibility proof

Workload ABI can evaluate persisted runtime evidence against an Amazon ECS task definition without calling AWS APIs or requiring the AWS CLI.

The ECS task definition is treated as an **environment contract**. Workload ABI asks whether behavior newly introduced by a candidate release conflicts with explicit runtime constraints in that contract.

## CLI

```bash
wabi compare-snapshots \
  --target task-definition.json \
  --target-kind ecs \
  --container api \
  baseline.json candidate.json
```

ECS JSON is auto-detected when `--target-kind auto` is used, so the explicit kind is optional for standard task definitions containing `containerDefinitions`.

## Container selection

A task definition with one container requires no selector.

A task definition with multiple containers must be disambiguated:

```bash
--container api
```

The target identity is:

```text
ecs:<family>#<container>
```

If `family` is absent, Workload ABI falls back to the task-definition filename.

## Proven constraints

### Read-only root filesystem

When a selected container declares:

```json
{
  "readonlyRootFilesystem": true
}
```

new candidate filesystem mutations are incompatible unless they occur beneath a writable ECS `mountPoint`.

Example:

```json
{
  "mountPoints": [
    {
      "containerPath": "/tmp",
      "readOnly": false
    }
  ]
}
```

A new write under `/tmp` is permitted by that target. A new write under `/var/lib/app` is a provable `filesystem / target-conflict`.

Workload ABI evaluates release regressions: a path already mutated by the baseline is not reclassified as a new candidate incompatibility.

### Hard memory limits

ECS container `memory` is treated as an explicit hard limit.

Task-level `memory` is also an upper bound on the task. When both are explicitly present, Workload ABI uses the lower positive value as the maximum the selected container can safely consume.

Values are expressed by ECS in MiB and normalized to bytes before comparison with observed memory usage.

### Soft memory reservation

`memoryReservation` is retained by the target model but is **not** used to produce a hard compatibility failure.

A candidate exceeding a soft reservation is not the same as a candidate exceeding the ECS hard `memory` limit.

### Explicit stop timeout

When `stopTimeout` is explicitly present, Workload ABI compares it to the observed shutdown duration.

```json
{
  "stopTimeout": 10
}
```

If the candidate takes longer than 10 seconds to stop, the target conflict is provable.

When `stopTimeout` is absent, Workload ABI does not invent a default value. ECS behavior can depend on launch type and container-agent configuration, so absence remains unknown rather than becoming a false `BREAKING` verdict.

### Capabilities

The ECS target parser reads:

```text
linuxParameters.capabilities.add
linuxParameters.capabilities.drop
```

The current compatibility model reuses the existing conservative rule applied to Compose/Kubernetes: if ECS drops `ALL` capabilities while the candidate container configuration explicitly adds capabilities, that configuration is incompatible.

## Constraints intentionally not inferred

The first ECS proof does not claim that:

- exceeding `memoryReservation` is a hard failure;
- an absent `stopTimeout` means a fixed timeout;
- every observed listener must have an ECS `portMapping`;
- a target `user` setting is incompatible without runtime evidence proving a user requirement;
- ECS network mode alone proves destination reachability;
- absence of an observed behavior proves the workload can never perform it.

The v0.7 rule remains:

> **Unknown is not breaking.**

## End-to-end repository proof

The ECS workflow uses two real container releases:

```text
v1 -> writes /tmp/version
v2 -> introduces /var/lib/demo/version
```

The target task definition has:

```text
readonlyRootFilesystem = true
writable mount          = /tmp
```

The candidate therefore introduces a write that the ECS environment cannot support. CI requires exit code `4`, a `BREAKING` verdict, target identity `ecs:wabi-ecs-proof#api`, and a `filesystem / target-conflict` referring to `/var/lib/demo`.

This demonstrates that the same persisted Operational ABI evidence can be solved against Docker Compose, Kubernetes, and Amazon ECS without changing the core comparison model.
