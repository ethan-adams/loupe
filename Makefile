.DEFAULT_GOAL := help

demo: ## Run the live demo: an agent fixes a failing test, streamed step by step
	go run ./cmd/loupe demo

demo-fast: ## The demo with no pacing delay (full speed)
	go run ./cmd/loupe demo --pace 0

demo-ollama: ## The demo driven by a real local model (needs `ollama serve` + a pulled model)
	go run ./cmd/loupe demo --model ollama

demo-resume: ## Watch a run survive a worker crash: A commits the fix, dies, B resumes it
	go run ./cmd/loupe resume-demo

test: ## Run the tests
	go test ./...

build: ## Compile the loupe binary to ./bin/loupe
	go build -o bin/loupe ./cmd/loupe

fmt: ## Format the Go code
	go fmt ./...

vet: ## Static checks
	go vet ./...

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-13s\033[0m %s\n", $$1, $$2}'
