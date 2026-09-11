# kube-origin

See which kubeconfig files supply the context, cluster, API server, namespace, and user your Kubernetes client will select. `kube-origin` works locally. It does not contact a cluster or run credential plugins.

```text
$ kube-origin
Context:        dev (selected by current-context in ~/.kube/config)
Context entry:  ~/.kube/config
Cluster:        dev-cluster (from ~/.kube/config)
API server:     https://api.dev.example (from ~/.kube/config)
Namespace:      default (Kubernetes default)
User:           dev-user (from ~/.kube/config)
Authentication: exec plugin: cloud-login (not executed)

Inputs, in precedence order:
  1. ~/.kube/config
```

## Install

With Go 1.26 or newer:

```sh
go install github.com/jad-fahmi/kube-origin/cmd/kube-origin@latest
```

Or build from a checkout:

```sh
go build -o kube-origin ./cmd/kube-origin
```

## Usage

```sh
kube-origin                         # explain current-context
kube-origin --context staging      # explain a named context
kube-origin --kubeconfig ./team.yaml
kube-origin --context prod --json  # machine-readable output
```

To see a two-file merge with the included examples, set `KUBECONFIG` to
`examples/kubeconfig-first.yaml:examples/kubeconfig-second.yaml` on Linux and
macOS (use `;` between paths on Windows), then run `kube-origin`. The later
file's same-named entries appear under **Shadowed entries**.

By default, client-go reads `KUBECONFIG` using its standard file-list and merge rules, or `~/.kube/config` when `KUBECONFIG` is unset. `--kubeconfig` selects one file. Context, cluster, and user entries from earlier files take precedence over same-named entries in later files. The report lists inputs in precedence order and identifies duplicate entries that were ignored.

## Safety and limits

- The tool is read-only and makes no network requests.
- It uses the official client-go loader to determine effective configuration.
- Credential values, client keys, and certificate data are never included in the report.
- An `exec` authenticator is identified by its command's basename. The executable is never launched and its arguments are not printed.
- API server URLs have user information, query strings, and fragments removed before display.
- Source attribution is at the kubeconfig entry level. It does not claim to identify a separate file for every field inside an entry.
- `default` namespace is labeled as Kubernetes' implicit default when no namespace is configured.
- A local configuration report does not prove cluster reachability or successful authentication.

## Development

```sh
go build ./cmd/kube-origin
go vet ./...
```

The project uses Go modules and the official `k8s.io/client-go` loading implementation.

## License

MIT. See [LICENSE](LICENSE).
