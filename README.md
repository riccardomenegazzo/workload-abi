# Workload ABI

> **Your API didn't change. Your tests pass. Your image builds. Production can still break.**

Workload ABI (`wabi`) is an experimental, vendor-neutral tool for discovering **operational breaking changes** between container releases.

It runs two container images under equivalent conditions, records their observable runtime behavior, normalizes that behavior into a versioned workload snapshot, compares the two snapshots, and can then prove whether a change violates a real deployment target or an explicit compatibility policy.

## Why

Software already has contracts for source APIs, binary ABIs, package dependencies, SBOMs, signatures, and provenance. What is usually missing is a compatibility model for the **operational interface** between a workload and its environment.

A patch release can keep the same HTTP API and still:

- spawn a new helper process;
- listen on a new runtime port;
- mutate new filesystem paths;
- change its runtime user or entrypoint;
- require additional capabilities;
- fail one step of the same smoke workflow;
- exceed the target memory or shutdown budget;
- stop satisfying a read-only-rootfs or `runAsNonRoot` constraint.

Those are operational changes. `wabi` makes them observable, comparable, policy-enforceable, and target-aware.

## What works now

### Runtime evidence

- Docker-backed experiment orchestration;
- immutable image provenance/configuration separated from temporary container state;
- process snapshots via `docker top`;
- filesystem mutations via `docker diff`;
- CPU/memory/network/block-I/O snapshots via `docker stats`;
- runtime TCP listener discovery from container procfs when available;
- Docker network mode and attached-network capture;
- startup/shutdown lifecycle timing;
- schema-versioned snapshots;
- stable SHA-256 evidence fingerprints that exclude volatile timing/stat measurements.

### Equivalent experiments

Scenario files can apply the same:

- environment variables;
- command override;
- ordered multi-step `docker exec` workflow;
- per-step delay;
- per-step timeout;
- allowed-failure semantics.

Each step records its command, duration, exit code, success state, bounded output, and error. A baseline-success → candidate-failure regression becomes `BREAKING`.

### Target solving

Docker Compose targets are rendered with `docker compose config --format json` and checked for:

- read-only root filesystems and writable mounts;
- memory limits;
- shutdown grace periods;
- dropped capabilities.

Kubernetes targets support:

- `Pod`;
- `Deployment`;
- `StatefulSet`;
- `DaemonSet`;
- `ReplicaSet`;
- `Job`;
- `CronJob`.

Kubernetes JSON manifests are parsed natively. YAML manifests are normalized through `kubectl --dry-run=client` when `kubectl` is available.

Current Kubernetes checks cover:

- `readOnlyRootFilesystem`;
- writable volume mounts;
- memory limits;
- `terminationGracePeriodSeconds`;
- `runAsNonRoot` / `runAsUser`;
- dropped capabilities;
- privilege-escalation constraints.

### CI policy and output

- `COMPATIBLE`, `CHANGED`, and `BREAKING` verdicts;
- human-readable output;
- JSON output;
- SARIF 2.1.0 output;
- explicit JSON policy files;
- CI-safe exit codes;
- strict `gofmt`, `go vet`, race tests, build gate, and real Docker end-to-end tests;
- cross-platform release workflow for Linux, macOS, and Windows on amd64/arm64.

## Install

Requirements:

- Go 1.23+
- Docker Engine / Docker Desktop
- Docker Compose v2 for Compose target analysis
- `kubectl` only when a Kubernetes target is YAML rather than JSON

```bash
git clone https://github.com/riccardomenegazzo/workload-abi.git
cd workload-abi
make build
```

The CLI is built at `./bin/wabi`.

## Compare two releases

```bash
./bin/wabi compare app:v1 app:v2
```

Increase the minimum experiment window:

```bash
./bin/wabi compare --observe 5s app:v1 app:v2
```

JSON:

```bash
./bin/wabi compare --format json app:v1 app:v2
```

SARIF:

```bash
./bin/wabi compare --format sarif app:v1 app:v2 > wabi.sarif
```

Fail CI whenever any operational change is found:

```bash
./bin/wabi compare --fail-on-change app:v1 app:v2
```

## Multi-step scenarios

Example:

```json
{
  "name": "checkout-smoke",
  "environment": {
    "APP_MODE": "production-like"
  },
  "steps": [
    {
      "name": "verify-config",
      "after": "100ms",
      "exec": ["sh", "-c", "test \"$APP_MODE\" = production-like"],
      "timeout": "2s"
    },
    {
      "name": "health-probe",
      "after": "250ms",
      "exec": ["sh", "-c", "wget -qO- http://127.0.0.1:8080/health"],
      "timeout": "3s"
    }
  ]
}
```

Run the same experiment against both releases:

```bash
./bin/wabi compare \
  --scenario scenario.json \
  app:v1 app:v2
```

If a required step succeeds in the baseline and fails in the candidate, the comparison becomes `BREAKING`.

## Docker Compose target

```bash
./bin/wabi compare \
  --scenario scenario.json \
  --target compose.yaml \
  --service api \
  app:v1 app:v2
```

Auto-detection recognizes Compose files containing `services:`.

## Kubernetes target

JSON is parsed without any external dependency:

```bash
./bin/wabi compare \
  --scenario scenario.json \
  --target deployment.json \
  --target-kind kubernetes \
  --container api \
  app:v1 app:v2
```

For YAML:

```bash
./bin/wabi compare \
  --scenario scenario.json \
  --target deployment.yaml \
  --workload payments \
  --container api \
  app:v1 app:v2
```

When `--target-kind auto` is used, Workload ABI distinguishes Compose and Kubernetes targets from their structure.

## Policy gate

Example policy:

```json
{
  "name": "production",
  "require_scenario": true,
  "require_target": true,
  "deny_severities": ["warning"],
  "deny_surfaces": ["privilege"],
  "deny_kinds": ["target-conflict"],
  "max_changes": 10
}
```

Apply it:

```bash
./bin/wabi compare \
  --scenario scenario.json \
  --target compose.yaml \
  --policy policy.json \
  app:v1 app:v2
```

Policy violations are represented as explicit `policy-violation` evidence and promote the verdict to `BREAKING`.

## Record one workload snapshot

```bash
./bin/wabi record \
  --observe 3s \
  --scenario scenario.json \
  app:v2 > snapshot.json
```

A snapshot includes:

- schema version;
- image ID;
- scenario provenance;
- stable evidence fingerprint;
- process/file/listener/network evidence;
- scenario-step outcomes;
- image/runtime configuration;
- lifecycle measurements;
- point-in-time runtime stats;
- collection warnings.

## Fingerprints

Workload ABI computes a SHA-256 fingerprint over the normalized operational contract. Volatile values such as capture time, exact CPU percentage, and lifecycle durations are deliberately excluded.

This gives future OCI attestations a stable payload anchor without pretending that noisy point-in-time telemetry is deterministic.

## Exit codes

- `0`: no fatal error;
- `3`: changes found with `--fail-on-change`;
- `4`: breaking runtime, scenario, target, or policy regression;
- `1`: execution/runtime error;
- `2`: CLI usage error.

## Example result

```text
WORKLOAD ABI
================================================================
payment-api:1.8.3 -> payment-api:1.8.4
baseline fingerprint:  sha256:...
candidate fingerprint: sha256:...
scenario: checkout-smoke
target:   kubernetes:Deployment/payments#api
policy:   production

SCENARIO
  ~ [BREAKING] required scenario step regressed: health-probe
      before: health-probe exit=0
      after:  health-probe exit=7

FILESYSTEM
  ~ [BREAKING] candidate introduces a new filesystem mutation outside
      writable Kubernetes volume mounts while readOnlyRootFilesystem is enabled

----------------------------------------------------------------
RUNTIME COMPATIBILITY: BREAKING
```

## Design principles

1. **Observe, do not guess.** Runtime evidence is first-class.
2. **Same experiment, two releases.** Comparison is meaningful only under equivalent inputs.
3. **Semantic differences, not log diffs.** Raw evidence is normalized before comparison.
4. **Environment-aware compatibility.** A difference and a breaking change are not the same thing.
5. **Policy is evidence, not a hidden switch.** Policy failures are explicit result entries.
6. **Preserve provenance.** Image identity, fingerprints, scenario, target, and policy belong to the result.
7. **Progressive depth.** Portable Docker evidence comes first; eBPF can deepen the recorder without replacing the compatibility model.
8. **Machine-consumable by design.** JSON and SARIF are first-class outputs.

## Architecture

```text
                         scenario
                            |
             +--------------+--------------+
             |                             |
         image:v1                       image:v2
             |                             |
             v                             v
      runtime snapshot A            runtime snapshot B
             |                             |
             +--------------+--------------+
                            |
                      semantic diff
                            |
                   +--------+--------+
                   |                 |
             target solver       policy gate
          Compose / Kubernetes       |
                   +--------+--------+
                            |
                            v
                 compatibility verdict
                   /        |        \
              human        JSON      SARIF
```

See [`docs/architecture.md`](docs/architecture.md) for the internal model.

## Reproduce the included end-to-end proof

```bash
docker build -t wabi-demo:v1 ./examples/demo/v1
docker build -t wabi-demo:v2 ./examples/demo/v2

./bin/wabi compare \
  --observe 500ms \
  --scenario ./examples/demo/scenario.json \
  --target ./examples/demo/compose.yaml \
  --service app \
  --policy ./examples/demo/policy.json \
  wabi-demo:v1 wabi-demo:v2
```

The candidate writes under `/var/lib/demo`, while the target has a read-only root filesystem and only `/tmp` is writable. Workload ABI proves the target-specific incompatibility and exits with code `4`.

## Release artifacts

Pushing a `v*` tag builds and publishes:

- Linux amd64/arm64;
- macOS amd64/arm64;
- Windows amd64/arm64;
- SHA-256 checksums.

The version is embedded into the binary:

```bash
wabi version
```

## Roadmap

The remaining depth is intentionally concentrated in evidence collection rather than CLI surface area:

- eBPF process/file/network/syscall recorder;
- DNS and outbound dependency graph;
- syscall/capability requirement inference;
- seccomp/AppArmor/NetworkPolicy solving;
- Helm-rendered Kubernetes targets;
- causal runtime graph;
- OCI-linked Workload ABI attestations;
- Sigstore signing and verification.

## Status

Workload ABI is an early-stage research/engineering project. The current implementation is intentionally conservative: it reports evidence it can explain and calls a target or policy conflict breaking only when it has a concrete reason.

## License

Apache License 2.0.
