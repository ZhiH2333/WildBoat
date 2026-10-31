.PHONY: up down logs test fmt vet

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f --tail=200

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...
