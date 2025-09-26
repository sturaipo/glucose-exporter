include ${PWD}/.env
export 

.PHONY: clean
clean:
	@echo "Cleaning up..."
	rm -rf --one-file-system build/

.PHONY: build
build: clean
	@echo "Building the application..."

	CGO_ENABLED=0 go build -o build/glucose-exporter cli/main.go

.PHONY: run
run:
	@echo "Running the application..."
	LOG_LEVEL="debug" \
	LOG_FORMAT="console" \
	CGO_ENABLED=0 go run cli/main.go 