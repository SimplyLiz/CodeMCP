FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /ckb ./cmd/ckb

FROM alpine:3.21

RUN apk add --no-cache git ca-certificates

COPY --from=builder /ckb /usr/local/bin/ckb

WORKDIR /workspace

ENTRYPOINT ["ckb", "mcp"]
