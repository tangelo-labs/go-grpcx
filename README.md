# gRPCx

This package provides a simple set of utilities for common use cases when 
working with gRPC protocol and protocol buffers.

## Installation

```bash
go get github.com/tangelo-labs/go-grpcx
```

## Features

### Client Dialing

Simplifies the process of dialing a gRPC backend by allowing you to specify a configuration string.
This string can include options such as TLS, connection pooling, and more.

**Example: Dialing a gRPC Backend**

```go
package main

import (
	"github.com/tangelo-labs/go-grpcx"
)

func main() {
	// Creates a pool of 10 gRPC connections to the backend.
    conn, err := grpcx.ParseClientConfigDial(`grpc://example.com:443?tls=true&pool.size=10`)
    if err != nil {
        panic(err)
    }
    
    // use conn to build a gRPC client.
    // For example, using the generated client code:
    client := NewMyServiceClient(conn)
	client.DoSomething(ctx, &MyRequest{
        Field: "value",
    })
}
```

See `client.go` and related tests for more examples and available options.

### Protobuf + HTTP 

Provides utilities to Protocol Buffers over HTTP. This is useful for scenarios where you want to
use gRPC-like features over HTTP without the full gRPC stack.

- `UnmarshalHTTPRequest` & `UnmarshalHTTPResponse`: Functions that assumes that the given request/response's
  body holds a serialized Protocol Buffers message. It will read the body, decode it as a proto.Message
  using the HTTP `content-type` header, see `ContentTypeProtoHeader` details.
- `WriteGinResponse`: A function that writes a Protocol Buffers message to an HTTP response using the Gin framework.
- `ContentTypeProtoHeader`: Is a function that returns the appropriate `Content-Type` header for a given Protocol 
   Buffers message. The format is `application/x-protobuf; messageType="<FQN>"`, where `<FQN>` is the fully qualified
   name of the protobuf message type. For example, "google.protobuf.Empty" or "google.protobuf.Timestamp".

### Error Handling

Provides a set of utilities to handle Golang errors and map them to gRPC status codes.

**Example**

```go
package main

import (
	"context"

	"github.com/tangelo-labs/go-grpcx"
	"google.golang.org/grpc/codes"
	"service/users"
)

func main() {
	var errMapper = grpcx.NewBaseErrorMapper(codes.Unknown).
		With(codes.NotFound, users.ErrNotFound).
		With(codes.AlreadyExists, users.ErrAlreadyExists).
		With(codes.InvalidArgument,
			users.ErrInvalidArgument,
			users.ErrInvalidName,
			users.ErrInvalidAge,
		)

	appErr := users.DoSomething(context.TODO())
	grpcErr := errMapper.Map(appErr)
	
	// Use grpcErr in your gRPC handler to return the appropriate status code.
}
```

### gRPC Interceptors

Provides a set of gRPC interceptors and utilities for common use cases such as:

- Panic recovery.
- Interceptors chaining.
- Metadata (http headers) propagation.

See package `interception` for more details and examples.
