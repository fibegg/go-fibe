set dotenv-load := true

check:
    go fmt ./...
    go vet ./...
    go test ./...
    GOROOT=$(go env GOROOT) go run github.com/99designs/gqlgen@v0.17.90 generate
    bash scripts/check-generated.sh
    docker compose config --quiet
    bash scripts/check-migrations.sh
    bash scripts/optional-tools.sh
