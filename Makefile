PLATFORM ?= amd64
DOCKER_TAG := latest

default: help

help:
	@cat $(MAKEFILE_LIST)

build:
	make -C server/ build

push:
	make -C server/ push

build-docs:
	cd docs && npm install && npm run build

build-local:
	make -C server/ build-local

run-local:
	make -C server/ run-local

test:
	make -C server/ test

lint:
	make -C server/ lint
