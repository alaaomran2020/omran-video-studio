.PHONY: run test vet
run:
	go run ./cmd/studio
test:
	go test ./...
vet:
	go vet ./...
