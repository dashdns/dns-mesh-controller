.PHONY: build docker-buildx deploy clean test

IMAGE_NAME ?= emirozbir/dashdns-admission-webhook
IMG ?= latest
CONTAINER_TOOL ?= docker

build:
	go build -o bin/webhook ./cmd/webhook/main.go

PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name dashdns-admission-webhook
	$(CONTAINER_TOOL) buildx use dashdns-admission-webhook
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm dashdns-admission-webhook
	rm Dockerfile.cross

generate-certs:
	chmod +x scripts/generate-certs.sh
	./scripts/generate-certs.sh

deploy: generate-certs
	kubectl apply -f deploy/rbac.yaml
	kubectl apply -f deploy/secret.yaml
	kubectl apply -f deploy/deployment.yaml
	kubectl apply -f deploy/service.yaml
	@echo "Update deploy/webhook.yaml with the CA bundle from deploy/ca-bundle.txt"
	@echo "Then run: kubectl apply -f deploy/webhook.yaml"

undeploy:
	kubectl delete -f deploy/webhook.yaml --ignore-not-found=true
	kubectl delete -f deploy/service.yaml --ignore-not-found=true
	kubectl delete -f deploy/deployment.yaml --ignore-not-found=true
	kubectl delete -f deploy/secret.yaml --ignore-not-found=true
	kubectl delete -f deploy/rbac.yaml --ignore-not-found=true

clean:
	rm -rf bin/
	rm -f deploy/secret.yaml
	rm -f deploy/ca-bundle.txt

test:
	go test -v ./...
