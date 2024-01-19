package grpcx

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
)

// ErrInvalidClientConnectionString raised when failing to parse a connection
// string using the ParseClientConfig function.
var ErrInvalidClientConnectionString = errors.New("invalid client connection string")

// parserFunc is a function that parses a query-string and alters the given
// ClientConfig instance.
type parserFunc func(config *ClientConfig, firstValue string, allValues ...string) error

// clientOptionsParsers list of acceptable options for a client connection
// string and their respective parsers.
var clientOptionsParsers = map[string]parserFunc{
	"tls": func(config *ClientConfig, tls string, _ ...string) error {
		b, err := strconv.ParseBool(tls)
		if err != nil {
			return fmt.Errorf("%w: invalid tls value, details = %w", ErrInvalidClientConnectionString, err)
		}

		config.Insecure = !b

		return nil
	},
	"tls.skipVerify": func(config *ClientConfig, tlsSkipVerify string, _ ...string) error {
		b, err := strconv.ParseBool(tlsSkipVerify)
		if err != nil {
			return fmt.Errorf("%w: invalid tls.skipVerify value, details = %w", ErrInvalidClientConnectionString, err)
		}

		if config.TLS == nil {
			config.TLS = &tls.Config{}
		}

		config.TLS.InsecureSkipVerify = b

		return nil

	},
	"tls.rootCAs": func(config *ClientConfig, tlsRootCAs string, _ ...string) error {
		file, err := os.Open(tlsRootCAs)
		if err != nil {
			return fmt.Errorf("%w: could not open tls.rootCAs path `%s`, details = %w", ErrInvalidClientConnectionString, tlsRootCAs, err)
		}

		certBytes, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("%w: could not read tls.rootCAs file `%s`, details = %w", ErrInvalidClientConnectionString, tlsRootCAs, err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(certBytes) {
			return fmt.Errorf("failed to append tls.rootCAs certificates `%s`", tlsRootCAs)
		}

		if config.TLS == nil {
			config.TLS = &tls.Config{}
		}

		config.TLS.RootCAs = pool

		return nil
	},
	"tls.minVersion": func(config *ClientConfig, tlsMinVersion string, _ ...string) error {
		ver, err := strconv.ParseUint(tlsMinVersion, 10, 16)
		if err != nil {
			return fmt.Errorf("%w: invalid tls.minVersion value, details = %w", ErrInvalidClientConnectionString, err)
		}

		if config.TLS == nil {
			config.TLS = &tls.Config{}
		}

		config.TLS.MinVersion = uint16(ver)

		return nil

	},
	"tls.maxVersion": func(config *ClientConfig, tlsMaxVersion string, _ ...string) error {
		ver, err := strconv.ParseUint(tlsMaxVersion, 10, 16)
		if err != nil {
			return fmt.Errorf("%w: invalid tls.maxVersion value, details = %w", ErrInvalidClientConnectionString, err)
		}

		if config.TLS == nil {
			config.TLS = &tls.Config{}
		}

		config.TLS.MaxVersion = uint16(ver)

		return nil

	},
	"tls.serverName": func(config *ClientConfig, tlsServerName string, _ ...string) error {
		if tlsServerName == "" {
			return fmt.Errorf("%w: tls.ServerName cannot be empty if provided", ErrInvalidClientConnectionString)
		}

		if config.TLS == nil {
			config.TLS = &tls.Config{}
		}

		config.TLS.ServerName = tlsServerName

		return nil

	},
	"blocking": func(config *ClientConfig, blocking string, _ ...string) error {
		b, err := strconv.ParseBool(blocking)
		if err != nil {
			return fmt.Errorf("%w: invalid blocking value, details = %w", ErrInvalidClientConnectionString, err)
		}

		config.Blocking = b

		return nil
	},
	"timeout": func(config *ClientConfig, timeout string, _ ...string) error {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return fmt.Errorf("%w: invalid timeout value, details = %w", ErrInvalidClientConnectionString, err)
		}

		config.Timeout = d

		return nil
	},
	"authority": func(config *ClientConfig, authority string, _ ...string) error {
		if authority == "" {
			return fmt.Errorf("%w: authority cannot be empty", ErrInvalidClientConnectionString)
		}

		config.Authority = authority

		return nil
	},
	"userAgent": func(config *ClientConfig, userAgent string, _ ...string) error {
		if userAgent == "" {
			return fmt.Errorf("%w: userAgent cannot be empty", ErrInvalidClientConnectionString)
		}

		config.UserAgent = userAgent

		return nil
	},
	"maxHeaderListSize": func(config *ClientConfig, maxHeaderListSize string, _ ...string) error {
		size, err := strconv.ParseUint(maxHeaderListSize, 10, 32)
		if err != nil {
			return fmt.Errorf("%w: invalid maxHeaderListSize value, details = %w", ErrInvalidClientConnectionString, err)
		}

		config.MaxHeaderListSize = uint32(size)

		return nil
	},
	"keepAlive.interval": func(config *ClientConfig, keepAliveInterval string, _ ...string) error {
		d, err := time.ParseDuration(keepAliveInterval)
		if err != nil {
			return fmt.Errorf("%w: invalid keepAliveInterval value, details = %w", ErrInvalidClientConnectionString, err)
		}

		config.KeepAliveInterval = d

		return nil
	},
	"keepAlive.timeout": func(config *ClientConfig, keepAliveTimeout string, _ ...string) error {
		d, err := time.ParseDuration(keepAliveTimeout)
		if err != nil {
			return fmt.Errorf("%w: invalid keepAliveTimeout value, details = %w", ErrInvalidClientConnectionString, err)
		}

		config.KeepAliveTimeout = d

		return nil
	},
	"headers": func(config *ClientConfig, _ string, tuples ...string) error {
		if len(tuples) == 0 {
			return fmt.Errorf("%w: headers cannot be empty", ErrInvalidClientConnectionString)
		}

		config.Headers = make(map[string]string)

		for i := range tuples {
			kv := strings.Split(tuples[i], ":")
			if len(kv) != 2 {
				return fmt.Errorf("%w: invalid header, must be in the form key:value, got `%s`", ErrInvalidClientConnectionString, tuples[i])
			}

			config.Headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}

		return nil
	},
	"resolver.scheme": func(config *ClientConfig, scheme string, _ ...string) error {
		if scheme != "passthrough" && scheme != "dns" && scheme != "unix" {
			return fmt.Errorf("%w: invalid resolver.scheme value, expecting `passthrough`, `dns` or `unix`, got `%s`", ErrInvalidClientConnectionString, scheme)
		}

		config.ResolverScheme = scheme

		return nil
	},
	"defaultServiceConfig": func(config *ClientConfig, sc string, _ ...string) error {
		switch sc {
		case "lbp-pick_first":
			sc = `{"loadBalancingPolicy":"pick_first"}`
		case "lbp-round_robin":
			sc = `{"loadBalancingPolicy":"round_robin"}`
		}

		if !json.Valid([]byte(sc)) {
			return fmt.Errorf("%w: invalid JSON for defaultServiceConfig value, got `%s`", ErrInvalidClientConnectionString, sc)
		}

		config.DefaultServiceConfig = sc

		return nil
	},
	"pool": func(config *ClientConfig, sc string, _ ...string) error {
		return nil
	},
}

// ClientConfig captures the configuration details for a gRPC client connection.
type ClientConfig struct {
	// Host a valid hostname, e.g. `example.com`. If not provided, an empy
	// string will be used which is interpreted as `localhost` when dialing.
	Host string

	// Port a valid TCP number, in the range 1-65535. Required.
	Port int

	// Insecure makes the client to use an insecure channel.
	Insecure bool

	// TLS captures TLS/SSL configuration details when Insecure is false.
	TLS *tls.Config

	// Authority specifies the value to be used as the `:authority`
	// pseudo-header and as the server name in authentication handshake.
	Authority string

	// UserAgent specifies a user agent string for all the RPCs.
	UserAgent string

	// Headers specifies a set of HTTP headers to be sent with each RPC call.
	Headers map[string]string

	// MaxHeaderListSize specifies the maximum (uncompressed) size of header
	// list that the client is prepared to accept. Default is 0, which means
	// unlimited.
	MaxHeaderListSize uint32

	// KeepAliveInterval after a duration of KeepAliveInterval, if the client
	// doesn't see any activity, it pings the server to see if the transport is
	// still alive. If set below 10s, a minimum value of 10s will be used
	// instead. The current default value is infinity.
	KeepAliveInterval time.Duration

	// KeepAliveTimeout after having pinged for keepalive check, the client
	// waits for a duration of KeepAliveTimeout and if no activity is seen even
	// after that the connection is closed. The current default value is 20
	// seconds.
	KeepAliveTimeout time.Duration

	// Blocking makes the client to block when connecting to the server.
	Blocking bool

	// Timeout is the timeout for the connection. This option is only valid when
	// using a blocking connection.
	Timeout time.Duration

	// Resolver is the name of the resolver to use. Default is `passthrough` if
	// not provided.
	//
	// Available resolvers values are:
	//
	// - `passthrough`
	// - `dns`
	// - `unix`
	ResolverScheme string

	// DefaultServiceConfig is a JSON representation of the default service
	// config.
	//
	// The following shorthand values are also accepted:
	//
	// - `lbp-pick_first` is equivalent to `{"loadBalancingPolicy":"pick_first"}`
	// - `lbp-round_robin` is equivalent to `{"loadBalancingPolicy":"round_robin"}`
	//
	// For more information about service configs, see:
	// https://github.com/grpc/grpc/blob/master/doc/service_config.md
	DefaultServiceConfig string
}

// NewDialer builds a Dialer object that can be tweaked before dialing.
func (cfg ClientConfig) NewDialer() *Dialer {
	return &Dialer{
		cfg:                &cfg,
		unaryInterceptors:  make([]grpc.UnaryClientInterceptor, 0),
		streamInterceptors: make([]grpc.StreamClientInterceptor, 0),
		options:            make([]grpc.DialOption, 0),
	}
}

// ParseClientConfig parses a ClientConfig from a string.
//
// The string must be in URI-like format:
//
//	grpc://[HOST]:<PORT>[?OPTIONS]
//
// You can specify options for the connection in a URI-like string by appending
// `?attribute=value`. The following options are available:
//
//   - tls (Default true): Whether o not to use TLS. If set to false, an insecure
//     connection will be used.
//   - tls.skipVerify (Default false): Whether to skip TLS verification or not.
//     This option is only valid when using TLS.
//   - tls.rootCAs (Default host): Path to a file containing a list of trusted
//     root CAs. If not defined, host's root CA set will be used.
//   - tls.maxVersion: The maximum TLS version that is acceptable. If zero, the
//     maximum version supported by the grpc package is used, which is currently
//     TLS 1.3.
//   - tls.serverName (Default none): Is used to verify the hostname on the
//     returned certificates unless tls.SkipVerify is given.
//   - blocking (Default true): if set to false, the client will not block when
//     connecting to the server.
//   - timeout (Default 10s): the timeout for the connection. This option is only
//     valid when using a blocking connection.
//   - authority: specifies the value to be used as the `:authority` pseudo-header
//     and as the server name in authentication handshake.
//   - userAgent: specifies a user agent string for all the RPCs.
//   - maxHeaderListSize (Default 0, unlimited): specifies the maximum
//     (uncompressed) size of header list that the client is prepared to accept.
//   - keepAlive.interval (Default 0, infinity): after a duration of
//     `keepAliveInterval`, if the client doesn't see any activity, it pings the
//     server to see if the transport is still alive. If set below 10s, a minimum
//     value of 10s will be used instead.
//   - keepAlive.timeout (Default 20s): after having pinged for keepalive check,
//     the client waits for a duration of `keepAliveTimeout` and if no activity is
//     seen even after that the connection is closed.
//   - headers: a list of header tuples to be sent to the server. Each header must
//     be in the form `key:value` (value may be empty, e.g. `no-value:`), to
//     indicate more than one header simply repeat the option. Example:
//     `headers=foo:bar&headers=bar:baz`.
//
// Duration strings are a possibly signed sequence of decimal numbers, each with
// optional fraction and a unit suffix, such as "300ms", "-1.5h" or "2h45m".
// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
//
// Examples:
//
//	grpc://example.com:8080
//	grpc://:8080?tls=false
//	grpc://:8080?blocking=false&timeout=5s
//	grpc://example.com:8080?headers=foo:bar&headers=bar:baz
func ParseClientConfig(dsn string) (ClientConfig, error) {
	config := &ClientConfig{
		Insecure: false,
		Blocking: true,
		Timeout:  10 * time.Second,
	}

	if dsn == "" {
		return ClientConfig{}, fmt.Errorf("%w: empty dsn", ErrInvalidClientConnectionString)
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return ClientConfig{}, fmt.Errorf("%w: invalid dsn, details = %w", ErrInvalidClientConnectionString, err)
	}

	if u.Scheme != "grpc" {
		return ClientConfig{}, fmt.Errorf("%w: invalid scheme `%s`, expecting `grpc`", ErrInvalidClientConnectionString, u.Scheme)
	}

	if u.Host == "" {
		return ClientConfig{}, fmt.Errorf("%w: host cannot be empty", ErrInvalidClientConnectionString)
	}

	config.Host = u.Hostname()

	if u.Port() == "" {
		return ClientConfig{}, fmt.Errorf("%w: port cannot be empty", ErrInvalidClientConnectionString)
	}

	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return ClientConfig{}, fmt.Errorf("%w: port is not a number, details = %w", ErrInvalidClientConnectionString, err)
	}

	if port < 0 || port > 65535 {
		return ClientConfig{}, fmt.Errorf("%w: port is out of range [1, 65535]", ErrInvalidClientConnectionString)
	}

	config.Port = port
	q := u.Query()

	for k := range q {
		if _, ok := clientOptionsParsers[k]; !ok {
			return ClientConfig{}, fmt.Errorf("%w: unknown grpc client option `%s`", ErrInvalidClientConnectionString, k)
		}

		if pErr := clientOptionsParsers[k](config, q.Get(k), q[k]...); pErr != nil {
			return ClientConfig{}, pErr
		}
	}

	return *config, nil
}

// ParseHostAndPort parses a host and port from a string given in the format:
// `host:port`. If given string is invalid, zero values are returned.
func ParseHostAndPort(s string) (host string, port int) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return "", 0
	}

	host = parts[0]
	port, err := strconv.Atoi(parts[1])

	if err != nil {
		return "", 0
	}

	return host, port
}
