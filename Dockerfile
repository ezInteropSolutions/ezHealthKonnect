# Stage 1: Compile Go binary
#
# --platform=$BUILDPLATFORM + explicit GOOS/GOARCH cross-compiles natively on
# the build host instead of emulating the target arch via QEMU. Go's toolchain
# cross-compiles for free; there's no reason to pay for (or risk) emulation.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS gobuilder

ARG TARGETOS
ARG TARGETARCH

RUN apk upgrade --no-cache && apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o go-api .

# Stage 2: Install Node.js production dependencies
#
# --platform=$BUILDPLATFORM runs npm natively on the build host rather than
# under QEMU emulation for the target arch — under emulation, `npm ci` on
# node:22-alpine reliably crashes with SIGILL (exit 132) building linux/arm64
# on an amd64 runner. Safe here because every prod dependency (see
# package.json) is pure JS with no native/prebuilt-binary addons — the
# resulting node_modules is architecture-independent and copied as-is into
# the target-platform runtime image in Stage 3.
FROM --platform=$BUILDPLATFORM node:22-alpine AS nodebuilder

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
    # node:22-alpine ships with npm 10.9.x, which bundles several transitive
    # deps with known CVEs (picomatch CVE-2026-33671, plus tar CVE-2026-59873/
    # 59874/73566, brace-expansion CVE-2026-13149/14257/69152, ip-address
    # CVE-2026-69192, sigstore CVE-2026-48815 — found via Trivy image scan).
    # npm cannot self-upgrade on Alpine. Since our container never invokes npm
    # at runtime (app.js/server.js only use the already-installed node_modules
    # copied in below), removing these bundled copies is safe and fixes the
    # vulnerabilities at the image level rather than suppressing the scan.
    rm -rf /usr/local/lib/node_modules/npm/node_modules/picomatch \
           /usr/local/lib/node_modules/npm/node_modules/tar \
           /usr/local/lib/node_modules/npm/node_modules/brace-expansion \
           /usr/local/lib/node_modules/npm/node_modules/ip-address \
           /usr/local/lib/node_modules/npm/node_modules/sigstore

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

# Architecture/product docs — read by the Go AI knowledge-ingestion service at
# runtime (services/ai/knowledge_ingestion.go's IngestAppDocs walks this dir
# for *.md, plus the generated pipeline_step_docs.json under generated/).
# Previously missing here entirely, so that ingestion path was a silent no-op
# in any deployment relying on the built image rather than a bind mount.
COPY architecture/    ./architecture/

# Go binary (compiled in gobuilder)
COPY --from=gobuilder /app/go-api ./go-api

# CDA/USCDI schema files — read by Go binary at runtime for CDA parsing
COPY cda/         ./cda/
COPY uscdi/       ./uscdi/

# Runtime directories (overlaid by named volumes in production)
RUN mkdir -p schemas logs uploads

EXPOSE 3000 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:3000/health || exit 1

CMD ["sh", "-c", "./go-api & sleep 3 && node server.js"]
