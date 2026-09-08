# Operational ABI specification — v1alpha1

An Operational ABI is the observed interface between a workload and the environment required to execute it successfully.

Workload ABI currently models six surfaces.

## Process ABI

Observed executable/process identities and commands. A new helper binary, shell, or daemon is a semantic addition.

## Network ABI

Listening sockets and active connections. Listening and inbound flows are separated from outbound flows so test probes do not masquerade as new egress behavior.

## Filesystem ABI

Writable-layer mutations classified using Docker's `A`, `C`, and `D` mutation semantics.

## Privilege ABI

Image/runtime user, effective UID/GID, Linux effective capability set, seccomp mode, no-new-privileges state, and Docker privilege settings.

## Resource ABI

Sampled peak memory and CPU. This is currently observational rather than a proven minimum resource requirement.

## Lifecycle ABI

Time to successful probe, graceful shutdown duration, exit code, and OOM state.

## Versioning

All machine-readable artifacts carry `schema_version: wabi.dev/v1alpha1`. Backwards-incompatible schema evolution before v1.0 will use a new API version. The CLI should continue to read older versions when practical.
