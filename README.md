# cert-manager-webhook-mindns

A [cert-manager](https://cert-manager.io/) webhook solver for [mindns](https://github.com/greatliontech/mindns).

## Installation

### Helm

```bash
helm install mindns-webhook oci://ghcr.io/greatliontech/mindns-webhook-helm-chart \
  --namespace cert-manager
```

### Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Image repository | `ghcr.io/greatliontech/cert-manager-webhook-mindns` |
| `image.tag` | Image tag | `latest` |
| `mindns.tokenSecretRef.name` | Secret name for default MINDNS_TOKEN env var | `""` |
| `mindns.tokenSecretRef.key` | Key in secret containing the token | `token` |

## Usage

Create a `ClusterIssuer` or `Issuer` that references the webhook:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: you@example.com
    privateKeySecretRef:
      name: letsencrypt-account-key
    solvers:
      - dns01:
          webhook:
            groupName: acme.greatlion.tech
            solverName: mindns
            config:
              serverAddr: "mindns.default.svc:50051"
              # zone: "example.com."  # optional, derived from challenge if omitted
              tokenSecretRef:         # optional, for authenticated mindns
                name: mindns-token
                key: token            # defaults to "token" if omitted
```

Create the token secret in the same namespace as your Issuer (or any namespace for ClusterIssuer):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mindns-token
  namespace: cert-manager
type: Opaque
stringData:
  token: "your-mindns-token"
```

## Development

Requires [Task](https://taskfile.dev/).

```bash
task build      # Build binary
task test       # Run tests
task lint       # Run linter
task docker-build TAG=v1.0.0  # Build Docker image
task helm-lint  # Lint Helm chart
```

## License

Apache 2.0
