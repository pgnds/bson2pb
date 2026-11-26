//go:generate protoc --go_out=. --go_opt=paths=source_relative testpb/test.proto
package bson2pb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type shape uint8

const (
	shapeScalar shape = iota
	shapeMessage
	shapeList
	shapeMap
)

var errFloatOverflow = errors.New("float overflow")
var errNegativeUnsigned = errors.New("negative unsigned")

var plans sync.Map
var bufPool = sync.Pool{
	New: func() any { b := make([]byte, 0, 512); return &b },
}

type msgPlan struct {
	byName map[string]*fieldPlan
}

type fieldPlan struct {
	fd    protoreflect.FieldDescriptor
	msg   *msgPlan
	shape shape
}

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

	bp := bufPool.Get().(*[]byte)
	buf := (*bp)[:0]
	defer func() {
		if cap(buf) <= 64<<10 {
			*bp = buf[:0]
			bufPool.Put(bp)
		}
	}()

	buf, err = appendDocument(buf, planFor(m.Descriptor()), rest)
	if err != nil {
		return nil, fmt.Errorf("bson2pb: %w", err)
	}
	if truncated {
		return nil, fmt.Errorf("bson2pb: malformed document: %w", errTruncated)
	}
	if len(buf) == 0 {
		return msg, nil
	}

	err = proto.UnmarshalOptions{Merge: true}.Unmarshal(buf, msg)
	if err != nil {
		return nil, fmt.Errorf("bson2pb: %w", err)
	}

	return msg, nil
}

// appendDocument encodes the BSON document body rest (the elements after the
// length prefix, up to and including the 0x00 terminator) as protobuf wire
// bytes for message described by the plan mp and appends them to the buffer.
// Keys that don't match a field of md are skipped. It returns the extended buffer.
func appendDocument(buffer []byte, mp *msgPlan, rest []byte) ([]byte, error) {
	var typ byte
	var err error
	var key, val []byte

	for {
		if len(rest) == 0 {
			return buffer, fmt.Errorf("malformed document: %w", errTruncated)
		}
		if rest[0] == 0x00 {
			if len(rest) != 1 {
				return buffer, fmt.Errorf("malformed document: %w", errInvalidLength)
			}

			return buffer, nil
		}

		typ, key, rest, err = readHeader(rest)
		if err != nil {
			return buffer, fmt.Errorf("malformed element: %w", err)
		}

		val, rest, err = readValue(typ, rest)
		if err != nil {
			return buffer, fmt.Errorf("field %q: malformed value: %w", key, err)
		}

		fp, ok := mp.byName[string(key)]
		if !ok {
			continue
		}

		fd := fp.fd
		switch fp.shape {
		case shapeList:
			buffer, err = appendList(buffer, fp, typ, val)
		case shapeMap:
			buffer, err = appendMap(buffer, fd, typ, val)
		case shapeMessage:
			buffer, err = appendMessage(buffer, fp, typ, val)
		default:
			buffer, err = appendValue(buffer, fd, typ, val)
		}

		if err != nil {
			return buffer, fmt.Errorf("field %q: %w", key, err)
		}
	}
}

// appendMessage encodes the BSON sub-document val as a length-delimited protobuf
// message for field fd and appends it to the buffer. The child is written
// straight into the buffer and its length inserted afterwards, so no separate
// child buffer is allocated. Unlike scalar zero values, an empty sub-document is
// still written, since message fields always have presence. It returns the extended
// buffer.
func appendMessage(buffer []byte, fp *fieldPlan, typ byte, val []byte) ([]byte, error) {
	if typ != 0x03 {
		return buffer, fmt.Errorf("malformed value: want document, got BSON type 0x%02x", typ)
	}

	fd := fp.fd
	buffer = protowire.AppendTag(buffer, fd.Number(), protowire.BytesType)
	valStart := len(buffer)

	buffer, err := appendDocument(buffer, fp.msg, val[4:])
	if err != nil {
		return buffer, err
	}

	return insertLength(buffer, valStart), nil
}

// appendList encodes the BSON array val as a repeated field fd, one tag per element.
func appendList(buffer []byte, fp *fieldPlan, typ byte, val []byte) ([]byte, error) {
	var elemTyp byte
	var err error
	var key, elemVal []byte

	if typ != 0x04 {
		return buffer, fmt.Errorf("malformed value: want array, got BSON type 0x%02x", typ)
	}
	rest := val[4:]
	fd := fp.fd

	for {
		if len(rest) == 0 {
			return buffer, fmt.Errorf("malformed array: %w", errTruncated)
		}
		if rest[0] == 0x00 {
			if len(rest) != 1 {
				return buffer, fmt.Errorf("malformed array: %w", errInvalidLength)
			}

			return buffer, nil
		}

		elemTyp, key, rest, err = readHeader(rest)
		if err != nil {
			return buffer, fmt.Errorf("malformed element: %w", err)
		}

		elemVal, rest, err = readValue(elemTyp, rest)
		if err != nil {
			return buffer, fmt.Errorf("element %s: malformed value: %w", key, err)
		}

		if fd.Kind() == protoreflect.MessageKind {
			buffer, err = appendMessage(buffer, fp, elemTyp, elemVal)
		} else {
			buffer = protowire.AppendTag(buffer, fd.Number(), wireType(fd.Kind()))
			buffer, err = encodeValue(buffer, fd, elemTyp, elemVal)
		}

		if err != nil {
			return buffer, fmt.Errorf("element %s: %w", key, err)
		}
	}
}

func appendMap(buffer []byte, fd protoreflect.FieldDescriptor, typ byte, val []byte) ([]byte, error) {
	var elemTyp byte
	var err error
	var key, elemVal []byte

	if typ != 0x03 {
		return buffer, fmt.Errorf("malformed value: want map, got BSON type 0x%02x", typ)
	}
	rest := val[4:]

	for {
		if len(rest) == 0 {
			return buffer, fmt.Errorf("malformed map: %w", errTruncated)
		}
		if rest[0] == 0x00 {
			if len(rest) != 1 {
				return buffer, fmt.Errorf("malformed map: %w", errInvalidLength)
			}

			return buffer, nil
		}

		elemTyp, key, rest, err = readHeader(rest)
		if err != nil {
			return buffer, fmt.Errorf("malformed element: %w", err)
		}

		elemVal, rest, err = readValue(elemTyp, rest)
		if err != nil {
			return buffer, fmt.Errorf("element %q: malformed value: %w", key, err)
		}

		buffer = protowire.AppendTag(buffer, fd.Number(), protowire.BytesType)
		entryStart := len(buffer)
		buffer, err = appendMapKey(buffer, fd.MapKey(), key)
		if err != nil {
			return buffer, fmt.Errorf("element %q: %w", key, err)
		}

		buffer, err = appendValue(buffer, fd.MapValue(), elemTyp, elemVal)
		if err != nil {
			return buffer, fmt.Errorf("element %q: %w", key, err)
		}

		buffer = insertLength(buffer, entryStart)
	}
}

func appendMapKey(buffer []byte, kd protoreflect.FieldDescriptor, key []byte) ([]byte, error) {
	buffer = protowire.AppendTag(buffer, kd.Number(), wireType(kd.Kind()))

	switch kd.Kind() {
	case protoreflect.StringKind:
		return protowire.AppendBytes(buffer, key), nil
	case protoreflect.BoolKind:
		switch string(key) {
		case "true":
			return protowire.AppendVarint(buffer, 1), nil
		case "false":
			return protowire.AppendVarint(buffer, 0), nil
		default:
			return buffer, fmt.Errorf("invalid bool key %q", key)
		}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		i, err := strconv.ParseInt(string(key), 10, 32)
		if err != nil {
			return buffer, fmt.Errorf("invalid %s key: %w", kd.Kind(), err)
		}
		return appendInt(buffer, kd.Kind(), i), nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		i, err := strconv.ParseInt(string(key), 10, 64)
		if err != nil {
			return buffer, fmt.Errorf("invalid %s key: %w", kd.Kind(), err)
		}
		return appendInt(buffer, kd.Kind(), i), nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		u, err := strconv.ParseUint(string(key), 10, 32)
		if err != nil {
			return buffer, fmt.Errorf("invalid %s key: %w", kd.Kind(), err)
		}
		return appendInt(buffer, kd.Kind(), int64(u)), nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		u, err := strconv.ParseUint(string(key), 10, 64)
		if err != nil {
			return buffer, fmt.Errorf("invalid %s key: %w", kd.Kind(), err)
		}
		return appendInt(buffer, kd.Kind(), int64(u)), nil
	default:
		return buffer, fmt.Errorf("map key kind %s not supported", kd.Kind())
	}
}

// appendValue encodes the BSON value val (of BSON type typ) as protobuf wire
// bytes for field fd and appends them to the buffer. Zero values of fields without
// presence are skipped. It returns the extended buffer.
func appendValue(buffer []byte, fd protoreflect.FieldDescriptor, typ byte, val []byte) ([]byte, error) {
	start := len(buffer)
	buffer = protowire.AppendTag(buffer, fd.Number(), wireType(fd.Kind()))
	valStart := len(buffer)
	buffer, err := encodeValue(buffer, fd, typ, val)
	if err != nil {
		return buffer, err
	}
	if !fd.HasPresence() && allZero(buffer[valStart:]) {
		return buffer[:start], nil
	}

	return buffer, nil
}

// encodeValue appends the wire encoding of val for fd, with no tag and no zero-skip
func encodeValue(buffer []byte, fd protoreflect.FieldDescriptor, typ byte, val []byte) ([]byte, error) {
	switch fd.Kind() {
	case protoreflect.StringKind:
		if typ != 0x02 {
			return buffer, fmt.Errorf("malformed value: want string, got BSON type 0x%02x", typ)
		}
		s, err := readStringBytes(val)
		if err != nil {
			return buffer, err
		}
		if !utf8.Valid(s) {
			return buffer, errInvalidUTF8
		}

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
		buffer = protowire.AppendVarint(buffer, protowire.EncodeBool(b))
		return buffer, nil
	case protoreflect.Int32Kind,
		protoreflect.Sint32Kind,
		protoreflect.Sfixed32Kind,
		protoreflect.Uint32Kind,
		protoreflect.Fixed32Kind:
		if typ != 0x10 {
			return buffer, fmt.Errorf("malformed value: want int32, got BSON type 0x%02x", typ)
		}
		i, err := readInt32(val)
		if err != nil {
			return buffer, err
		}
		if (fd.Kind() == protoreflect.Uint32Kind || fd.Kind() == protoreflect.Fixed32Kind) && i < 0 {
			return buffer, errNegativeUnsigned
		}

		return appendInt(buffer, fd.Kind(), int64(i)), nil
	case protoreflect.Int64Kind,
		protoreflect.Sint64Kind,
		protoreflect.Sfixed64Kind,
		protoreflect.Uint64Kind,
		protoreflect.Fixed64Kind:
		if typ != 0x12 {
			return buffer, fmt.Errorf("malformed value: want int64, got BSON type 0x%02x", typ)
		}
		i64, err := readInt64(val)
		if err != nil {
			return buffer, err
		}
		if (fd.Kind() == protoreflect.Uint64Kind || fd.Kind() == protoreflect.Fixed64Kind) && i64 < 0 {
			return buffer, errNegativeUnsigned
		}

		return appendInt(buffer, fd.Kind(), i64), nil
	case protoreflect.DoubleKind, protoreflect.FloatKind:
		if typ != 0x01 {
			return buffer, fmt.Errorf("malformed value: want double, got BSON type 0x%02x", typ)
		}
		f, err := readDouble(val)
		if err != nil {
			return buffer, err
		}

		switch fd.Kind() {
		case protoreflect.DoubleKind:
			fbits := math.Float64bits(f)
			buffer = protowire.AppendFixed64(buffer, fbits)
		default:
			f32 := float32(f)
			if math.IsInf(float64(f32), 0) && !math.IsInf(f, 0) {
				return buffer, errFloatOverflow
			}
			fbits := math.Float32bits(f32)
			buffer = protowire.AppendFixed32(buffer, fbits)
		}
		return buffer, nil
	case protoreflect.BytesKind:
		if typ != 0x05 {
			return buffer, fmt.Errorf("malformed value: want binary, got BSON type 0x%02x", typ)
		}
		b, err := readBinary(val)
		if err != nil {
			return buffer, err
		}
		buffer = protowire.AppendBytes(buffer, b)
		return buffer, nil
	case protoreflect.EnumKind:
		var i int32

		switch typ {
		case 0x10:
			var err error
			i, err = readInt32(val)
			if err != nil {
				return buffer, err
			}
		case 0x02:
			s, err := readStringBytes(val)
			if err != nil {
				return buffer, err
			}
			e := fd.Enum().Values().ByName(protoreflect.Name(s))
			if e == nil {
				return buffer, fmt.Errorf("unknown enum name: %q", s)
			}
			i = int32(e.Number())
		default:
			return buffer, fmt.Errorf("malformed value: want int32 or string, got BSON type 0x%02x", typ)
		}

		buffer = protowire.AppendVarint(buffer, uint64(int64(i)))
		return buffer, nil
	default:
		return buffer, fmt.Errorf("unsupported field kind %s", fd.Kind())
	}
}

func planFor(md protoreflect.MessageDescriptor) *msgPlan {
	v, ok := plans.Load(md)
	if ok {
		return v.(*msgPlan)
	}

	seen := make(map[protoreflect.MessageDescriptor]*msgPlan)
	p := buildPlan(md, seen)
	actual, _ := plans.LoadOrStore(md, p)
	return actual.(*msgPlan)
}

func buildPlan(
	md protoreflect.MessageDescriptor,
	seen map[protoreflect.MessageDescriptor]*msgPlan) *msgPlan {
	p, ok := seen[md]
	if ok {
		return p
	}

	p = &msgPlan{byName: make(map[string]*fieldPlan)}
	seen[md] = p
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		fp := &fieldPlan{fd: fd}

		if fd.Kind() == protoreflect.MessageKind && !fd.IsMap() {
			fp.msg = buildPlan(fd.Message(), seen)
		}

		if fd.IsList() {
			fp.shape = shapeList
		} else if fd.IsMap() {
			fp.shape = shapeMap
		} else if fd.Kind() == protoreflect.MessageKind {
			fp.shape = shapeMessage
		} else {
			fp.shape = shapeScalar
		}

		p.byName[string(fd.Name())] = fp
	}

	return p
}

func wireType(kind protoreflect.Kind) protowire.Type {
	switch kind {
	case protoreflect.BoolKind,
		protoreflect.EnumKind,
		protoreflect.Int32Kind,
		protoreflect.Sint32Kind,
		protoreflect.Uint32Kind,
		protoreflect.Int64Kind,
		protoreflect.Sint64Kind,
		protoreflect.Uint64Kind:
		return protowire.VarintType
	case protoreflect.Fixed32Kind,
		protoreflect.Sfixed32Kind,
		protoreflect.FloatKind:
		return protowire.Fixed32Type
	case protoreflect.Fixed64Kind,
		protoreflect.Sfixed64Kind,
		protoreflect.DoubleKind:
		return protowire.Fixed64Type
	case protoreflect.StringKind,
		protoreflect.BytesKind,
		protoreflect.MessageKind:
		return protowire.BytesType
	default: // protoreflect.GroupKind (legacy proto2)
		return protowire.StartGroupType
	}
}

func allZero(b []byte) bool {
	for _, byt := range b {
		if byt != 0x00 {
			return false
		}
	}
	return true
}

// insertLength turns the bytes written since pos into a length-delimited value
// in place: it shifts them right to make room for their length and writes the
// length as a varint at pos. it returns the extended buffer.
func insertLength(buffer []byte, pos int) []byte {
	n := len(buffer) - pos
	k := protowire.SizeVarint(uint64(n))

	buffer = append(buffer, make([]byte, k)...) // grow by k bytes
	copy(buffer[pos+k:], buffer[pos:pos+n])     // shift the values right by k

	var length [binary.MaxVarintLen64]byte
	copy(buffer[pos:], protowire.AppendVarint(length[:0], uint64(n)))

	return buffer
}

func appendInt(buffer []byte, kind protoreflect.Kind, v int64) []byte {
	switch kind {
	case protoreflect.Sint32Kind, protoreflect.Sint64Kind:
		return protowire.AppendVarint(buffer, protowire.EncodeZigZag(v))
	case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind:
		return protowire.AppendFixed32(buffer, uint32(v))
	case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
		return protowire.AppendFixed64(buffer, uint64(v))
	default: // Int32Kind, Int64Kind, Uint32Kind, Uint64Kind
		return protowire.AppendVarint(buffer, uint64(v))
	}
}
