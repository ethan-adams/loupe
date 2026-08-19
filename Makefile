.DEFAULT_GOAL := help

demo: ## Run the live demo: an agent fixes a failing test, streamed step by step
	go run ./cmd/loupe demo

demo-fast: ## The demo with no pacing delay (full speed)
	go run ./cmd/loupe demo --pace 0

demo-ollama: ## The demo driven by a real local model (needs `ollama serve` + a pulled model)
	go run ./cmd/loupe demo --model ollama

demo-resume: ## Watch a run survive a worker crash: A commits the fix, dies, B resumes it
	go run ./cmd/loupe resume-demo

code-consensus: ## Solve a real coding problem N ways, run each against tests, ship one that passes (needs Ollama)
	go run ./cmd/loupe code-consensus $(if $(N),--n $(N),)

eval: ## Score a model across a suite of coding problems and flag regressions vs the last run (needs Ollama)
	go run ./cmd/loupe eval $(if $(N),--n $(N),)

consensus: ## Answer one short question N ways, gate by majority vote + a judge (needs Ollama)
	go run ./cmd/loupe consensus $(if $(N),--n $(N),) $(if $(Q),--question "$(Q)",)

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

# --- Kubernetes on kind: the full thing on a local cluster ---
IMAGE ?= loupe:dev
CLUSTER ?= loupe

image: ## Build the container image and load it into the kind cluster
	docker build -t $(IMAGE) .
	kind load docker-image $(IMAGE) --name $(CLUSTER)

up: ## Create the kind cluster and deploy Postgres + Loupe (open http://localhost:8899)
	kind create cluster --name $(CLUSTER) --config deploy/kind.yaml
	$(MAKE) image
	kubectl apply -f deploy/k8s/postgres.yaml
	kubectl rollout status deploy/loupe-db --timeout=180s
	kubectl apply -f deploy/k8s/loupe.yaml
	kubectl rollout status deploy/loupe --timeout=180s
	@echo "Loupe is up at http://localhost:8899"

down: ## Delete the kind cluster
	kind delete cluster --name $(CLUSTER)

metrics-server: ## Install a kind-friendly metrics-server (needed for the HPA)
	kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
	kubectl patch deploy metrics-server -n kube-system --type=json \
		-p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
	kubectl rollout status deploy/metrics-server -n kube-system --timeout=180s

hpa: metrics-server ## Install metrics-server and apply the autoscaler
	kubectl apply -f deploy/k8s/hpa.yaml

loadtest: ## Fire the k6 load test at the cluster (VUS, DURATION, BASE_URL)
	k6 run loadtest/load.js

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
