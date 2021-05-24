# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: gone android ios gone-cross evm all test clean
.PHONY: gone-linux gone-linux-386 gone-linux-amd64 gone-linux-mips64 gone-linux-mips64le
.PHONY: gone-linux-arm gone-linux-arm-5 gone-linux-arm-6 gone-linux-arm-7 gone-linux-arm64
.PHONY: gone-darwin gone-darwin-386 gone-darwin-amd64
.PHONY: gone-windows gone-windows-386 gone-windows-amd64

GOBIN = ./build/bin
GO ?= latest
GORUN = env GO111MODULE=on go run

gone:
	$(GORUN) build/ci.go install ./cmd/gone
	@echo "Done building."
	@echo "Run \"$(GOBIN)/gone\" to launch gone."

all:
	$(GORUN) build/ci.go install

android:
	$(GORUN) build/ci.go aar --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/gone.aar\" to use the library."
	@echo "Import \"$(GOBIN)/gone-sources.jar\" to add javadocs"
	@echo "For more info see https://stackoverflow.com/questions/20994336/android-studio-how-to-attach-javadoc"

ios:
	$(GORUN) build/ci.go xcode --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/Gong.framework\" to use the library."

test: all
	$(GORUN) build/ci.go test

lint: ## Run linters.
	$(GORUN) build/ci.go lint

clean:
	env GO111MODULE=on go clean -cache
	rm -fr build/_workspace/pkg/ $(GOBIN)/*

# The devtools target installs tools required for 'go generate'.
# You need to put $GOBIN (or $GOPATH/bin) in your PATH to use 'go generate'.

devtools:
	env GOBIN= go get -u golang.org/x/tools/cmd/stringer
	env GOBIN= go get -u github.com/kevinburke/go-bindata/go-bindata
	env GOBIN= go get -u github.com/fjl/gencodec
	env GOBIN= go get -u github.com/golang/protobuf/protoc-gen-go
	env GOBIN= go install ./cmd/abigen
	@type "npm" 2> /dev/null || echo 'Please install node.js and npm'
	@type "solc" 2> /dev/null || echo 'Please install solc'
	@type "protoc" 2> /dev/null || echo 'Please install protoc'

# Cross Compilation Targets (xgo)

gone-cross: gone-linux gone-darwin gone-windows gone-android gone-ios
	@echo "Full cross compilation done:"
	@ls -ld $(GOBIN)/gone-*

gone-linux: gone-linux-386 gone-linux-amd64 gone-linux-arm gone-linux-mips64 gone-linux-mips64le
	@echo "Linux cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-*

gone-linux-386:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/386 -v ./cmd/gone
	@echo "Linux 386 cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-* | grep 386

gone-linux-amd64:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/amd64 -v ./cmd/gone
	@echo "Linux amd64 cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-* | grep amd64

gone-linux-arm: gone-linux-arm-5 gone-linux-arm-6 gone-linux-arm-7 gone-linux-arm64
	@echo "Linux ARM cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-* | grep arm

gone-linux-arm-5:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/arm-5 -v ./cmd/gone
	@echo "Linux ARMv5 cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-* | grep arm-5

gone-linux-arm-6:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/arm-6 -v ./cmd/gone
	@echo "Linux ARMv6 cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-* | grep arm-6

gone-linux-arm-7:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/arm-7 -v ./cmd/gone
	@echo "Linux ARMv7 cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-* | grep arm-7

gone-linux-arm64:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/arm64 -v ./cmd/gone
	@echo "Linux ARM64 cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-* | grep arm64

gone-linux-mips:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/mips --ldflags '-extldflags "-static"' -v ./cmd/gone
	@echo "Linux MIPS cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-* | grep mips

gone-linux-mipsle:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/mipsle --ldflags '-extldflags "-static"' -v ./cmd/gone
	@echo "Linux MIPSle cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-* | grep mipsle

gone-linux-mips64:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/mips64 --ldflags '-extldflags "-static"' -v ./cmd/gone
	@echo "Linux MIPS64 cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-* | grep mips64

gone-linux-mips64le:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/mips64le --ldflags '-extldflags "-static"' -v ./cmd/gone
	@echo "Linux MIPS64le cross compilation done:"
	@ls -ld $(GOBIN)/gone-linux-* | grep mips64le

gone-darwin: gone-darwin-386 gone-darwin-amd64
	@echo "Darwin cross compilation done:"
	@ls -ld $(GOBIN)/gone-darwin-*

gone-darwin-386:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=darwin/386 -v ./cmd/gone
	@echo "Darwin 386 cross compilation done:"
	@ls -ld $(GOBIN)/gone-darwin-* | grep 386

gone-darwin-amd64:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=darwin/amd64 -v ./cmd/gone
	@echo "Darwin amd64 cross compilation done:"
	@ls -ld $(GOBIN)/gone-darwin-* | grep amd64

gone-windows: gone-windows-386 gone-windows-amd64
	@echo "Windows cross compilation done:"
	@ls -ld $(GOBIN)/gone-windows-*

gone-windows-386:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=windows/386 -v ./cmd/gone
	@echo "Windows 386 cross compilation done:"
	@ls -ld $(GOBIN)/gone-windows-* | grep 386

gone-windows-amd64:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=windows/amd64 -v ./cmd/gone
	@echo "Windows amd64 cross compilation done:"
	@ls -ld $(GOBIN)/gone-windows-* | grep amd64
