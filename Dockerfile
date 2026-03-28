# ─── Stage 1: Build ───────────────────────────────────────────────────────────
# Compiles the Go binary and installs Node.js production dependencies.
FROM node:18-alpine AS builder

RUN apk add --no-cache go git bash

ENV GOPATH=/go
ENV PATH=$GOPATH/bin:/usr/local/go/bin:$PATH

WORKDIR /app

# Copy dependency manifests first (layer-cache friendly)
COPY package*.json go.mod go.sum ./

# Install Node.js production-only dependencies
RUN npm ci --omit=dev

# Download Go modules
RUN go mod download

# Copy all source code
COPY . .

# Compile Go backend
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o go-api main.go

# ─── Stage 2: Runtime ─────────────────────────────────────────────────────────
# Minimal image — only what the app needs to run.
FROM node:18-alpine

# curl: health-checks   postgresql-client: optional db diagnostics
RUN apk add --no-cache curl

WORKDIR /app

# ── Node.js application ───────────────────────────────────────────────────────
COPY --from=builder /app/node_modules    ./node_modules
COPY --from=builder /app/package*.json   ./
COPY --from=builder /app/app.js          ./app.js
COPY --from=builder /app/server.js       ./server.js

# Source directories required at runtime
COPY --from=builder /app/controllers     ./controllers
COPY --from=builder /app/services        ./services
COPY --from=builder /app/middleware      ./middleware
COPY --from=builder /app/routes          ./routes
COPY --from=builder /app/config          ./config
COPY --from=builder /app/models          ./models
COPY --from=builder /app/processing      ./processing
COPY --from=builder /app/utils           ./utils

# Static front-end assets
COPY --from=builder /app/public          ./public

# Flyway migrations (bind-mounted by the flyway service, but keep a copy in image)
COPY --from=builder /app/database        ./database

# NOTE: schemas/ is NOT baked into the image.
# Users download schema packs on-demand via Settings -> Schema Packages.

# ── Go backend binary ─────────────────────────────────────────────────────────
COPY --from=builder /app/go-api          ./go-api

# ── Runtime directories (created empty; volumes overlay in production) ────────
RUN mkdir -p schemas logs uploads

# ─── Ports ────────────────────────────────────────────────────────────────────
# 3000 = Node.js frontend   8080 = Go backend API
EXPOSE 3000 8080

# ─── Healthcheck ──────────────────────────────────────────────────────────────
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:3000/api/auth/session || exit 1

# ─── Startup ──────────────────────────────────────────────────────────────────
# Go backend starts first; Node waits 3 s to give it time to bind.
CMD ["sh", "-c", "./go-api & sleep 3 && node server.js"]
