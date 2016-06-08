APPENV ?= testenv
PROJECT := pracovnik
GITCOMMIT := $(shell git rev-parse --short HEAD)
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
GITUNTRACKEDCHANGES := $(shell git status --porcelain --untracked-files=no)
ifneq ($(GITUNTRACKEDCHANGES),)
 	GITCOMMIT := $(GITCOMMIT)-dirty
	endif

all: build

fmt:
	@gofmt -w ./

deps:
	docker-compose up -d
	docker run --link $(PROJECT)_postgresql:postgres aanand/wait

migrate:
	migrate -url $($(shell echo $(PROJECT) | tr a-z A-Z)_POSTGRES_CONN) -path ./migrations up

build: deps $(APPENV)
	docker run \
		--link $(PROJECT)_postgresql:postgresql \
		--env-file ./$(APPENV) \
		-e "TARGETS=linux/amd64" \
		-e GODEBUG=netdns=cgo \
		-e PROJECT=github.com/opsee/$(PROJECT) \
		-v `pwd`:/gopath/src/github.com/opsee/$(PROJECT) \
		quay.io/opsee/build-go:16
	docker build -t quay.io/opsee/$(PROJECT):$(GITCOMMIT) .

run: build $(APPENV)
	docker run \
		--link $(PROJECT)_postgresql:postgresql \
		--env-file ./$(APPENV) \
		-e GODEBUG=netdns=cgo \
		-e AWS_DEFAULT_REGION \
		-e AWS_ACCESS_KEY_ID \
		-e AWS_SECRET_ACCESS_KEY \
		--rm \
		quay.io/opsee/$(PROJECT):$(GITCOMMIT)

.PHONY: build run migrate all
