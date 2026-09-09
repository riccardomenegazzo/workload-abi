# Security policy

Workload ABI executes user-selected container images and optional scenario commands. Treat those workloads as untrusted code.

## Supported versions

Until the first stable release, security fixes are applied to the latest development line and latest published release.

## Reporting a vulnerability

Please avoid filing a public issue for a vulnerability that could enable arbitrary host compromise, credential exposure, container escape, unsafe command execution, or trust bypass in snapshot/attestation verification.

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

### Scenario commands

Scenario `exec` steps run inside the experiment container. They are argument arrays, not shell strings, but they can still execute arbitrary programs present in the image.

### Persisted snapshots

Snapshots are evidence, not secrets containers. Scenario output or process command lines may contain sensitive data if the workload prints or exposes it. Review artifacts before sharing them publicly.

The operational fingerprint detects accidental or deliberate modification of persisted evidence, but it is **not a cryptographic signature** and does not establish who produced the snapshot.

### Attestations

The built-in in-toto attestation is an unsigned statement whose verifier checks structural and digest consistency. Authenticity requires external signing and verification (for example with Sigstore or another organizational trust system).

### Target parsing

Target adapters are read-only. Kubernetes YAML normalization uses client-side `kubectl create --dry-run=client`; Workload ABI should not mutate clusters as part of compatibility analysis.

## Security design principles

- fail closed when persisted evidence fails fingerprint verification;
- keep signing/authenticity separate from evidence integrity;
- avoid shell interpolation for scenario commands;
- cap captured command output;
- use bounded timeouts for scenario execution;
- keep target analysis read-only;
- prefer explainable deterministic rules over opaque classification.
