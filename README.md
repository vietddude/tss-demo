# TSS Demo

A demonstration of threshold ECDSA signature scheme implementation using gRPC for node communication.

## Overview

This project implements a distributed threshold signature scheme where multiple parties can collaboratively generate and use ECDSA keys without any single party having access to the complete private key.

### Key Components

- Threshold ECDSA signatures using [bnb-chain/tss-lib](https://github.com/bnb-chain/tss-lib)
- gRPC for inter-node communication
- HTTP API endpoints for wallet operations

## Features

- Distributed Key Generation (DKG)
- Threshold signatures (t-of-n)
- Configurable threshold parameters
- Fault tolerance
- RESTful API interface
- Secure P2P communication

## API Endpoints

### Create Wallet

```http
POST /wallet/create
Content-Type: application/json

{
    "threshold": 2,
    "party_ids": [1, 2, 3]
}
```

### Sign Message

```http
POST /wallet/sign
Content-Type: application/json

{
    "message": "base64_encoded_message"
}
```

## Getting Started

### Prerequisites

- Go 1.22 or later
- Protocol Buffers compiler
- gRPC tools

### Installation

```bash
# Clone repository
git clone https://github.com/yourusername/tss-demo.git
cd tss-demo

# Generate protocol buffers
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/tss.proto

# Build project
go build -o tss-demo server/main.go
```

### Running

```bash
# Start demo with 3 nodes
./tss-demo
```

Default ports:

- Node 1: gRPC 50051, HTTP 8081
- Node 2: gRPC 50052, HTTP 8082
- Node 3: gRPC 50053, HTTP 8083

## Project Structure

```
.
├── proto/          # Protocol buffer definitions
├── tss/           # Core TSS implementation
│   ├── node.go    # Node implementation with network layer
│   └── tss.go     # TSS protocol implementation
└── server/        # Main application entry point
```

## Security Considerations

This is a demonstration project. For production use, consider:

- Secure communication channels
- Proper key management
- Network security
- Input validation
- Error handling

## License

MIT License
