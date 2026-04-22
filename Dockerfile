FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o vitra-mcp ./cmd/mcp

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/vitra-mcp .
EXPOSE 3000
CMD ["./vitra-mcp"]
