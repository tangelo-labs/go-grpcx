package grpcx_test

import (
	"crypto/tls"
	"reflect"
	"testing"
	"time"

	"github.com/tangelo-labs/go-grpcx"
)

func TestParseClientConfig(t *testing.T) {
	tests := []struct {
		uri     string
		want    grpcx.ClientConfig
		wantErr bool
	}{
		{
			uri: "grpc://example.com:443?tls=true",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: false,
			},
		},
		{
			uri: "grpc://example.com:443?tls=true&tls.skipVerify=true",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: false,
				TLS: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
		{
			uri: "grpc://:443?",
			want: grpcx.ClientConfig{
				Host:     "",
				Port:     443,
				Insecure: false,
			},
		},
		{
			uri: "grpc://:443?tls=true",
			want: grpcx.ClientConfig{
				Host:     "",
				Port:     443,
				Insecure: false,
			},
		},
		{
			uri: "grpc://example.com:443?tls=true&authority=example.com&userAgent=grpc-go/1.38.0&maxHeaderListSize=50&keepAlive.interval=11s&keepAlive.timeout=22s",
			want: grpcx.ClientConfig{
				Host:              "example.com",
				Port:              443,
				Authority:         "example.com",
				UserAgent:         "grpc-go/1.38.0",
				Insecure:          false,
				MaxHeaderListSize: 50,
				KeepAliveInterval: 11 * time.Second,
				KeepAliveTimeout:  22 * time.Second,
			},
		},
		{
			uri: "grpc://example.com:443?tls=false",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: true,
			},
		},
		{
			uri: "grpc://example.com:443?tls=false",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: true,
			},
		},
		{
			uri: "grpc://example.com:443?tls=false",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: true,
			},
		},
		{
			uri:     "grpc://example.com:1212?tls=xxxx",
			want:    grpcx.ClientConfig{},
			wantErr: true,
		},
		{
			uri: "grpc://example.com:333?keepAlive.interval=33ms&keepAlive.timeout=200ms",
			want: grpcx.ClientConfig{
				Host:              "example.com",
				Port:              333,
				Authority:         "",
				UserAgent:         "",
				Insecure:          false,
				MaxHeaderListSize: 0,
				KeepAliveInterval: 33 * time.Millisecond,
				KeepAliveTimeout:  200 * time.Millisecond,
			},
		},
		{
			uri:     "xxxx://example.com:5588?blocking=xxxx",
			want:    grpcx.ClientConfig{},
			wantErr: true,
		},
		{
			uri:     "grpc://example.com:5588?invalidOption=xxxx",
			want:    grpcx.ClientConfig{},
			wantErr: true,
		},
		{
			uri: "grpc://example.com:443?tls=true&headers=foo:bar&headers=apikey:abc123&headers=no-value:",
			want: grpcx.ClientConfig{
				Host: "example.com",
				Headers: map[string]string{
					"foo":      "bar",
					"apikey":   "abc123",
					"no-value": "",
				},
				Port:     443,
				Insecure: false,
			},
		},
		{
			uri: "grpc://example.com:443?tls=true&resolver.scheme=passthrough",
			want: grpcx.ClientConfig{
				Host:           "example.com",
				Port:           443,
				Insecure:       false,
				ResolverScheme: "passthrough",
			},
		},
		{
			uri: "grpc://example.com:443?tls=true&resolver.scheme=dns",
			want: grpcx.ClientConfig{
				Host:           "example.com",
				Port:           443,
				Insecure:       false,
				ResolverScheme: "dns",
			},
		},
		{
			uri: "grpc://example.com:443?tls=true&resolver.scheme=dns&correlationKey=x-correlation-id",
			want: grpcx.ClientConfig{
				Host:           "example.com",
				Port:           443,
				Insecure:       false,
				ResolverScheme: "dns",
				CorrelationKey: "x-correlation-id",
			},
		},
		{
			uri: "grpc://example.com:443?tls=true&resolver.scheme=dns&defaultServiceConfig=lbp-round_robin",
			want: grpcx.ClientConfig{
				Host:                 "example.com",
				Port:                 443,
				Insecure:             false,
				ResolverScheme:       "dns",
				DefaultServiceConfig: `{"loadBalancingPolicy":"round_robin"}`,
			},
		},
		{
			uri: "grpc://example.com:443?tls=true&resolver.scheme=dns&defaultServiceConfig=lbp-pick_first",
			want: grpcx.ClientConfig{
				Host:                 "example.com",
				Port:                 443,
				Insecure:             false,
				ResolverScheme:       "dns",
				DefaultServiceConfig: `{"loadBalancingPolicy":"pick_first"}`,
			},
		},
		{
			uri: "grpc://example.com:443?pool.size=10",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: true,
				PoolOptions: []grpcx.PoolOption{
					grpcx.WithPoolSize(10),
				},
			},
		},
		{
			uri: "grpc://example.com:443?pool.size=10&pool.connLifetime=5s",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: true,
				PoolOptions: []grpcx.PoolOption{
					grpcx.WithPoolSize(10),
					grpcx.WithPoolConnLifetime(5 * time.Second),
				},
			},
		},
		{
			uri: "grpc://example.com:443?pool.size=10&pool.connLifetime=5s&pool.jitter=2m",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: true,
				PoolOptions: []grpcx.PoolOption{
					grpcx.WithPoolSize(10),
					grpcx.WithPoolConnLifetime(5 * time.Second),
					grpcx.WithPoolJitter(2 * time.Minute),
				},
			},
		},
		{
			uri: "grpc://example.com:443?pool.size=10&pool.connLifetime=5s&pool.jitter=2m&pool.healthCheckFreq=1m",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: true,
				PoolOptions: []grpcx.PoolOption{
					grpcx.WithPoolSize(10),
					grpcx.WithPoolConnLifetime(5 * time.Second),
					grpcx.WithPoolJitter(2 * time.Minute),
					grpcx.WithPoolHealthCheckFreq(time.Minute),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got, err := grpcx.ParseClientConfig(tt.uri)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseClientConfig() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if len(tt.want.PoolOptions) == 0 {
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("ParseClientConfig() got = %v, want %v", got, tt.want)
				}
			}

			if len(tt.want.PoolOptions) > 0 {
				assertEqualPoolOptions(t, tt.want.PoolOptions, got.PoolOptions)
			}
		})
	}
}

func TestParseHostAndPort(t *testing.T) {
	tests := []struct {
		input    string
		wantHost string
		wantPort int
	}{
		{
			input:    "example.com:443",
			wantHost: "example.com",
			wantPort: 443,
		},
		{
			input:    ":1212",
			wantHost: "",
			wantPort: 1212,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotHost, gotPort := grpcx.ParseHostAndPort(tt.input)
			if gotHost != tt.wantHost {
				t.Errorf("ParseHostAndPort() gotHost = %v, want %v", gotHost, tt.wantHost)
			}

			if gotPort != tt.wantPort {
				t.Errorf("ParseHostAndPort() gotPort = %v, want %v", gotPort, tt.wantPort)
			}
		})
	}
}

func assertEqualPoolOptions(t *testing.T, expected, actual []grpcx.PoolOption) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Errorf("PoolOptions length mismatch: actual %d, expected %d", len(actual), len(expected))

		return
	}

	expectedOptions := &grpcx.PoolOptions{}
	actualOptions := &grpcx.PoolOptions{}

	for _, opt := range expected {
		opt(expectedOptions)
	}

	for _, opt := range actual {
		opt(actualOptions)
	}

	if !reflect.DeepEqual(expectedOptions, actualOptions) {
		t.Errorf("PoolOptions mismatch: actual %v, expected %v", actualOptions, expectedOptions)
	}
}
