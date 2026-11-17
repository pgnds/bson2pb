//go:generate protoc --go_out=. --go_opt=paths=source_relative testpb/test.proto
package bson2pb

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Convert decodes the BSON document raw into msg.
func Convert(raw bson.Raw, msg proto.Message) error {
	m := msg.ProtoReflect()
	fields := m.Descriptor().Fields()

	length, rest, err := readDocHeader(raw)
	if err != nil {
		return fmt.Errorf("bson2pb: malformed document: %w", err)
	}

	truncated := int(length) > len(raw)
	if !truncated {
		rest = raw[4:length]
	}

	var typ byte
	var key, val []byte

	for {
		if len(rest) == 0 || rest[0] == 0x00 {
			if truncated {
				return fmt.Errorf("bson2pb: malformed document: %w", errTruncated)
			}
			if len(rest) != 1 {
				return fmt.Errorf("bson2pb: malformed document: %w", errInvalidLength)
			}
			return nil
		}

		typ, key, rest, err = readHeader(rest)
		if err != nil {
			return fmt.Errorf("bson2pb: malformed element: %w", err)
		}

		val, rest, err = readValue(typ, rest)
		if err != nil {
			return fmt.Errorf("bson2pb: field %q: malformed value: %w", key, err)
		}

		fd := fields.ByName(protoreflect.Name(key))
		if fd == nil {
			continue
		}

		if fd.IsList() || fd.IsMap() {
			return fmt.Errorf("bson2pb: field %q: repeated and map fields not supported yet", key)
		}

		v, err := decodeValue(fd, typ, val)
		if err != nil {
			return fmt.Errorf("bson2pb: field %q: %w", key, err)
		}

		m.Set(fd, v)
	}
}

// decodeValue converts one BSON value into a protoreflect.Value for field fd
func decodeValue(fd protoreflect.FieldDescriptor, typ byte, val []byte) (protoreflect.Value, error) {
	switch fd.Kind() {
	case protoreflect.StringKind:
		if typ != 0x02 {
			return protoreflect.Value{}, fmt.Errorf("malformed value: want string, got BSON type 0x%02x", typ)
		}
		s, err := readString(val)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfString(s), nil
	case protoreflect.BoolKind:
		if typ != 0x08 {
			return protoreflect.Value{}, fmt.Errorf("malformed value: want bool, got BSON type 0x%02x", typ)
		}
		b, err := readBool(val)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfBool(b), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		if typ != 0x10 {
			return protoreflect.Value{}, fmt.Errorf("malformed value: want int32, got BSON type 0x%02x", typ)
		}
		i, err := readInt32(val)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfInt32(i), nil
	default:
		return protoreflect.Value{}, fmt.Errorf("unsupported field kind %s", fd.Kind())
	}
}
