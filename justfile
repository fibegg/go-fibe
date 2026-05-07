set dotenv-load := true

check:
    go fmt ./...
    go vet ./...
    go test ./...
    GOROOT=$(go env GOROOT) go run github.com/99designs/gqlgen@v0.17.90 generate
    bash scripts/check-generated.sh
    npm --prefix frontend ci --prefer-offline --no-audit --no-fund
    npm --prefix frontend run typecheck
    npm --prefix frontend run lint
    npm --prefix frontend run build
    docker compose config --quiet
    bash scripts/check-migrations.sh
    bash scripts/optional-tools.sh
