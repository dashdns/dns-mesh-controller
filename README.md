# DNS Mesh Controller

A Kubernetes-native DNS policy management system that provides fine-grained DNS control for your workloads through EBPF based dns mesh system.

## Overview

DNS Mesh Controller enables you to control which DNS queries are allowed or blocked at the pod level using Custom Resource Definitions (CRDs). The system consists of two main components:

- **Controller**: Kubernetes operator that watches DnsPolicy CRDs and reconciles the desired state
- **Webhook**: Mutating admission webhook that automatically adjust DNS configuration of deployments and route them to the `dnsd` daemonsets.

## Project Structure

```
.
├── controller/     # DNS Mesh Controller operator
├── webhook/        # DNS Config injector webhook
└── deploy/         # Helm chart for deployment
```

## Documentation

Each component has its own detailed documentation:

| Component | Description | Documentation |
|-----------|-------------|---------------|
| Controller | Kubernetes operator for DNS policy management | [controller/README.md](controller/README.md) |
| Webhook | Mutating webhook for dns config injection | [webhook/README.md](webhook/README.md) |

## Installation

### Prerequisites

- Kubernetes cluster v1.16+
- Helm 3.x

### Deploy with Helm

The Helm chart is located in the `deploy/dns-mesh-controller` directory:

```bash
cd deploy/dns-mesh-controller
helm upgrade -i dns-mesh-controller -f values.yaml -n dns-mesh-controller-system --create-namespace .
```

### Verify Installation

```bash
kubectl get pods -n dns-mesh-controller-system
kubectl get crd dnspolicies.dns.dnspolicies.io
```

## License

Copyright 2025. Licensed under the Apache License, Version 2.0.
