NAME=proxy
REPO=registry.blackwoodseven.com/blackwoodseven/$(NAME)
WORKDIR=/go/src/$(NAME)
DOCKER=docker run --rm -ti -v `pwd`:/go -w $(WORKDIR) --env CGO_ENABLED=0 golang:1.6

compile: dependencies
	$(DOCKER) go build -a -installsuffix cgo .

build: compile
	docker build -t $(REPO) .

push:
	docker push $(REPO)

watch: dependencies
	$(DOCKER) ginkgo watch

dependencies:
	test -s bin/ginkgo || ( $(DOCKER) go get github.com/onsi/ginkgo/ginkgo; )
	$(DOCKER) ginkgo bootstrap || true;
	$(DOCKER) go get -t ./...

test: dependencies
	$(DOCKER) go test ./...

fmt:
	$(DOCKER) go fmt ./...

.PHONY: compile build watch dependencies test init
