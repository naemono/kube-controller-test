GOFILES:=$(shell find . -name '*.go' | grep -v -E '(./vendor)')

all: \
	bin/linux/mongodb-controller

images: GVERSION=$(shell $(CURDIR)/git-version.sh)
images: bin/linux/mongodb-controller
	docker build -f Dockerfile-controller -t naemono/mongodb-controller:$(GVERSION) .
	LATEST=`docker images | head | tail -n 1 | awk '{print $3}'`
	docker tag $(LATEST) naemono/mongodb-controller:latest

check:
	@find . -name vendor -prune -o -name '*.go' -exec gofmt -s -d {} +
	@go vet $(shell go list ./... | grep -v '/vendor/')
	@go test -v $(shell go list ./... | grep -v '/vendor/')

vendor:
	dep ensure

clean:
	rm -rf bin

bin/%: LDFLAGS=-X github.com/naemono/kube-controller-test/common.Version=$(shell $(CURDIR)/git-version.sh)
bin/%: $(GOFILES)
	mkdir -p $(dir $@)
	glide up -v
	GOOS=$(word 1, $(subst /, ,$*)) GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $@ github.com/naemono/kube-controller-test/$(notdir $@)
