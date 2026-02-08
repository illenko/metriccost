FROM node:22-alpine AS frontend

WORKDIR /build/web

COPY web/package.json web/package-lock.json* ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /build/web/dist ./api/dist

ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=$(git rev-parse --short HEAD) -X main.buildTime=$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
    -o whodidthis .

FROM alpine:3.23

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 1000 whodidthis

WORKDIR /app

COPY --from=builder /build/whodidthis /app/whodidthis

RUN mkdir -p /app/data && chown -R whodidthis:whodidthis /app

USER whodidthis

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/api/health"]

ENTRYPOINT ["/app/whodidthis"]