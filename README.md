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
    cfg, err := grpcx.ParseClientConfig(`grpc://example.com:443?tls=true&blocking=true&timeout=10s`)
    if err != nil {
        panic(err)
    }
    
    ctx := context.Background()
    
    cc, err := cfg.NewDialer().Dial(ctx)
    if err != nil {
        panic(err)
    }
    
    // ...
}
```

See `client.go` and related tests for more examples and available options.
