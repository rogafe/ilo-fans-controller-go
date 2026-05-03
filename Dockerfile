# Stage 1: Build frontend assets
FROM node:20-alpine AS frontend
WORKDIR /app

# Install pnpm
RUN npm install -g pnpm

# Copy package files and install dependencies
COPY package.json pnpm-lock.yaml* ./
RUN pnpm install

# Copy frontend source files
COPY . .

RUN pnpm approve-builds 
# Build frontend assets
RUN pnpm run build:css
RUN pnpm run build:js

# Stage 2: Build Go application
FROM golang:1.26-alpine AS backend
WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod files and download dependencies
COPY go.mod go.sum* ./
RUN go mod download

# Copy Go source code
COPY . .

# Copy built assets from frontend stage
COPY --from=frontend /app/assets/dist ./assets/dist

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ilo-fans-controller-go .

# Stage 3: Final image
FROM alpine:latest
WORKDIR /app

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Copy binary from build stage
COPY --from=backend /app/ilo-fans-controller-go .

# Copy views and static assets
COPY --from=backend /app/views ./views
COPY --from=frontend /app/assets/dist ./assets/dist

# Environment variables
ENV ENV=production

# Expose the application port
EXPOSE 3000

# Run the application
CMD ["./ilo-fans-controller-go"]
