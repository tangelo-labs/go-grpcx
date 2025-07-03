package grpcx

import (
	"crypto/tls"
	"fmt"

	"github.com/tangelo-labs/go-grpcx/interception/headers"
	"github.com/tangelo-labs/go-grpcx/interception/metadata"
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

// Dial dials the backend and returns a ClientConn instance.
// If the dialer was configured with a connection pool,
// it will return a pooled connection instance.
func (d *Dialer) Dial() (ClientConn, error) {
	if len(d.cfg.PoolOptions) > 0 {
		fn := PoolDialerFunc(func() (ClientConn, error) {
			return d.dial()
		})

		return NewClientConnPool(fn, d.cfg.PoolOptions...)
	}

	return d.dial()
}

func (d *Dialer) dial() (*grpc.ClientConn, error) {
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

	if d.cfg.CorrelationKey != "" {
		unaryInterceptors = append(unaryInterceptors, metadata.UnaryClientInterceptor(d.cfg.CorrelationKey))
		streamInterceptors = append(streamInterceptors, metadata.StreamClientInterceptor(d.cfg.CorrelationKey))
	}

	if d.cfg.CausationKey != "" {
		unaryInterceptors = append(unaryInterceptors, metadata.UnaryClientInterceptor(d.cfg.CausationKey))
		streamInterceptors = append(streamInterceptors, metadata.StreamClientInterceptor(d.cfg.CausationKey))
	}

	if len(unaryInterceptors) > 0 {
		additionalOptions = append(additionalOptions, grpc.WithChainUnaryInterceptor(unaryInterceptors...))
	}

	if len(streamInterceptors) > 0 {
		additionalOptions = append(additionalOptions, grpc.WithChainStreamInterceptor(streamInterceptors...))
	}

	return grpc.NewClient(target, additionalOptions...)
}
