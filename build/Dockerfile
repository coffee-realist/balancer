# Stage 1: Build binary
FROM golang:1.24-alpine3.21 AS builder

WORKDIR /app

# Устанавливаем зависимости для CGO и SQLite
RUN apk add --no-cache git gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Включаем CGO для работы с SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -o balancer ./cmd

# Stage 2: Runtime image
FROM alpine:3.18

# Устанавливаем runtime зависимости
RUN apk add --no-cache libc6-compat

RUN addgroup -S app && adduser -S -G app app
WORKDIR /app

COPY --from=builder /app/balancer /app/balancer
COPY configs/config.yaml /app/configs/config.yaml

# Создаем директории и назначаем права
RUN mkdir -p /app/data && chown -R app:app /app
VOLUME ["/app/data"]

USER app

ENTRYPOINT ["/app/balancer", "-config", "/app/config.yaml"]