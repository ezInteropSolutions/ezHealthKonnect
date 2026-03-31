# Stage 1: Compile Go binary
FROM golang:1.25-alpine AS gobuilder

RUN apk upgrade --no-cache && apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o go-api main.go

# Stage 2: Install Node.js production dependencies
FROM node:22-alpine AS nodebuilder

RUN apk upgrade --no-cache

WORKDIR /app

COPY package*.json ./
RUN npm ci --omit=dev && \
    node -e "const v=require('./node_modules/picomatch/package.json').version; \
    if(v!=='4.0.4'){console.error('FAIL: picomatch '+v+' (need 4.0.4)');process.exit(1);} \
    console.log('OK: picomatch '+v);"

# Stage 3: Runtime image
FROM node:22-alpine

RUN apk upgrade --no-cache && apk add --no-cache curl && \
    # node:22-alpine ships with npm 10.9.x which bundles picomatch 4.0.3
    # (CVE-2026-33671). npm cannot self-upgrade on Alpine. Since our container
    # never invokes npm at runtime, removing the bundled copy is safe and fixes
    # the vulnerability at the image level rather than suppressing the scan.
    rm -rf /usr/local/lib/node_modules/npm/node_modules/picomatch

WORKDIR /app

# Node.js app
COPY --from=nodebuilder /app/node_modules ./node_modules
# Only copy package.json (not package-lock.json) — the lockfile is build
# infrastructure only; it is not needed at runtime and confuses Trivy into
# reporting false-positive CVEs from nested dependency declarations.
COPY package.json     ./
COPY app.js           ./
COPY server.js        ./
COPY controllers/     ./controllers/
COPY services/        ./services/
COPY middleware/      ./middleware/
COPY routes/          ./routes/
COPY config/          ./config/
COPY models/          ./models/
COPY processing/      ./processing/
COPY utils/           ./utils/
COPY public/          ./public/
COPY database/        ./database/

# Go binary (compiled in gobuilder)
COPY --from=gobuilder /app/go-api ./go-api

# Runtime directories (overlaid by named volumes in production)
RUN mkdir -p schemas logs uploads

EXPOSE 3000 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:3000/health || exit 1

CMD ["sh", "-c", "./go-api & sleep 3 && node server.js"]
