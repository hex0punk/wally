# syntax=docker/dockerfile:1
FROM golang:latest

# Set the working directory inside the container
WORKDIR /go/src/app

# Copy the local package files to the container
COPY . .
RUN go mod download

# Build the app
RUN go build -o wally .

# Set the entry point to your binary
ENTRYPOINT ["./wally"]