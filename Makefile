ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

iv ?= v1

all: build

# Build binarys
build: fmt vet prepare csi

prepare:
	mkdir -p bin
	cp config/crd/* bin/

csi:
	go build -o bin/hyper cmd/main.go

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

dbg-image:
	buildctl b  --frontend dockerfile.v0 --local context=. --local dockerfile=. -o type=image,name=hub.easystack.io/csi-alcub/hyper:$(iv),push=true

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) crd paths="./..."
	$(CONTROLLER_GEN) object paths="./..."

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif


.PHONY: copy-csi-alcub
copy-csi-alcub: prepare
	@export GO111MODULE=off
	@export GOPROXY="https://goproxy.cn,direct"
	@export PATH=$(GOBIN):$(PATH)
	@mkdir -p $(GOPATH)/src/yylt
	@cp -r $(shell command pwd;) $(GOPATH)/src/yylt
	@cd "$(GOPATH)/src/yylt/csi-alcub"
	@go build -o bin/hyper cmd/main.go
	@echo "$(GOPATH)/src/yylt/csi-alcub/bin/hyper"

.PHONY: test-style
test-style:
	@echo "TODO"

.PHONY: test-unit
test-unit:
	@echo "TODO"

.PHONY: coverage
coverage:
	@echo "TODO"
