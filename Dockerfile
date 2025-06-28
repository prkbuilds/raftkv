FROM golang:1.24

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Copy local replace modules before downloading deps
COPY raft ./raft
COPY proto ./proto

# Download modules
RUN go mod download

# Copy the rest of the code
COPY . .

# Build the binary
RUN go build -o raft-server server.go

EXPOSE 50051

CMD ["./raft-server"]

