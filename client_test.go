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
		dsn     string
		want    grpcx.ClientConfig
		wantErr bool
	}{
		{
			dsn: "grpc://example.com:443?tls=true&blocking=true&timeout=10s",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: false,
				Blocking: true,
				Timeout:  10 * time.Second,
			},
		},
		{
			dsn: "grpc://example.com:443?tls=true&tls.skipVerify=true&blocking=true&timeout=10s",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: false,
				TLS: &tls.Config{
					InsecureSkipVerify: true,
				},
				Blocking: true,
				Timeout:  10 * time.Second,
			},
		},
		{
			dsn: "grpc://:443?",
			want: grpcx.ClientConfig{
				Host:     "",
				Port:     443,
				Insecure: false,
				Blocking: true,
				Timeout:  10 * time.Second,
			},
		},
		{
			dsn: "grpc://:443?tls=true&blocking=true&timeout=10s",
			want: grpcx.ClientConfig{
				Host:     "",
				Port:     443,
				Insecure: false,
				Blocking: true,
				Timeout:  10 * time.Second,
			},
		},
		{
			dsn: "grpc://example.com:443?tls=true&blocking=true&timeout=10s&authority=example.com&userAgent=grpc-go/1.38.0&maxHeaderListSize=50&keepAlive.interval=11s&keepAlive.timeout=22s",
			want: grpcx.ClientConfig{
				Host:              "example.com",
				Port:              443,
				Authority:         "example.com",
				UserAgent:         "grpc-go/1.38.0",
				Insecure:          false,
				MaxHeaderListSize: 50,
				KeepAliveInterval: 11 * time.Second,
				KeepAliveTimeout:  22 * time.Second,
				Blocking:          true,
				Timeout:           10 * time.Second,
			},
		},
		{
			dsn: "grpc://example.com:443?tls=false&blocking=false",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: true,
				Blocking: false,
				Timeout:  10 * time.Second,
			},
		},
		{
			dsn: "grpc://example.com:443?tls=false&blocking=false&timeout=10s",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: true,
				Blocking: false,
				Timeout:  10 * time.Second,
			},
		},
		{
			dsn: "grpc://example.com:443?tls=false&blocking=false&timeout=10s&timeout=20s",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: true,
				Blocking: false,
				Timeout:  10 * time.Second,
			},
		},
		{
			dsn: "grpc://example.com:443?tls=false&blocking=false&timeout=10s&timeout=20s&timeout=30s",
			want: grpcx.ClientConfig{
				Host:     "example.com",
				Port:     443,
				Insecure: true,
				Blocking: false,
				Timeout:  10 * time.Second,
			},
		},
		{
			dsn:     "grpc://example.com:443?tls=false&blocking=false&timeout=xxxx",
			want:    grpcx.ClientConfig{},
			wantErr: true,
		},
		{
			dsn:     "grpc://example.com:1212?tls=xxxx",
			want:    grpcx.ClientConfig{},
			wantErr: true,
		},
		{
			dsn:     "grpc://example.com:5588?blocking=xxxx",
			want:    grpcx.ClientConfig{},
			wantErr: true,
		},
		{
			dsn: "grpc://example.com:333?keepAlive.interval=33ms&keepAlive.timeout=200ms",
			want: grpcx.ClientConfig{
				Host:              "example.com",
				Port:              333,
				Authority:         "",
				UserAgent:         "",
				Insecure:          false,
				MaxHeaderListSize: 0,
				KeepAliveInterval: 33 * time.Millisecond,
				KeepAliveTimeout:  200 * time.Millisecond,
				Blocking:          true,
				Timeout:           10 * time.Second,
			},
		},
		{
			dsn:     "xxxx://example.com:5588?blocking=xxxx",
			want:    grpcx.ClientConfig{},
			wantErr: true,
		},
		{
			dsn:     "grpc://example.com:5588?invalidOption=xxxx",
			want:    grpcx.ClientConfig{},
			wantErr: true,
		},
		{
			dsn: "grpc://example.com:443?tls=true&blocking=true&timeout=10s&headers=foo:bar&headers=apikey:abc123&headers=no-value:",
			want: grpcx.ClientConfig{
				Host: "example.com",
				Headers: map[string]string{
					"foo":      "bar",
					"apikey":   "abc123",
					"no-value": "",
				},
				Port:     443,
				Insecure: false,
				Blocking: true,
				Timeout:  10 * time.Second,
			},
		},
		{
			dsn: "grpc://example.com:443?tls=true&blocking=true&timeout=10s&resolver.scheme=passthrough",
			want: grpcx.ClientConfig{
				Host:           "example.com",
				Port:           443,
				Insecure:       false,
				Blocking:       true,
				Timeout:        10 * time.Second,
				ResolverScheme: "passthrough",
			},
		},
		{
			dsn: "grpc://example.com:443?tls=true&blocking=true&timeout=10s&resolver.scheme=dns",
			want: grpcx.ClientConfig{
				Host:           "example.com",
				Port:           443,
				Insecure:       false,
				Blocking:       true,
				Timeout:        10 * time.Second,
				ResolverScheme: "dns",
			},
		},
		{
			dsn: "grpc://example.com:443?tls=true&blocking=true&timeout=10s&resolver.scheme=dns&defaultServiceConfig=lbp-round_robin",
			want: grpcx.ClientConfig{
				Host:                 "example.com",
				Port:                 443,
				Insecure:             false,
				Blocking:             true,
				Timeout:              10 * time.Second,
				ResolverScheme:       "dns",
				DefaultServiceConfig: `{"loadBalancingPolicy":"round_robin"}`,
			},
		},
		{
			dsn: "grpc://example.com:443?tls=true&blocking=true&timeout=10s&resolver.scheme=dns&defaultServiceConfig=lbp-pick_first",
			want: grpcx.ClientConfig{
				Host:                 "example.com",
				Port:                 443,
				Insecure:             false,
				Blocking:             true,
				Timeout:              10 * time.Second,
				ResolverScheme:       "dns",
				DefaultServiceConfig: `{"loadBalancingPolicy":"pick_first"}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.dsn, func(t *testing.T) {
			got, err := grpcx.ParseClientConfig(tt.dsn)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseClientConfig() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseClientConfig() got = %v, want %v", got, tt.want)
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
