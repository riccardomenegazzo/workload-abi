# Seccomp compatibility proof

Workload ABI can prove whether a **new exact syscall requirement** observed in a candidate release is incompatible with a Docker/OCI-style seccomp profile.

This is an environment proof, not a seccomp policy generator.

## Why this belongs in Workload ABI

A release can keep the same API and image contract while introducing a new kernel dependency. For example, a candidate may begin using `bpf`, `io_uring_setup`, `userfaultfd`, or another syscall that the production seccomp profile rejects.

A raw runtime diff can tell us that the syscall appeared. Workload ABI adds the target question:

> **Would the deployment seccomp profile permit the new syscall requirement?**

## CLI

```bash
wabi compare-snapshots \
  --seccomp-profile ./seccomp.json \
  --format json \
  baseline.json candidate.json
```

The seccomp profile is an independent target constraint. It can be used with or without a Compose/Kubernetes target. When another target is present, the comparison target identity is extended with the seccomp artifact.

Example target identity:

```text
kubernetes:Deployment/api#api+seccomp:production.json
```

## Evidence boundary

The current writer schema is `wabi.dev/v1alpha3`.

That schema intentionally normalizes common kernel activity into provider-neutral behavior:

```text
openat   -> file/open
connect  -> network/connect
execve   -> process/exec
```

Unknown or otherwise unclassified syscall evidence is preserved as:

```json
{
  "category": "syscall",
  "operation": "bpf"
}
```

The seccomp proof uses **only** this exact syscall identity. It does not reverse-map `file/open` back to `openat`, because multiple kernel syscalls can normalize to the same Operational ABI behavior.

This means the initial proof is deliberately narrower than full syscall coverage, but every `BREAKING` verdict is evidence-backed.

## Release-regression semantics

Only syscall requirements that are new in the candidate are evaluated as release regressions.

If both baseline and candidate already contain:

```text
syscall/bpf
```

then a supplied profile that blocks `bpf` does not retroactively turn the release comparison into a new breaking regression. The environment may still be invalid, but that is not a behavior introduced by this release.

## Profile decisions

Workload ABI parses Docker/OCI-style profile fields:

```json
{
  "defaultAction": "SCMP_ACT_ERRNO",
  "syscalls": [
    {
      "names": ["read", "write"],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}
```

Actions are classified conservatively:

| Seccomp action | Workload ABI decision |
|---|---|
| `SCMP_ACT_ALLOW` | allow |
| `SCMP_ACT_LOG` | allow |
| `SCMP_ACT_ERRNO` | deny |
| `SCMP_ACT_KILL` | deny |
| `SCMP_ACT_KILL_PROCESS` | deny |
| `SCMP_ACT_KILL_THREAD` | deny |
| `SCMP_ACT_TRAP` | deny |
| `SCMP_ACT_TRACE` | unknown |
| `SCMP_ACT_NOTIFY` | unknown |
| unrecognized action | unknown |

`SCMP_ACT_LOG` allows the syscall after logging it. `TRACE` and `NOTIFY` can depend on an external tracer or userspace decision, so Workload ABI does not claim deterministic denial.

## Conditional rules

A syscall rule that contains argument predicates or Docker profile `includes` / `excludes` conditions is treated as **unknown** by this first proof layer.

Example:

```json
{
  "names": ["clone"],
  "action": "SCMP_ACT_ALLOW",
  "args": [
    {
      "index": 0,
      "value": 268435456,
      "op": "SCMP_CMP_MASKED_EQ"
    }
  ]
}
```

The current `RuntimeEvent` contract does not carry syscall argument values, so evaluating that rule would require guessing. Unknown does not create a `BREAKING` target conflict.

Conflicting unconditional rules for the same syscall also remain unknown rather than choosing an arbitrary winner.

## Breaking proof

Given candidate evidence:

```json
{
  "category": "syscall",
  "operation": "bpf",
  "process": "/usr/bin/app"
}
```

and a profile containing:

```json
{
  "defaultAction": "SCMP_ACT_ALLOW",
  "syscalls": [
    {
      "names": ["bpf"],
      "action": "SCMP_ACT_ERRNO"
    }
  ]
}
```

Workload ABI produces a target conflict on the `syscall` surface and a `BREAKING` verdict.

The repository CI reproduces this path with real persisted snapshots and public generic `RuntimeEvent` enrichment.

## What this does not claim yet

The current slice does not:

- infer raw syscall names from normalized file/network/process events;
- evaluate syscall argument predicates;
- resolve architecture-specific profile branches;
- evaluate capability/min-kernel `includes` or `excludes` conditions;
- generate a least-privilege seccomp profile;
- claim that absence of a syscall from one observation means the workload never needs it.

A future schema can add raw syscall identity and argument evidence without changing the meaning of already-published v1alpha3 artifacts.
