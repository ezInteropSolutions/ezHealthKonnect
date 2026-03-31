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
RUN npm ci --omit=dev

# Stage 3: Runtime image
FROM node:22-alpine

RUN apk upgrade --no-cache && apk add --no-cache curl

WORKDIR /app

# Node.js app
COPY --from=nodebuilder /app/node_modules ./node_modules
COPY package*.json    ./
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
