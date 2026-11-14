//go:generate protoc --go_out=. --go_opt=paths=source_relative testpb/test.proto
package bson2pb

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

func Convert(raw bson.Raw, msg proto.Message) error {
	return nil
}
