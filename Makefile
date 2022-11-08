REPOSITORY ?= "tombokombo"
APP_NAME ?= "pod-as-resource"
GIT_HASH ?= $(shell git log --format="%h" -n 1)

.PHONY: help

help: ## This help.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help

build:
	docker build --tag ${REPOSITORY}/${APP_NAME}:${GIT_HASH} .
push:
    docker push ${DOCKER_USERNAME}/${APPLICATION_NAME}
