FROM node:18-alpine

# Install what we need
RUN apk add --no-cache go git bash curl postgresql-client

# Set Go path
ENV GOPATH=/go PATH=$GOPATH/bin:/usr/local/go/bin:$PATH

WORKDIR /app

# Copy and install dependencies
COPY package*.json go.mod go.sum ./
RUN npm install && go mod download

# Copy everything else
COPY . .

# Build Go app
RUN go mod tidy && go build -o go-api main.go

# Expose ports
EXPOSE 3000 8080

# Start both Go backend and Node.js frontend
# Recompile Go at startup to pick up volume-mounted source changes
CMD ["sh", "-c", "echo 'Recompiling Go...' && go build -o go-api main.go && echo 'Go build complete' && ./go-api & sleep 5 && node server.js"]