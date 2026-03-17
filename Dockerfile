# ── Build stage ──────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o nisaba .

# ── Runtime stage ─────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/nisaba .

# All assets (templates, static, schema.sql) are embedded in the binary via go:embed

EXPOSE 8080

ENV NISABA_PORT=8080
ENV NISABA_DB_PATH=/data/nisaba.db

VOLUME ["/data"]

CMD ["./nisaba"]
