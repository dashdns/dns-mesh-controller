#!/bin/bash
#
# DNS Mesh Controller - E2E Test Suite
# This script sets up a kind cluster, deploys the DNS mesh controller stack,
# and validates that DNS blocking policies work correctly.
#

set -euo pipefail

#######################################
# Configuration
#######################################
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly KIND_VERSION="v0.27.0"
readonly KIND_CLUSTER_NAME="dnsmesh"
readonly TEST_NAMESPACE="default"
readonly ROLLOUT_TIMEOUT="120s"
readonly LOG_COLLECTION_TIMEOUT=30
readonly EXPECTED_BLOCKED_QUERIES=3

# Image configuration
readonly WEB_HOOK_IMAGE_NAME="docker.io/emirozbir/dashdns-admission-webhook"
readonly CONTROLLER_IMAGE_NAME="docker.io/emirozbir/dashdns-controller"
readonly DNSD_IMAGE_NAME="emirozbir/dnsd"
readonly DNSD_IMAGE_TAG="v1.0.0-rc"

#######################################
# Logging utilities
#######################################
log_info() {
    echo "[INFO] $(date '+%Y-%m-%d %H:%M:%S') $*"
}

log_error() {
    echo "[ERROR] $(date '+%Y-%m-%d %H:%M:%S') $*" >&2
}

log_success() {
    echo "[PASS] $(date '+%Y-%m-%d %H:%M:%S') $*"
}

log_fail() {
    echo "[FAIL] $(date '+%Y-%m-%d %H:%M:%S') $*" >&2
}

#######################################
# Cleanup function
#######################################
cleanup() {
    local exit_code=$?
    log_info "Cleaning up test resources..."

    # Remove temporary log file
    rm -f "${SCRIPT_DIR}/tests/suite/manifests/logs.txt"

    # Optionally delete test deployments (cluster deletion handles this anyway)
    #kubectl delete -f "${SCRIPT_DIR}/tests/suite/manifests/." --ignore-not-found=true 2>/dev/null || true

    if [[ "${KEEP_CLUSTER:-false}" != "true" ]]; then
        log_info "Deleting kind cluster '${KIND_CLUSTER_NAME}'..."
        kind delete cluster --name "${KIND_CLUSTER_NAME}" 2>/dev/null || true
    else
        log_info "Keeping cluster as KEEP_CLUSTER=true"
    fi

    exit "${exit_code}"
}

#######################################
# Pre-flight checks
#######################################
check_prerequisites() {
    log_info "Checking prerequisites..."

    local missing_tools=()

    if ! command -v docker &>/dev/null; then
        missing_tools+=("docker")
    fi

    if ! command -v kubectl &>/dev/null; then
        missing_tools+=("kubectl")
    fi

    if ! command -v helm &>/dev/null; then
        missing_tools+=("helm")
    fi

    if [[ ${#missing_tools[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_error "Please install them before running this script."
        exit 1
    fi

    log_info "All prerequisites satisfied"
}

#######################################
# Install kind if not present
#######################################
install_kind() {
    if command -v kind &>/dev/null; then
        log_info "kind is already installed: $(kind version)"
        return 0
    fi

    log_info "Installing kind CLI ${KIND_VERSION}..."

    local arch
    local os
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"

    case "${arch}" in
        x86_64)
            arch="amd64"
            ;;
        aarch64|arm64)
            arch="arm64"
            ;;
        *)
            log_error "Unsupported architecture: ${arch}"
            exit 1
            ;;
    esac

    local kind_url="https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-${os}-${arch}"
    local kind_bin="${SCRIPT_DIR}/kind"

    if ! curl -fsSL -o "${kind_bin}" "${kind_url}"; then
        log_error "Failed to download kind from ${kind_url}"
        exit 1
    fi

    chmod +x "${kind_bin}"

    # Try to move to system path, fall back to local
    if sudo mv "${kind_bin}" /usr/local/bin/kind 2>/dev/null; then
        log_info "kind installed to /usr/local/bin/kind"
    else
        export PATH="${SCRIPT_DIR}:${PATH}"
        log_info "kind installed locally at ${kind_bin}"
    fi
}

#######################################
# Create kind cluster
#######################################
create_cluster() {
    log_info "Creating kind cluster '${KIND_CLUSTER_NAME}'..."

    # Delete existing cluster if it exists
    if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
        log_info "Cluster '${KIND_CLUSTER_NAME}' already exists, deleting..."
        kind delete cluster --name "${KIND_CLUSTER_NAME}"
    fi

    if ! kind create cluster \
        --name "${KIND_CLUSTER_NAME}" \
        --config "${SCRIPT_DIR}/tests/suite/manifests/cluster.yaml"; then
        log_error "Failed to create kind cluster"
        exit 1
    fi

    # Wait for cluster to be ready
    log_info "Waiting for cluster nodes to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout="${ROLLOUT_TIMEOUT}"

    log_info "Kind cluster '${KIND_CLUSTER_NAME}' is ready"
}

#######################################
# Load images into kind
#######################################
load_images() {
    local image_tag="$1"

    log_info "Loading container images into kind cluster..."

    local images=(
        "${WEB_HOOK_IMAGE_NAME}:${image_tag}"
        "${CONTROLLER_IMAGE_NAME}:${image_tag}"
    )

    for image in "${images[@]}"; do
        log_info "Loading image: ${image}"
        if ! kind load docker-image "${image}" --name "${KIND_CLUSTER_NAME}"; then
            log_error "Failed to load image: ${image}"
            exit 1
        fi
    done

    log_info "All images loaded successfully"
}

#######################################
# Deploy DNS mesh controller stack
#######################################
deploy_stack() {
    local image_tag="$1"

    log_info "Deploying DNS mesh contrlsoller stack via Helm..."
    ./webhook/scripts/generate-certs.sh
    # Deploy dns-mesh-controller

    CA_CERT=$(cat webhook/scripts/deploy/ca-bundle.txt)
    CERT=$(cat webhook/scripts/deploy/cert.txt|base64| tr -d '\n')
    CERT_KEY=$(cat webhook/scripts/deploy/key.txt|base64| tr -d '\n')

    echo $CERT_KEY
    echo $CERT
    helm upgrade --install dns-mesh-stack \
        --set controller.image.repository="${CONTROLLER_IMAGE_NAME}" \
        --set controller.image.tag="${image_tag}" \
        --set controller.image.pullPolicy="Always" \
        --set webhook.image.repository="${WEB_HOOK_IMAGE_NAME}" \
        --set webhook.image.tag="${image_tag}" \
        --set webhook.image.pullPolicy="Always" \
        --set webhook.certChain.ca="${CA_CERT}" \
        --set webhook.certChain.cert="${CERT}" \
        --set webhook.certChain.key="${CERT_KEY}" \
        --values "${SCRIPT_DIR}/charts/dns-mesh-controller/values.yaml" \
        "${SCRIPT_DIR}/charts/dns-mesh-controller" \
        --wait \
        --timeout "${ROLLOUT_TIMEOUT}"

    log_info "DNS mesh controller deployed"

    # Deploy dnsd
    helm upgrade --install dnsd \
        --set image.repository="${DNSD_IMAGE_NAME}" \
        --set image.tag="${DNSD_IMAGE_TAG}" \
        --values "${SCRIPT_DIR}/charts/dashdns/values.yaml" \
        "${SCRIPT_DIR}/charts/dashdns" \
        --wait \
        --timeout "${ROLLOUT_TIMEOUT}"

    kubectl get po -o wide
    log_info "DNSD deployed"
}

#######################################
# Wait for workload to be ready
#######################################
wait_for_workloads() {
    log_info "Waiting for all workloads to be ready..."

    # Wait for controller deployment
    kubectl rollout status deployment -l app.kubernetes.io/name=dns-mesh-controller \
        --timeout="${ROLLOUT_TIMEOUT}" 2>/dev/null || true

    # Wait for webhook deployment
    kubectl rollout status deployment -l app.kubernetes.io/name=dashdns-admission-webhook \
        --timeout="${ROLLOUT_TIMEOUT}" 2>/dev/null || true

    # Wait for daemonset
    kubectl rollout status daemonset/dnsd-dashdns \
        --timeout="${ROLLOUT_TIMEOUT}" 2>/dev/null || true

    log_info "All workloads are ready"
}

#######################################
# Run test scenario
#######################################
run_test_scenario() {
    log_info "Setting up test scenario..."

    local manifests_dir="${SCRIPT_DIR}/tests/suite/manifests"
    local logs_file="${manifests_dir}/logs.txt"

    # Apply CRD and test deployment
    kubectl apply -f "${manifests_dir}/crd.yaml"
    kubectl apply -f "${manifests_dir}/deployment.yaml"

    # Collect logs for analysis
    kubectl rollout status deployment/frontend-deployment --timeout="${ROLLOUT_TIMEOUT}"  2>/dev/null
    log_info "Collecting logs for ${LOG_COLLECTION_TIMEOUT} seconds..."
    timeout 45 kubectl logs -f -l app=frontend >> "${logs_file}" 2>/dev/null || true

    # Analyze results
    analyze_results "${logs_file}"
}

#######################################
# Analyze test results
#######################################
analyze_results() {
    local logs_file="$1"

    log_info "Analyzing test results..."

    if [[ ! -f "${logs_file}" ]]; then
        log_fail "Log file not found: ${logs_file}"
        exit 1
    fi

    local blocked_count
    blocked_count=$(grep -c "Ign:" "${logs_file}" 2>/dev/null)

    log_info "Number of blocked DNS queries: ${blocked_count}"

    if [[ "${blocked_count}" -gt "${EXPECTED_BLOCKED_QUERIES}" ]]; then
        log_success "DNS blocking policies are working correctly"
        log_success "Expected >${EXPECTED_BLOCKED_QUERIES} blocked queries, got ${blocked_count}"
        rm -f "${logs_file}"
        exit 0
    else
        log_fail "DNS blocking policies are NOT working as expected"
        log_fail "Expected >${EXPECTED_BLOCKED_QUERIES} blocked queries, got ${blocked_count}"
        log_info "--- Logs from test pod ---"
        cat "${logs_file}"
        log_info "--- End of logs ---"
        rm -f "${logs_file}"
        exit 1
    fi
}

#######################################
# Print usage
#######################################
usage() {
    cat <<EOF
Usage: $(basename "$0") <IMAGE_TAG> [OPTIONS]

DNS Mesh Controller E2E Test Suite

Arguments:
    IMAGE_TAG       The image tag to test (e.g., v0.0.1-rc)

Environment Variables:
    KEEP_CLUSTER    Set to 'true' to keep the kind cluster after tests (default: false)

Examples:
    $(basename "$0") v0.0.1-rc
    KEEP_CLUSTER=true $(basename "$0") v0.0.1-rc

EOF
}

#######################################
# Main
#######################################
main() {
    local image_tag="${1:-}"

    if [[ -z "${image_tag}" ]]; then
        log_error "Image tag is required"
        usage
        exit 1
    fi

    log_info "Starting DNS Mesh Controller E2E Test Suite"
    log_info "Testing image tag: ${image_tag}"

    # Set up cleanup trap
    trap cleanup EXIT

    # Run test pipeline
    check_prerequisites
    install_kind
    create_cluster
    deploy_stack "${image_tag}"
    wait_for_workloads
    run_test_scenario

    log_success "E2E Test Suite completed successfully"
}

main "$@"
