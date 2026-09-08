# GitHub Action

Workload ABI can be used directly as a composite GitHub Action once the project is tagged.

```yaml
name: Runtime compatibility

on:
  pull_request:

jobs:
  workload-abi:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v4

      # Build or pull the two image references before this step.
      - name: Compare runtime ABI
        id: wabi
        uses: riccardomenegazzo/workload-abi@v0.1.0
        with:
          baseline: ghcr.io/acme/payment-api:1.8.3
          candidate: ghcr.io/acme/payment-api:1.8.4
          scenario: .wabi/payment-api.json
          target: compose.yaml
          service: payment-api
          fail-on: breaking

      - name: Show compatibility status
        if: always()
        run: echo "Workload ABI status: ${{ steps.wabi.outputs.status }}"
```

## Inputs

| Input | Required | Default | Description |
| --- | --- | --- | --- |
| `baseline` | yes | — | baseline image reference |
| `candidate` | yes | — | candidate image reference |
| `scenario` | no | empty | controlled scenario JSON |
| `target` | no | empty | Docker Compose target |
| `service` | no | empty | Compose service |
| `fail-on` | no | `breaking` | `breaking`, `degraded`, or `never` |
| `timeout` | no | `5m` | overall comparison timeout |

The action exposes `result` (path to JSON output) and `status` (`compatible`, `degraded`, `breaking`, or `not-evaluated`).

The runner must have a working Docker daemon. GitHub-hosted Ubuntu runners satisfy this requirement.
