# Security policy

Workload ABI executes user-selected container images and can optionally observe host runtime behavior with a native Linux eBPF provider. Treat both workload execution and privileged observation as security-sensitive operations.

## Supported versions

Until the first stable release, security fixes are applied to the latest development line and latest published release.

## Reporting a vulnerability

Please avoid filing a public issue for a vulnerability that could enable arbitrary host compromise, credential exposure, container escape, unsafe command execution, privileged eBPF misuse, or trust bypass in snapshot/attestation verification.

Use GitHub's private vulnerability reporting for this repository when available. Include:

- affected commit or release;
- reproduction steps;
- expected and actual behavior;
- impact assessment;
- any suggested mitigation.

## Threat model

### Untrusted workload execution

`wabi record` and `wabi compare` execute container images through the configured Docker daemon. The project does not claim to sandbox a malicious image beyond the isolation provided by that container runtime and the options used for the experiment.

Do not run untrusted images against a privileged or production Docker daemon.

### Docker socket access

Running the `wabi` container with `/var/run/docker.sock` mounted grants it the ability to control the Docker daemon. On a typical host that is effectively privileged access. Use a disposable development/CI environment when possible.

### Native eBPF provider

`wabi-native` loads and attaches eBPF programs to Linux syscall tracepoints. On many systems this requires root or a similarly privileged capability set.

Treat native collection as privileged host instrumentation, not as an ordinary user-space parser.

Security properties and boundaries:

- the provider reads tracefs metadata and attaches eBPF tracepoint programs;
- it creates a perf-event map and receives captured samples;
- it does not intentionally change workload files, networking, credentials, kernel policy, or process state;
- it does not mount tracefs automatically;
- it does not require the core `wabi` CLI to run with eBPF privileges;
- native evidence is emitted as the same `RuntimeEvent` data contract used by external providers.

Do not grant privileged eBPF access to an untrusted binary or run `wabi-native` on a sensitive production host unless the host-instrumentation implications are understood and accepted.

Kernel/distribution policies differ. The project does not assume that capability-only deployments are equivalent to root across systems.

### Tracefs

The native provider expects tracefs at `/sys/kernel/tracing` or `/sys/kernel/debug/tracing`.

If tracefs must be mounted, that is an explicit administrator/environment action. `wabi-native` intentionally does not mount host filesystems itself.

### Scenario commands

Scenario `exec` steps run inside the experiment container. They are argument arrays, not shell strings, but they can still execute arbitrary programs present in the image.

### Persisted snapshots and runtime events

Snapshots and runtime-event artifacts are evidence, not secrets containers. Scenario output, process command lines, filesystem paths, or network destinations may contain sensitive data. Review artifacts before sharing them publicly.

The operational fingerprint detects accidental or deliberate modification of persisted evidence, but it is **not a cryptographic signature** and does not establish who produced the snapshot.

Native recorder diagnostic data such as PIDs and probe errors is not part of RuntimeEvent semantic identity, but it can still expose host details.

### Attestations

The built-in in-toto attestation is an unsigned statement whose verifier checks structural and digest consistency. Authenticity requires external signing and verification (for example with Sigstore or another organizational trust system).

### Target parsing

Target adapters are read-only. Kubernetes YAML normalization uses client-side `kubectl create --dry-run=client`; Workload ABI should not mutate clusters as part of compatibility analysis.

## Security design principles

- fail closed when persisted evidence fails fingerprint verification;
- keep signing/authenticity separate from evidence integrity;
- keep privileged native collection separate from the portable compatibility engine;
- do not mount host filesystems implicitly;
- avoid shell interpolation for scenario commands;
- cap captured command output;
- use bounded timeouts and event limits for runtime collection;
- account for dropped native samples explicitly;
- keep target analysis read-only;
- prefer explainable deterministic rules over opaque classification.
