.PHONY: dev web api test e2e audit icons build desktop seed

dev:
	cd apps/web && npm run dev

api:
	go run ./cmd/sentrymed --dev

web:
	cd apps/web && npm ci && npm run build

test:
	go test ./...
	cd apps/web && npm test -- --run
	cd apps/web && npm run typecheck:e2e

e2e: web
	cd apps/web && npm run test:e2e

audit:
	go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
	cd apps/web && npm audit --audit-level=high

icons:
	go run ./scripts/generate-icons.go

build: web
	go build -o build/bin/sentrymed-server ./cmd/sentrymed

desktop: web
	wails build

seed:
	go run ./cmd/sentrymed --seed
