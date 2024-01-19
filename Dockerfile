# syntax=docker/dockerfile:1
FROM golang:latest

# Set the working directory inside the container
WORKDIR /go/src/app

# Copy the local package files to the container
COPY . .
RUN go mod download

# Build the app
RUN go build -o wally .

ENTRYPOINT ["/go/src/app/wally"]

# Copy the entire project to the container
COPY *.go ./

# Build the app
RUN go build -o wally .
RUN go install .

# Set the entry point to a shell
ENTRYPOINT ["wally"]

# Default command to run when the container starts
CMD ["--help"]
