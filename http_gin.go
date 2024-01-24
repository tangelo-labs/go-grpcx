package grpcx

import (
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

// WriteGinResponse writes the given proto.Message as the response body of the
// given gin.Context, using the given HTTP status code.
//
// This function will automatically set the "content-type" header of the response
// to the correct value for the given proto.Message.
func WriteGinResponse(ctx *gin.Context, code int, message proto.Message) error {
	bs, err := proto.Marshal(message)
	if err != nil {
		return err
	}

	ctx.Data(code, ContentTypeProtoHeader(message), bs)

	return nil
}
