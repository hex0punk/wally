FROM golang:latest

# Set the working directory inside the container
WORKDIR /go/src/app

# Copy the local package files to the container
COPY . .
RUN go mod download

# Copy the entire project to the container
COPY *.go ./

# Build the app
RUN go build -o wally .

# Set the entry point to a shell
ENTRYPOINT ["/bin/sh", "-c"]

# Default command to run when the container starts
CMD ["./wally"]