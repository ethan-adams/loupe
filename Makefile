.DEFAULT_GOAL := help

demo: ## Run the live demo: an agent fixes a failing test, streamed step by step
	go run ./cmd/loupe demo

demo-fast: ## The demo with no pacing delay (full speed)
	go run ./cmd/loupe demo --pace 0

demo-ollama: ## The demo driven by a real local model (needs `ollama serve` + a pulled model)
	go run ./cmd/loupe demo --model ollama

demo-resume: ## Watch a run survive a worker crash: A commits the fix, dies, B resumes it
	go run ./cmd/loupe resume-demo

db-up: ## Start the Postgres run store in Docker
	docker compose up -d db

db-down: ## Stop and remove the Postgres run store
	docker compose down -v

submit: ## Enqueue a run in Postgres (TASK="..." optional). Prints the run id
	go run ./cmd/loupe submit $(if $(TASK),--task "$(TASK)",)

worker: ## Drain the Postgres queue, streamed live (ID=name MODEL=scripted|ollama)
	go run ./cmd/loupe worker $(if $(ID),--id $(ID),) $(if $(MODEL),--model $(MODEL),)

serve: ## Serve GraphQL + playground + live SSE, with embedded workers (needs db-up)
	go run ./cmd/loupe serve $(if $(MODEL),--model $(MODEL),)

generate: ## Regenerate the GraphQL code from internal/gql/schema.graphqls
	go run github.com/99designs/gqlgen generate

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
