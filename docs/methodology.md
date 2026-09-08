# Methodology and limitations

Workload ABI is a differential experiment, not static proof.

## Controlled execution

The same scenario is applied to baseline and candidate images. A scenario fixes environment variables, command overrides, published ports, HTTP probes, observation duration, sample interval, and shutdown timeout. The goal is to reduce input variance before comparing behavior.

## Evidence in v0.1

The Docker recorder currently uses:

- `docker top` for process samples;
- `/proc/net/tcp*` and `/proc/net/udp*` inside the container for socket observations;
- `/proc/1/status` for runtime UID/GID, capabilities, seccomp, and no-new-privileges state;
- `docker diff` for writable-layer filesystem mutations;
- `docker stats --no-stream` for sampled CPU and memory;
- `docker inspect` for container/image identity and final lifecycle state;
- elapsed time around readiness probes and graceful `docker stop` for lifecycle timing.

This deliberately avoids a privileged sensor in v0.1.

## What a result means

A reported change means the behavior was observed in the candidate and not in the baseline during this specific experiment. A reported compatibility conflict means the project can explain a direct contradiction between observed candidate behavior and a modeled target constraint.

A clean result does **not** prove that two releases are universally compatible. Unexercised code paths can contain behavior the scenario never triggered.

## Sources of uncertainty

- sampling can miss very short-lived processes or sockets;
- IP addresses are unstable identities and DNS attribution is not yet retained;
- `docker diff` reports layer mutations, not every read access;
- resource peaks are samples, not continuous maxima;
- application warm-up, caches, randomness, external services, and scheduler timing can create nondeterminism;
- Docker Desktop introduces an additional VM boundary, although the current observer operates through Docker APIs/exec rather than host PID assumptions.

## Planned confidence model

A future release should execute each version multiple times and classify changes as stable, intermittent, or noisy. Compatibility gates should eventually include a confidence score rather than treating one observation as absolute truth.
