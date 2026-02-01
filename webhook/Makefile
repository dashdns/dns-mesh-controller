.PHONY: build docker-build deploy clean test

IMAGE_NAME ?= emirozbir/admission-webhook
IMAGE_TAG ?= latest

build:
	go build -o bin/webhook ./cmd/webhook/main.go

docker-build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

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
