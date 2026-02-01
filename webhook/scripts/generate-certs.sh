#!/bin/bash

set -e

# Variables
SERVICE_NAME="sidecar-injector-webhook"
NAMESPACE="default"
SECRET_NAME="sidecar-injector-certs"

# Create temp directory for certificates
TMP_DIR=$(mktemp -d)
echo "Creating certificates in ${TMP_DIR}"

# Generate CA key and certificate
openssl genrsa -out ${TMP_DIR}/ca.key 2048
openssl req -x509 -new -nodes -key ${TMP_DIR}/ca.key -subj "/CN=${SERVICE_NAME}.${NAMESPACE}.svc" -days 365 -out ${TMP_DIR}/ca.crt

# Generate server key
openssl genrsa -out ${TMP_DIR}/tls.key 2048

# Create CSR config
cat <<EOF > ${TMP_DIR}/csr.conf
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${SERVICE_NAME}
DNS.2 = ${SERVICE_NAME}.${NAMESPACE}
DNS.3 = ${SERVICE_NAME}.${NAMESPACE}.svc
DNS.4 = ${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local
EOF

# Generate CSR
openssl req -new -key ${TMP_DIR}/tls.key -subj "/CN=${SERVICE_NAME}.${NAMESPACE}.svc" -out ${TMP_DIR}/tls.csr -config ${TMP_DIR}/csr.conf

# Sign the certificate
openssl x509 -req -in ${TMP_DIR}/tls.csr -CA ${TMP_DIR}/ca.crt -CAkey ${TMP_DIR}/ca.key -CAcreateserial -out ${TMP_DIR}/tls.crt -days 365 -extensions v3_req -extfile ${TMP_DIR}/csr.conf

echo "Certificates generated successfully"

# Create Kubernetes secret
kubectl create secret generic ${SECRET_NAME} \
  --from-file=tls.key=${TMP_DIR}/tls.key \
  --from-file=tls.crt=${TMP_DIR}/tls.crt \
  --namespace=${NAMESPACE} \
  --dry-run=client -o yaml > deploy/secret.yaml

echo "Secret manifest created at deploy/secret.yaml"

# Export CA bundle for webhook configuration
CA_BUNDLE=$(cat ${TMP_DIR}/ca.crt | base64 | tr -d '\n')
echo "CA Bundle (use this in MutatingWebhookConfiguration):"
echo ${CA_BUNDLE}

# Save CA bundle to file
echo ${CA_BUNDLE} > deploy/ca-bundle.txt

# Cleanup
rm -rf ${TMP_DIR}

echo "Done! Apply the secret with: kubectl apply -f deploy/secret.yaml"
