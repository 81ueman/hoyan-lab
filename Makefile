.PHONY: ci check fmt vet staticcheck test install-tools

STATICCHECK ?= staticcheck

ci: install-tools check

check: fmt vet staticcheck test

fmt:
	test -z "$$(gofmt -l .)"

vet:
	go vet ./...

staticcheck:
	$(STATICCHECK) ./...

test:
	go test ./...

install-tools:
	go install honnef.co/go/tools/cmd/staticcheck@latest
