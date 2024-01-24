package grpcx_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tangelo-labs/go-grpcx"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	_ "google.golang.org/protobuf/types/known/emptypb" // force registration in global registry
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestUnmarshalHTTPBody(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		contentType string
		requestBody proto.Message
		want        proto.Message
		wantErr     bool
	}{
		{
			name:    "nil request -> error",
			wantErr: true,
		},
		{
			name:        "empty body but valid content-type -> empty message",
			contentType: `application/x-protobuf; messageType="google.protobuf.Empty"`,
			requestBody: nil,
			want:        nil,
			wantErr:     false,
		},
		{
			name:        "valid proto in body but missing content-type -> error",
			contentType: ``,
			requestBody: timestamppb.New(now),
			want:        nil,
			wantErr:     true,
		},
		{
			name:        "valid proto in body and content-type -> valid message",
			contentType: `application/x-protobuf; messageType="google.protobuf.Timestamp"`,
			requestBody: timestamppb.New(now),
			want:        timestamppb.New(now),
			wantErr:     false,
		},
		{
			name:        "valid proto in body and content-type with no quotes -> valid message",
			contentType: `application/x-protobuf; messageType=google.protobuf.Timestamp`,
			requestBody: timestamppb.New(now),
			want:        timestamppb.New(now),
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protoBytes, err := proto.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := &http.Request{
				Header: http.Header{
					"Content-Type": []string{tt.contentType},
				},
				Body: io.NopCloser(bytes.NewReader(protoBytes)),
			}

			got, err := grpcx.UnmarshalHTTPRequest(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalHTTPBody() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			wantJSON, err := protojson.Marshal(tt.want)
			require.NoError(t, err)

			gotJSON, err := protojson.Marshal(got)
			require.NoError(t, err)

			assert.Equalf(t, wantJSON, gotJSON, "UnmarshalHTTPBody(%s)", tt.name)
		})
	}
}

func TestUnmarshalHTTPBodyEncodingJSON(t *testing.T) {
	now := time.Now()
	payload, err := protojson.Marshal(timestamppb.New(now))
	require.NoError(t, err)

	req := &http.Request{
		Header: http.Header{
			"Content-Type": []string{
				"application/x-protobuf+json; messageType=google.protobuf.Timestamp",
			},
		},
		Body: io.NopCloser(bytes.NewReader(payload)),
	}

	got, err := grpcx.UnmarshalHTTPRequest(req)

	require.NoError(t, err)
	require.IsType(t, &timestamppb.Timestamp{}, got)
	require.EqualValues(t, now.Unix(), got.(*timestamppb.Timestamp).AsTime().Unix())
}

func TestUnmarshalHTTPBodyEncodingText(t *testing.T) {
	now := time.Now()
	payload, err := prototext.Marshal(timestamppb.New(now))
	require.NoError(t, err)

	req := &http.Request{
		Header: http.Header{
			"Content-Type": []string{
				"application/x-protobuf+text; messageType=google.protobuf.Timestamp",
			},
		},
		Body: io.NopCloser(bytes.NewReader(payload)),
	}

	got, err := grpcx.UnmarshalHTTPRequest(req)

	require.NoError(t, err)
	require.IsType(t, &timestamppb.Timestamp{}, got)
	require.EqualValues(t, now.Unix(), got.(*timestamppb.Timestamp).AsTime().Unix())
}

func TestUnmarshalHTTPBodyEncodingWire(t *testing.T) {
	now := time.Now()
	payload, err := proto.Marshal(timestamppb.New(now))
	require.NoError(t, err)

	req := &http.Request{
		Header: http.Header{
			"Content-Type": []string{
				"application/x-protobuf+wire; messageType=google.protobuf.Timestamp",
			},
		},
		Body: io.NopCloser(bytes.NewReader(payload)),
	}

	got, err := grpcx.UnmarshalHTTPRequest(req)

	require.NoError(t, err)
	require.IsType(t, &timestamppb.Timestamp{}, got)
	require.EqualValues(t, now.Unix(), got.(*timestamppb.Timestamp).AsTime().Unix())
}
