NAME ?= filelistbuilder
VERSION := 0.1.7

#go install golang.org/x/tools/cmd/goimports@latest
#go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
#go install golang.org/x/lint/golint@latest
#go install honnef.co/go/tools/cmd/staticcheck@v0.2.2

# CICD triggers this
.PHONY: set-variable
set-version:
	@echo "VERSION=${VERSION}" >> $$GITHUB_ENV
	
sanitize:
	rm -rf vendor/
	go vet -tags ci ./...
	test -z $(goimports -e -d . | tee /dev/stderr)
	gocyclo -over 30 .
	golint -set_exit_status $(go list -tags ci ./...)
	staticcheck -go 1.14 ./...
	go test -tags ci -covermode=atomic -coverprofile=coverage.out ./...
    coverage=`go tool cover -func coverage.out | grep total | tr -s '\t' | cut -f 3 | grep -o '[^%]*'`
run: build-linux
	cd bin && mkdir -p rof
	cd bin/rof && echo "[LoginServer]" > eqhost.txt && echo "host=test.com:9000" >> eqhost.txt
	cd bin && ./filelistbuilder-linux-x64 rof https://test.com filelistbuilder-linux-x64
run-nexus: build-linux
	cd bin && mkdir -p rof
	cd bin/rof && echo "[LoginServer]" > eqhost.txt && echo "host=test.com:9000" >> eqhost.txt
	cd bin && ./filelistbuilder-linux-x64 rof https://test.com filelistbuilder-linux-x64 thj "The Heroes Journey" "https://raw.githubusercontent.com/The-Heroes-Journey-EQEMU/eqemupatcher/refs/heads/master/rof" "https://raw.githubusercontent.com/The-Heroes-Journey-EQEMU/eqemupatcher/refs/heads/master/rof" "0.1" "https://heroesjourneyemu.com/" "The Heroes’ Journey is a new spin on an old classic. Aiming to give players the chance to relive some of their fondest memories as the heroes they once were. Only this time instead of being bound to a single class path, players may choose to walk three paths at the same time, attaining incredible power through thousands of AAs, spells, and disciplines while personally progressing through each expansion seeking out Enchanted and Legendary versions of their favorite gear." "66.70.227.48" "dinput8.dll"
.PHONY: build-all
build-all: sanitize build-prepare build-linux build-darwin build-windows	
.PHONY: build-prepare
build-prepare:
	@echo "Preparing talkeq ${VERSION}"
	@rm -rf bin/*
	@-mkdir -p bin/
.PHONY: build-darwin
build-darwin:
	@echo "Building darwin ${VERSION}"
	@GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -buildmode=pie -ldflags="-X main.Version=${VERSION} -s -w" -o bin/${NAME}-darwin-x64 main.go
.PHONY: build-linux
build-linux:
	@echo "Building Linux ${VERSION}"
	@GOOS=linux GOARCH=amd64 go build -buildmode=pie -ldflags="-X main.Version=${VERSION} -w" -o bin/${NAME}-linux-x64 main.go		
.PHONY: build-windows
build-windows:
	@echo "Building Windows ${VERSION}"
	@GOOS=windows GOARCH=amd64 go build -buildmode=pie -ldflags="-X main.Version=${VERSION} -s -w" -o bin/${NAME}-win-x64.exe main.go
	@GOOS=windows GOARCH=386 go build -buildmode=pie -ldflags="-X main.Version=${VERSION} -s -w" -o bin/${NAME}-win-x86.exe main.go