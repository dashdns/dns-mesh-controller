# Sidecar Injector

A Kubernetes mutating admission webhook that automatically injects sidecar containers into Deployments.

## Features

- Automatic sidecar injection for Deployments with annotation
- JSON Patch-based mutation
- TLS-secured webhook server
- Health check endpoints

## Prerequisites

- Kubernetes cluster (1.16+)
- kubectl configured
- Docker (for building images)
- OpenSSL (for certificate generation)

## Quick Start

### 1. Build the Docker image

```bash
make docker-build
```

If using a remote registry:
```bash
docker build -t <your-registry>/sidecar-injector:latest .
docker push <your-registry>/sidecar-injector:latest
```

Update the image in `deploy/deployment.yaml` accordingly.

### 2. Generate TLS certificates

```bash
make generate-certs
```

This will create:
- `deploy/secret.yaml` - Kubernetes secret with TLS certificates
- `deploy/ca-bundle.txt` - CA bundle for webhook configuration

### 3. Update webhook configuration

Copy the CA bundle from `deploy/ca-bundle.txt` and replace `<CA_BUNDLE>` in `deploy/webhook.yaml`:

```bash
CA_BUNDLE=$(cat deploy/ca-bundle.txt)
sed -i "s|<CA_BUNDLE>|${CA_BUNDLE}|g" deploy/webhook.yaml
```

### 4. Deploy the webhook

```bash
kubectl apply -f deploy/rbac.yaml
kubectl apply -f deploy/secret.yaml
kubectl apply -f deploy/deployment.yaml
kubectl apply -f deploy/service.yaml
kubectl apply -f deploy/webhook.yaml
```

Or use the Makefile (note: you'll still need to update webhook.yaml manually):
```bash
make deploy
# Then manually update deploy/webhook.yaml with CA bundle
kubectl apply -f deploy/webhook.yaml
```

### 5. Verify deployment

```bash
kubectl get pods
kubectl logs -l app=sidecar-injector
```

## Usage

To enable sidecar injection for a Deployment, add the annotation:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    sidecar-injector.io/inject: "true"
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

Apply the deployment:
```bash
kubectl apply -f examples/sample-deployment.yaml
```

Check that the sidecar was injected:
```bash
kubectl get deployment my-app -o jsonpath='{.spec.template.spec.containers[*].name}'
```

You should see both `main` and `sidecar` containers.

## Customization

### Modify the sidecar container

Edit `pkg/webhook/mutation.go` and update the `sidecarContainer` definition in the `createPatch` function:

```go
sidecarContainer := corev1.Container{
    Name:  "sidecar",
    Image: "your-sidecar-image:tag",
    // Add your configuration here
}
```

### Change the annotation

Update the `sidecarAnnotation` constant in `pkg/webhook/mutation.go`:

```go
const sidecarAnnotation = "your-custom-annotation"
```

## Cleanup

```bash
make undeploy
```

Or manually:
```bash
kubectl delete -f deploy/webhook.yaml
kubectl delete -f deploy/service.yaml
kubectl delete -f deploy/deployment.yaml
kubectl delete -f deploy/secret.yaml
kubectl delete -f deploy/rbac.yaml
```

## Troubleshooting

### Webhook not injecting

1. Check webhook pod logs:
   ```bash
   kubectl logs -l app=sidecar-injector
   ```

2. Verify webhook configuration:
   ```bash
   kubectl get mutatingwebhookconfiguration sidecar-injector-webhook -o yaml
   ```

3. Check if annotation is correct:
   ```bash
   kubectl get deployment <name> -o jsonpath='{.metadata.annotations}'
   ```

### Certificate issues

If you see TLS errors, regenerate certificates:
```bash
make clean
make generate-certs
kubectl delete secret sidecar-injector-certs
kubectl apply -f deploy/secret.yaml
kubectl rollout restart deployment/sidecar-injector-webhook
```

## Architecture

The webhook server intercepts Deployment creation/update requests and:
1. Checks for the injection annotation
2. Creates a JSON Patch to add the sidecar container
3. Returns the patch to the API server
4. The API server applies the patch and creates the modified Deployment

## License

MIT
