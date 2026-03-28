# Build stage: compile Go and Node dependencies
FROM node:18-alpine AS builder

# Install build tools
RUN apk add --no-cache go git bash curl postgresql-client

# Set Go path
ENV GOPATH=/go PATH=$GOPATH/bin:/usr/local/go/bin:$PATH

WORKDIR /app

# Copy dependency files
COPY package*.json go.mod go.sum ./

# Install Node and Go dependencies
RUN npm install && go mod download

# Copy source code
COPY . .

# Build Go app
RUN go mod tidy && go build -o go-api main.go

# Runtime stage: minimal image with only runtime essentials
FROM node:18-alpine

# Only install runtime dependencies (no build tools)
RUN apk add --no-cache curl postgresql-client

WORKDIR /app

# Copy only what's needed from builder
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package*.json ./
COPY --from=builder /app/go-api ./
COPY --from=builder /app/server.js ./
COPY --from=builder /app/public ./public
COPY --from=builder /app/views ./views

# Expose ports
EXPOSE 3000 8080

# Start both Go backend and Node.js frontend
CMD ["sh", "-c", "./go-api & sleep 5 && node server.js"]
