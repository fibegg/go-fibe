# syntax=docker/dockerfile:1.7

FROM golang:1.26.2-alpine AS backend
WORKDIR /app
ENV CGO_ENABLED=0

RUN apk add --no-cache ca-certificates git

COPY --link go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go mod download
COPY --link . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/go-fibe ./cmd/go-fibe

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --link --from=backend /out/go-fibe /usr/local/bin/go-fibe
COPY --link --from=backend /app/migrations ./migrations
USER nonroot:nonroot
EXPOSE 3000
ENTRYPOINT ["/usr/local/bin/go-fibe"]
CMD ["serve"]
