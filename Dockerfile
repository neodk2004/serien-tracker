# syntax=docker/dockerfile:1
ARG GO_VERSION=1.24


# --- Builder Stage ---
FROM golang:${GO_VERSION}-alpine AS builder

# Set working directory
WORKDIR /app

# Install build dependencies, including git and gcc for cgo
# gcc is needed for go-sqlite3 with cgo enabled
RUN apk add --no-cache git

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
# Download Go modules
# `go mod download` also handles `go mod tidy` implicitly for required modules
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the Go application
# -ldflags minimize the final binary size
# -trimpath removes file system paths from the resulting executable
# -o specifies the output file name
RUN GOOS=linux go build -ldflags="-s -w" -trimpath -o /app/series-tracker ./main.go

# --- Runner Stage ---
FROM alpine

# Install any runtime dependencies.
# For go-sqlite3, you might need sqlite-libs if it's not statically linked well,
# but often it's bundled. Testing will confirm.
# No specific runtime dependencies for the Go app after switching to PostgreSQL
# sqlite-libs is no longer needed

# Set working directory
WORKDIR /app

# Copy the built executable from the builder stage
COPY --from=builder /app/series-tracker /app/series-tracker

# Copy templates, static files, and fonts
COPY templates/ templates/
COPY static/ static/
COPY fonts/ fonts/

# Expose the port the application listens on
EXPOSE 8081

# Set the entry point to run the application
ENTRYPOINT ["/app/series-tracker"]
