FROM golang:alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o ./server .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /build/server .
HEALTHCHECK --interval=5m --timeout=3s \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/health || exit 1
EXPOSE 8080/tcp
ENTRYPOINT ["./server"]
