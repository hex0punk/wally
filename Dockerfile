<<<<<<< HEAD
# syntax=docker/dockerfile:1
=======
>>>>>>> 50f041c0b567c3097082fe03e5576d1c932bdd7f
FROM golang:latest

# Set the working directory inside the container
WORKDIR /go/src/app

# Copy the local package files to the container
COPY . .
RUN go mod download

<<<<<<< HEAD
# Build the app
RUN go build -o wally .

# Set the entry point to your binary
ENTRYPOINT ["./wally"]
=======
# Copy the entire project to the container
COPY *.go ./

# Build the app
RUN go build -o wally .

# Set the entry point to a shell
ENTRYPOINT ["/bin/sh", "-c"]

# Default command to run when the container starts
CMD ["./wally"]
>>>>>>> 50f041c0b567c3097082fe03e5576d1c932bdd7f
