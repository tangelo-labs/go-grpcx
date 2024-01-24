package grpcx

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const expectedProtoContentType = "application/x-protobuf"

// UnmarshalHTTPRequest assumes that the given request has a body that is a
// protobuf message, and unmarshal it into a proto.Message of the type
// defined by the Request's header `content-type`.
//
// Content-Type header must follow the following syntax:
//
//	application/x-protobuf; messageType="<FQN>"
//
// Where <FQN> is the fully qualified name of the protobuf message type. For
// example: "google.protobuf.Empty" or "google.protobuf.Timestamp".
//
// By default, this function uses the Global proto-registry to resolve the
// message types (see "protoregistry.GlobalTypes" for further details), but
// you can also provide a custom registry to use.
//
// # Encoding
//
// By default this function assumes that the request body holds a binary
// encoded protobuf message, but you can also specify the encoding in the
// content-type header:
//
//	application/x-protobuf+<encoding>; messageType="<FQN>"
//
// Where <encoding> must be one of the following:
//
// - "json": body holds a JSON encoded protobuf message (see protojson package).
// - "text": body holds a text encoded protobuf message (see prototext package).
// - "wire": body holds a binary encoded protobuf message (see proto package).
//
// By default, if no encoding is specified, then "wire" (binary) is assumed.
//
// # Registry
//
// By default, this function uses the Global proto-registry to resolve the message
// types (see "protoregistry.GlobalTypes" for further details), but you can also
// provide a custom registry to use.
func UnmarshalHTTPRequest(req *http.Request, registry ...*protoregistry.Types) (proto.Message, error) {
	reg := protoregistry.GlobalTypes
	if len(registry) > 0 {
		reg = registry[0]
	}

	if req == nil {
		return nil, fmt.Errorf("nil request")
	}

	if req.Body == nil {
		return nil, fmt.Errorf("nil request body")
	}

	return unmarshalBody(req.Body, req.Header, reg)
}

// UnmarshalHTTPResponse same as UnmarshalHTTPRequest, but for HTTP responses.
func UnmarshalHTTPResponse(res *http.Response, registry ...*protoregistry.Types) (proto.Message, error) {
	reg := protoregistry.GlobalTypes
	if len(registry) > 0 {
		reg = registry[0]
	}

	if res == nil {
		return nil, fmt.Errorf("nil response")
	}

	if res.Body == nil {
		return nil, fmt.Errorf("nil response body")
	}

	return unmarshalBody(res.Body, res.Header, reg)
}

// ContentTypeProtoHeader computes the corresponding value for the
// "content-type" HTTP header for the given proto message.
//
// Content-Type header will follow the following syntax:
//
//	application/x-protobuf; messageType="<FQN>"
//
// Where <FQN> is the fully qualified name of the protobuf message type. For
// example: "google.protobuf.Empty" or "google.protobuf.Timestamp".
func ContentTypeProtoHeader(msg proto.Message) string {
	return fmt.Sprintf(
		`%s; messageType=%q`,
		expectedProtoContentType,
		msg.ProtoReflect().Descriptor().FullName(),
	)
}

func unmarshalBody(b io.ReadCloser, headers http.Header, registry *protoregistry.Types) (proto.Message, error) {
	body, err := io.ReadAll(b)
	if err != nil {
		return nil, err
	}

	contentType := headers.Get("Content-Type")
	if contentType == "" {
		return nil, fmt.Errorf("missing content-type header")
	}

	messageType, encoding, err := parseMessageType(contentType)
	if err != nil {
		return nil, err
	}

	mt, err := registry.FindMessageByName(protoreflect.FullName(messageType))
	if err != nil {
		return nil, fmt.Errorf("unknown proto message `%s`", messageType)
	}

	msg := mt.New().Interface()

	switch encoding {
	case "json":
		if uErr := protojson.Unmarshal(body, msg); uErr != nil {
			return nil, uErr
		}
	case "text":
		if uErr := prototext.Unmarshal(body, msg); uErr != nil {
			return nil, uErr
		}
	case "wire", "":
		if uErr := proto.Unmarshal(body, msg); uErr != nil {
			return nil, uErr
		}
	default:
		return nil, fmt.Errorf("invalid content-type, unknown encoding `%s`", encoding)
	}

	return msg, nil
}

func parseMessageType(contentType string) (messageType string, encoding string, err error) {
	mt, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", "", err
	}

	if !strings.HasPrefix(mt, "application/x-protobuf") {
		return "", "", fmt.Errorf("invalid content-type, expecting `application/x-protobuf`, but got `%s`", mt)
	}

	messageType, ok := params["messagetype"]
	if !ok {
		return "", "", fmt.Errorf("invalid content-type, missing `messageType` parameter")
	}

	encoding = ""
	if parts := strings.Split(mt, "+"); len(parts) == 2 {
		encoding = parts[1]
	}

	return messageType, encoding, nil
}
