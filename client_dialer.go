package grpcx

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/tangelo-labs/go-grpcx/interception/headers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// Dialer is used to control the dialing process of a gRPC backend connection.
type Dialer struct {
	cfg                *ClientConfig
	unaryInterceptors  []grpc.UnaryClientInterceptor
	streamInterceptors []grpc.StreamClientInterceptor
	options            []grpc.DialOption
}

// WithOptions adds additional dial options to the dialer.
func (d *Dialer) WithOptions(options ...grpc.DialOption) *Dialer {
	d.options = append(d.options, options...)

	return d
}

// WithUnaryInterceptors adds additional unary interceptors to the dialer.
func (d *Dialer) WithUnaryInterceptors(interceptors ...grpc.UnaryClientInterceptor) *Dialer {
	d.unaryInterceptors = append(d.unaryInterceptors, interceptors...)

	return d
}

// WithStreamInterceptors adds additional stream interceptors to the dialer.
func (d *Dialer) WithStreamInterceptors(interceptors ...grpc.StreamClientInterceptor) *Dialer {
	d.streamInterceptors = append(d.streamInterceptors, interceptors...)

	return d
}

// Dial dials the backend using the given context and returns a *grpc.ClientConn
// instance. Note that the context is only used to dial the connection, it is
// not used to control the connection lifecycle.
func (d *Dialer) Dial(ctx context.Context) (*grpc.ClientConn, error) {
	scheme := ""
	if d.cfg.ResolverScheme != "" {
		scheme = fmt.Sprintf("%s:///", d.cfg.ResolverScheme)
	}

	target := fmt.Sprintf("%s%s:%d", scheme, d.cfg.Host, d.cfg.Port)
	additionalOptions := d.options
	unaryInterceptors := d.unaryInterceptors
	streamInterceptors := d.streamInterceptors

	if d.cfg.Insecure {
		additionalOptions = append(additionalOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if !d.cfg.Insecure {
		tlsCfg := &tls.Config{}
		if d.cfg.TLS != nil {
			tlsCfg = d.cfg.TLS.Clone()
		}

		cred := credentials.NewTLS(tlsCfg)
		additionalOptions = append(additionalOptions, grpc.WithTransportCredentials(cred))
	}

	if d.cfg.Blocking {
		additionalOptions = append(additionalOptions, grpc.WithBlock())

		if d.cfg.Timeout > 0 {
			c, cancel := context.WithTimeout(ctx, d.cfg.Timeout)
			defer cancel()

			ctx = c
		}
	}

	if d.cfg.Authority != "" {
		additionalOptions = append(additionalOptions, grpc.WithAuthority(d.cfg.Authority))
	}

	if d.cfg.UserAgent != "" {
		additionalOptions = append(additionalOptions, grpc.WithUserAgent(d.cfg.UserAgent))
	}

	if d.cfg.MaxHeaderListSize > 0 {
		additionalOptions = append(additionalOptions, grpc.WithMaxHeaderListSize(d.cfg.MaxHeaderListSize))
	}

	if d.cfg.DefaultServiceConfig != "" {
		additionalOptions = append(additionalOptions, grpc.WithDefaultServiceConfig(d.cfg.DefaultServiceConfig))
	}

	if d.cfg.KeepAliveTimeout > 0 || d.cfg.KeepAliveInterval > 0 {
		params := keepalive.ClientParameters{}

		if d.cfg.KeepAliveInterval > 0 {
			params.Time = d.cfg.KeepAliveInterval
		}

		if d.cfg.KeepAliveTimeout > 0 {
			params.Timeout = d.cfg.KeepAliveTimeout
		}

		additionalOptions = append(additionalOptions, grpc.WithKeepaliveParams(params))
	}

	if len(d.cfg.Headers) > 0 {
		unaryInterceptors = append(unaryInterceptors, headers.UnaryClientInterceptor(d.cfg.Headers))
		streamInterceptors = append(streamInterceptors, headers.StreamClientInterceptor(d.cfg.Headers))
	}

	if len(unaryInterceptors) > 0 {
		additionalOptions = append(additionalOptions, grpc.WithChainUnaryInterceptor(unaryInterceptors...))
	}

	if len(streamInterceptors) > 0 {
		additionalOptions = append(additionalOptions, grpc.WithChainStreamInterceptor(streamInterceptors...))
	}

	return grpc.DialContext(ctx, target, additionalOptions...)
}
