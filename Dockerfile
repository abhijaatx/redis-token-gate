# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/redis-token-gate ./cmd/redis-token-gate

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/redis-token-gate /redis-token-gate
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/redis-token-gate"]
