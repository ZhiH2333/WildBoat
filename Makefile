.PHONY: up down logs test test-integration fmt vet tidy smoke

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f --tail=200

test:
	go test ./...

test-integration:
	go test ./internal/persistence -run TestMigrateAndPing -count=1

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy

smoke:
	./scripts/smoke.sh
