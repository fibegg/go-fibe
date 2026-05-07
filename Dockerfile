# syntax=docker/dockerfile:1.7

FROM node:24.11.1-bookworm-slim AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

FROM golang:1.26.2-bookworm AS backend
WORKDIR /app
ENV CGO_ENABLED=0
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go mod download
COPY . .
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/go-fibe ./cmd/go-fibe

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=backend /out/go-fibe /usr/local/bin/go-fibe
COPY --from=backend /app/migrations ./migrations
COPY --from=backend /app/frontend/dist ./frontend/dist
USER nonroot:nonroot
EXPOSE 3000
ENTRYPOINT ["/usr/local/bin/go-fibe"]
CMD ["serve"]

