##  Changes Made

  1. Configuration Parameters (cmd/main.go:74-75)

  Added two new configurable command-line flags:
  - --kube-dns-namespace (default: kube-system) - Namespace where kube-dns TLS secrets are stored
  - --kube-dns-secret-name (default: kube-dns-tls) - Name of the TLS secret

  2. API Server Updates (internal/controller/api_server.go)

  New Response Structure:
  - PolicyResponse - Wraps the policy and TLS data together
  - TLSSecretData - Contains the certificate and private key as byte arrays

  Key Changes:
  - APIServer now holds a Kubernetes client and configuration (api_server.go:33-39)
  - Added fetchTLSSecret() method (api_server.go:105-124) to retrieve TLS secrets from Kubernetes
  - Updated handleGetPolicy() (api_server.go:126-171) to:
    - Fetch TLS secret data on each policy request
    - Return enhanced response with both policy and TLS data
    - Gracefully handle TLS fetch errors (logs error but still returns policy)

  3. API Response Format

  The /api/policies?hash=<selectorHash> endpoint now returns:


  {
    "policy": {
      "metadata": { ... },
      "spec": { ... },
      "status": { ... }
    },
    "tlsData": {
      "certificate": "<base64-encoded-cert>",
      "privateKey": "<base64-encoded-key>"
    }
  }

  The tlsData field will be omitted if the secret cannot be fetched, ensuring backward compatibility.

  Usage

  Start the controller with custom kube-dns configuration:

  ./dns-mesh-controller \
    --kube-dns-namespace=custom-namespace \
    --kube-dns-secret-name=my-tls-secret

  The sidecar can now fetch policies along with TLS certificates via the API, enabling DNS over HTTPS connections to kube-dns.