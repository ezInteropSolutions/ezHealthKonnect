FROM node:18-alpine

# Install what we need
RUN apk add --no-cache go git bash curl postgresql-client

# Set Go path
ENV GOPATH=/go PATH=$GOPATH/bin:/usr/local/go/bin:$PATH

WORKDIR /app

# Copy and install dependencies
COPY package*.json go.mod go.sum ./
RUN npm install --only=production && go mod download

# Copy everything else
COPY . .

# Build Go app with different name to avoid conflict
RUN go mod tidy && go build -o go-api main.go

# Expose ports
EXPOSE 3000 8080

# Simple startup command - NO EXTERNAL SCRIPTS
CMD ["sh", "-c", "echo 'Starting...' && ./go-api & sleep 5 && node server.js"]