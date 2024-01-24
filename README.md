# gRPCx

This package provides a simple set of utilities for common use cases when 
working with gRPC protocol and protocol buffers.

## Installation

```bash
go get github.com/tangelo-labs/go-grpcx
```

## Usage

Dialing a gRPC backend using a configuration string:

```go
package main

import (
	"context"

	"github.com/tangelo-labs/go-grpcx"
)

func main() {
	ctx := context.Background()
    conn, err := grpcx.ParseClientConfigDial(ctx, `grpc://example.com:443?tls=true&blocking=true&timeout=10s`)
    if err != nil {
        panic(err)
    }
    
    // use conn to build a gRPC client.
}
```

See `client.go` and related tests for more examples and available options.
