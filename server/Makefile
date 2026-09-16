DOCKER_REPOSITORY=rcm7/stashito
PLATFORM ?= amd64
DOCKER_TAG := latest

.PHONY: build push build-local run-local test

build:
	docker build -f docker/Dockerfile --platform linux/${PLATFORM} -t ${DOCKER_REPOSITORY}:${DOCKER_TAG} .

push:
	docker push ${DOCKER_REPOSITORY}:${DOCKER_TAG}

build-local:
	go build -o bin/stashito-server .

run-local:
	set -a && . ./.env && set +a && go run .

test:
	go test ./...
