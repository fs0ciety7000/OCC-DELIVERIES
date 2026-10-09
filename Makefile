.PHONY: dev dev-api dev-web test test-api test-web lint build docker up

dev: ## API (:8090) + front (:5173) en parallèle
	$(MAKE) -j2 dev-api dev-web

dev-api:
	cd backend && go run . serve --http=127.0.0.1:8090

dev-web:
	cd frontend && npm run dev

test: test-api test-web

test-api:
	cd backend && go vet ./... && test -z "$$(gofmt -l .)" && go test ./...

test-web:
	cd frontend && npm run typecheck && npm run lint && npm test

build:
	cd frontend && npm run build
	rm -rf backend/pb_public && cp -r frontend/dist backend/pb_public
	cd backend && CGO_ENABLED=0 go build -o occ .

docker:
	docker build -t occ-deliveries:local .

up:
	docker compose up --build
