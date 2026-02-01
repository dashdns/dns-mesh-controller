# DNS Mesh Controller

Kubernetes-native, eBPF-based DNS policy management system. Provides kernel-level DNS control at the pod level.

## Overview

DNS Mesh Controller is a Kubernetes operator that provides fine-grained DNS control for your workloads. The system is entirely **eBPF-based** and blocks/allows DNS queries at the kernel level.

### How It Works

```
+------------------+     +-------------------+     +------------------+
|   DnsPolicy CRD  | --> |    Controller     | --> |   API Server     |
|   (Block List)   |     | (Policy Indexing) |     |   (:5959)        |
+------------------+     +-------------------+     +------------------+
                                                           |
                                                           v
+------------------+     +-------------------+     +------------------+
|   Admission      | --> |   Pod DNS Config  | --> |  eBPF DaemonSet  |
|   Webhook        |     |   Injection       |     |  (Kernel-level)  |
+------------------+     +-------------------+     +------------------+
```

1. **DnsPolicy CRD** defines which domains to block/allow
2. **Controller** indexes policies and matches them with pod IPs
3. **Webhook** automatically redirects new pods' DNS configuration to the eBPF daemonset
4. **eBPF DaemonSet** intercepts DNS queries at kernel level and filters based on policies

### Features

- **Kernel-Level Enforcement**: DNS filtering via eBPF without userspace overhead
- **Zero-Touch Integration**: Automatic DNS injection via annotation
- **Label & Identity Based Targeting**: Target pods by labels or ServiceAccount
- **DNS over HTTPS (DoH)**: Encrypted DNS support
- **Dryrun Mode**: Safe testing in production
- **Dynamic Updates**: Policy updates without pod restarts

## Installation

### Prerequisites

- Kubernetes cluster v1.16+
- Helm 3.x
- eBPF-capable Linux kernel (5.4+)

### Install with Helm

```bash
# Pull the chart
helm pull oci://registry-1.docker.io/emirozbir/dns-mesh-controller --version 1.0.1-rc

# Install
helm install dns-mesh-controller oci://registry-1.docker.io/emirozbir/dns-mesh-controller \
  --version 1.0.1-rc \
  --namespace dns-mesh-system \
  --create-namespace
```

### Verify Installation

```bash
# Check pods
kubectl get pods -n dns-mesh-system

# Verify CRD is installed
kubectl get crd dnspolicies.dns.dnspolicies.io

# Check webhook configuration
kubectl get mutatingwebhookconfigurations
```

---

## 1. Controller

The Controller is the brain of the DNS Mesh system. It watches DnsPolicy CRDs, matches them with pods, and serves blocklist information to the eBPF daemonset.

### CRD: DnsPolicy

```yaml
apiVersion: dns.dnspolicies.io/v1alpha1
kind: DnsPolicy
metadata:
  name: my-dns-policy
  namespace: default
spec:
  # Targeting Method 1: Label Selector
  targetSelector:
    app: frontend
    tier: web

  # Targeting Method 2: ServiceAccount (Identity-based)
  # subject:
  #   serviceAccount: my-service-account

  # Blocked domain list
  blockList:
    - "*.malicious-site.com"
    - "tracking.ads.net"
    - "telemetry.example.com"

  # Enable DNS over HTTPS?
  doh: true

  # Test mode (log only, no blocking)
  dryrun: false
```

### Example 1: Label-Based Policy

Block specific domains from frontend applications:

```yaml
apiVersion: dns.dnspolicies.io/v1alpha1
kind: DnsPolicy
metadata:
  name: frontend-dns-policy
  namespace: default
spec:
  targetSelector:
    app: frontend
  blockList:
    - "*.ads.com"
    - "*.tracking.io"
    - "analytics.google.com"
```

```bash
kubectl apply -f - <<EOF
apiVersion: dns.dnspolicies.io/v1alpha1
kind: DnsPolicy
metadata:
  name: frontend-dns-policy
spec:
  targetSelector:
    app: frontend
  blockList:
    - "*.ads.com"
    - "*.tracking.io"
EOF
```

### Example 2: Identity-Based Policy (ServiceAccount)

Apply policy to all pods running with a specific ServiceAccount:

```yaml
apiVersion: dns.dnspolicies.io/v1alpha1
kind: DnsPolicy
metadata:
  name: payment-service-policy
  namespace: production
spec:
  subject:
    serviceAccount: payment-processor
  blockList:
    - "*"  # Block all external DNS
  doh: true
```

### Example 3: Testing with Dryrun Mode

Test your policy in production without blocking, just logging:

```yaml
apiVersion: dns.dnspolicies.io/v1alpha1
kind: DnsPolicy
metadata:
  name: test-policy
spec:
  dryrun: true  # Log only, no blocking
  targetSelector:
    environment: production
  blockList:
    - "*.suspicious-domain.com"
```

Check logs in dryrun mode:
```bash
kubectl logs -l app=dnsd-daemonset -n dns-mesh-system
```

### Controller API

The Controller hosts an API server that serves blocklist information to the eBPF daemonset:

```bash
# API endpoint (from within cluster)
curl http://dns-mesh-controller:5959/api/policies

# Example response:
{
  "blocklist": {
    "10.244.1.5": ["*.ads.com", "tracking.io"],
    "10.244.1.6": ["*.malicious.com"]
  }
}
```

### Controller Configuration

Controller settings via `values.yaml`:

```yaml
controller:
  replicaCount: 1
  image:
    repository: docker.io/emirozbir/dashdns-controller
    tag: "latest"
  service:
    type: ClusterIP
    port: 8443      # Metrics/webhook port
    apiPort: 5959   # Blocklist API port
  resources:
    limits:
      cpu: 500m
      memory: 128Mi
    requests:
      cpu: 10m
      memory: 64Mi
  # kube-dns TLS settings for DNS over HTTPS
  kubeDns:
    namespace: kube-system
    secretName: kube-dns-tls
```

### Policy Management

```bash
# List all policies
kubectl get dnspolicies -A

# View policy details
kubectl describe dnspolicy frontend-dns-policy

# Update policy
kubectl edit dnspolicy frontend-dns-policy

# Delete policy
kubectl delete dnspolicy frontend-dns-policy
```

---

## 2. Webhook

The Mutating Admission Webhook automatically redirects newly created Deployments' DNS configuration to the eBPF daemonset.

### How It Works

1. Deployment is created with `dashdns.io/watch: "true"` annotation
2. Webhook intercepts the Deployment
3. DNS configuration is injected into pod spec:
   - `dnsPolicy: None`
   - `nameserver`: eBPF daemonset service IP

### Enabling a Deployment

Add the annotation to your Deployment for the webhook to work:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    dashdns.io/watch: "true"  # This annotation activates the webhook
spec:
  replicas: 1
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: main
        image: nginx:latest
```

### Example: Frontend Application

```bash
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  annotations:
    dashdns.io/watch: "true"
spec:
  replicas: 2
  selector:
    matchLabels:
      app: frontend
  template:
    metadata:
      labels:
        app: frontend
    spec:
      containers:
      - name: frontend
        image: nginx:alpine
        ports:
        - containerPort: 80
EOF
```

Verify DNS configuration:
```bash
kubectl get pod -l app=frontend -o jsonpath='{.items[0].spec.dnsConfig}'
```

### Webhook Configuration

Webhook settings via `values.yaml`:

```yaml
webhook:
  image: emirozbir/dashdns-admission-webhook:latest
  replicas: 1
  # eBPF DaemonSet service information
  dns_service:
    name: dnsd-dashdns        # DaemonSet service name
    namespace: "default"       # DaemonSet namespace
  # TLS certificates (auto-generated if left empty)
  certChain:
    ca: ""
    cert: ""
    key: ""
  resources:
    limits:
      cpu: 500m
      memory: 128Mi
    requests:
      cpu: 100m
      memory: 64Mi
```

### Manual Configuration Without Webhook

If you don't want to use the webhook, you can add DNS configuration manually:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
spec:
  dnsPolicy: None
  dnsConfig:
    nameservers:
      - "10.96.100.100"  # eBPF daemonset service IP
    searches:
      - default.svc.cluster.local
      - svc.cluster.local
      - cluster.local
  containers:
  - name: app
    image: nginx
```

---

## Complete Scenario Example

### Scenario: E-commerce Application

1. **Install eBPF DaemonSet and DNS Mesh Controller**

```bash
helm install dns-mesh-controller oci://registry-1.docker.io/emirozbir/dns-mesh-controller \
  --version 1.0.1-rc \
  --namespace dns-mesh-system \
  --create-namespace
```

2. **Create DNS Policy for Frontend**

```bash
kubectl apply -f - <<EOF
apiVersion: dns.dnspolicies.io/v1alpha1
kind: DnsPolicy
metadata:
  name: frontend-policy
spec:
  targetSelector:
    app: frontend
    tier: web
  blockList:
    - "*.ads.com"
    - "*.tracking.io"
    - "malware.example.com"
  doh: true
EOF
```

3. **Create stricter policy for Backend**

```bash
kubectl apply -f - <<EOF
apiVersion: dns.dnspolicies.io/v1alpha1
kind: DnsPolicy
metadata:
  name: backend-strict-policy
spec:
  subject:
    serviceAccount: backend-sa
  blockList:
    - "*"  # Block everything except allowed
EOF
```

4. **Create Frontend Deployment**

```bash
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  annotations:
    dashdns.io/watch: "true"
spec:
  replicas: 2
  selector:
    matchLabels:
      app: frontend
      tier: web
  template:
    metadata:
      labels:
        app: frontend
        tier: web
    spec:
      containers:
      - name: frontend
        image: nginx:alpine
EOF
```

5. **Test DNS filtering**

```bash
# Connect to frontend pod
kubectl exec -it deploy/frontend -- sh

# Allowed domain
nslookup google.com
# Should succeed

# Blocked domain
nslookup ads.tracking.io
# Should fail (NXDOMAIN or timeout)
```

---

## Troubleshooting

### Controller Logs

```bash
kubectl logs -n dns-mesh-system -l app=dns-mesh-controller
```

### Webhook Logs

```bash
kubectl logs -n dns-mesh-system -l app=dashdns-webhook
```

### Policy Status

```bash
kubectl get dnspolicies -A -o wide
kubectl describe dnspolicy <policy-name>
```

### Webhook Configuration

```bash
kubectl get mutatingwebhookconfigurations -o yaml
```

### Pod DNS Configuration

```bash
kubectl get pod <pod-name> -o jsonpath='{.spec.dnsPolicy}'
kubectl get pod <pod-name> -o jsonpath='{.spec.dnsConfig}'
```

---

## Uninstall

```bash
# Delete all policies
kubectl delete dnspolicies --all -A

# Uninstall Helm release
helm uninstall dns-mesh-controller -n dns-mesh-system

# Delete namespace
kubectl delete namespace dns-mesh-system
```

---

## Helm Values Reference

All configuration options:

```yaml
webhook:
  image: emirozbir/dashdns-admission-webhook:latest
  replicas: 1
  pullPolicy: Always
  dns_service:
    name: dnsd-dashdns
    namespace: "default"
  certChain:
    ca: ""
    cert: ""
    key: ""
  resources:
    limits:
      cpu: 500m
      memory: 128Mi
    requests:
      cpu: 100m
      memory: 64Mi

controller:
  replicaCount: 1
  image:
    repository: docker.io/emirozbir/dashdns-controller
    pullPolicy: IfNotPresent
    tag: "latest"
  service:
    type: ClusterIP
    port: 8443
    apiPort: 5959
  kubeDns:
    namespace: kube-system
    secretName: kube-dns-tls
  resources:
    limits:
      cpu: 500m
      memory: 128Mi
    requests:
      cpu: 10m
      memory: 64Mi
  nodeSelector: {}
  tolerations: []
  affinity: {}
```

---

## License

Copyright 2025. Apache License, Version 2.0.
