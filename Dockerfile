FROM golang:latest AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o ./server ./cmd/server/main.go

FROM alpine:latest

WORKDIR /app
COPY --from=builder /build/server .

HEALTHCHECK --interval=5m --timeout=3s \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

EXPOSE 8080/tcp
ENTRYPOINT ["./server"]
