//go:generate protoc --go_out=. --go_opt=paths=source_relative testpb/test.proto
package bson2pb

import (
	"fmt"
	"sync"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var fieldMaps sync.Map

// Convert decodes raw into a new M and returns it.
func Convert[M any, PM interface {
	*M
	proto.Message
}](raw bson.Raw) (PM, error) {
	msg := PM(new(M))
	m := msg.ProtoReflect()

	length, rest, err := readDocHeader(raw)
	if err != nil {
		return nil, fmt.Errorf("bson2pb: malformed document: %w", err)
	}

	truncated := int(length) > len(raw)
	if !truncated {
		rest = raw[4:length]
	}

	var typ byte
	var key, val []byte
	buf := make([]byte, 0, len(raw))
	fm := fillMessageFields(m.Descriptor())

	for {
		if len(rest) == 0 || rest[0] == 0x00 {
			if truncated {
				return nil, fmt.Errorf("bson2pb: malformed document: %w", errTruncated)
			}
			if len(rest) != 1 {
				return nil, fmt.Errorf("bson2pb: malformed document: %w", errInvalidLength)
			}

			err = proto.UnmarshalOptions{Merge: true}.Unmarshal(buf, msg)
			if err != nil {
				return nil, fmt.Errorf("bson2pb: %w", err)
			}

			return msg, nil
		}

		typ, key, rest, err = readHeader(rest)
		if err != nil {
			return nil, fmt.Errorf("bson2pb: malformed element: %w", err)
		}

		val, rest, err = readValue(typ, rest)
		if err != nil {
			return nil, fmt.Errorf("bson2pb: field %q: malformed value: %w", key, err)
		}

		fd, ok := fm[string(key)]
		if !ok {
			continue
		}

		if fd.IsList() || fd.IsMap() {
			return nil, fmt.Errorf("bson2pb: field %q: repeated and map fields not supported yet", key)
		}

		buf, err = appendValue(buf, fd, typ, val)
		if err != nil {
			return nil, fmt.Errorf("bson2pb: field %q: %w", key, err)
		}
	}
}

// appendValue encodes the BSON value val (of BSON type typ) as protobuf wire
// bytes for field fd and appends them to the buffer. Zero values of fields without
// presence are skipped. It returns the extended buffer.
func appendValue(buffer []byte, fd protoreflect.FieldDescriptor, typ byte, val []byte) ([]byte, error) {
	switch fd.Kind() {
	case protoreflect.StringKind:
		if typ != 0x02 {
			return buffer, fmt.Errorf("malformed value: want string, got BSON type 0x%02x", typ)
		}
		s, err := readStringBytes(val)
		if err != nil {
			return buffer, err
		}
		if len(s) == 0 && !fd.HasPresence() {
			return buffer, nil
		}
		if !utf8.Valid(s) {
			return buffer, errInvalidUTF8
		}

		buffer = protowire.AppendTag(buffer, fd.Number(), protowire.BytesType)
		buffer = protowire.AppendBytes(buffer, s)
		return buffer, nil
	case protoreflect.BoolKind:
		if typ != 0x08 {
			return buffer, fmt.Errorf("malformed value: want bool, got BSON type 0x%02x", typ)
		}
		b, err := readBool(val)
		if err != nil {
			return buffer, err
		}
		if !b && !fd.HasPresence() {
			return buffer, nil // proto3 doesn't write false
		}
		buffer = protowire.AppendTag(buffer, fd.Number(), protowire.VarintType)
		buffer = protowire.AppendVarint(buffer, protowire.EncodeBool(b))
		return buffer, nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		if typ != 0x10 {
			return buffer, fmt.Errorf("malformed value: want int32, got BSON type 0x%02x", typ)
		}
		i, err := readInt32(val)
		if err != nil {
			return buffer, err
		}
		if i == 0 && !fd.HasPresence() {
			return buffer, nil
		}

		if fd.Kind() == protoreflect.Int32Kind {
			buffer = protowire.AppendTag(buffer, fd.Number(), protowire.VarintType)
			buffer = protowire.AppendVarint(buffer, uint64(int64(i)))
		} else if fd.Kind() == protoreflect.Sint32Kind {
			buffer = protowire.AppendTag(buffer, fd.Number(), protowire.VarintType)
			buffer = protowire.AppendVarint(buffer, protowire.EncodeZigZag(int64(i)))
		} else {
			buffer = protowire.AppendTag(buffer, fd.Number(), protowire.Fixed32Type)
			buffer = protowire.AppendFixed32(buffer, uint32(i))
		}
		return buffer, nil
	default:
		return buffer, fmt.Errorf("unsupported field kind %s", fd.Kind())
	}
}

func fillMessageFields(md protoreflect.MessageDescriptor) map[string]protoreflect.FieldDescriptor {
	v, ok := fieldMaps.Load(md)
	if ok {
		return v.(map[string]protoreflect.FieldDescriptor)
	}

	fields := md.Fields()
	fm := make(map[string]protoreflect.FieldDescriptor, fields.Len())

	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		fm[string(fd.Name())] = fd
	}

	actual, _ := fieldMaps.LoadOrStore(md, fm)
	return actual.(map[string]protoreflect.FieldDescriptor)
}
