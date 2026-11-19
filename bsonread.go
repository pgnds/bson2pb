package bson2pb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
)

var (
	errTruncated          = errors.New("truncated")
	errInvalidLength      = errors.New("invalid length")
	errUnsupportedType    = errors.New("unsupported type")
	errInvalidStringBytes = errors.New("invalid string bytes")
	errInvalidUTF8        = errors.New("invalid utf8")
	errInvalidBool        = errors.New("invalid bool")
	errInvalidInt32       = errors.New("invalid int32")
	errInvalidInt64       = errors.New("invalid int64")
	errInvalidDouble      = errors.New("invalid double")
)

// readDocHeader reads the length prefix of a BSON document
func readDocHeader(raw []byte) (length int32, rest []byte, err error) {
	if len(raw) < 4 {
		return 0, nil, errTruncated
	}

	length = int32(binary.LittleEndian.Uint32(raw[:4]))
	if length < 5 {
		return 0, nil, errInvalidLength
	}

	return length, raw[4:], nil
}

// readHeader reds the start of one element (type byte and it's key)
func readHeader(src []byte) (typ byte, key []byte, rest []byte, err error) {
	if len(src) < 1 {
		return 0, nil, nil, errTruncated
	}

	keyEnd := bytes.IndexByte(src[1:], 0)
	if keyEnd < 0 {
		return 0, nil, nil, errTruncated
	}

	typ = src[0]
	key = src[1 : keyEnd+1]
	rest = src[keyEnd+2:]

	return typ, key, rest, nil
}

// readValue splits the value of BSON type off the front of src without
// decoding it, returning the value bytes and the remainder.
func readValue(typ byte, src []byte) (val []byte, rest []byte, err error) {
	size := 0

	switch typ {
	case 0x10: // int32
		size = 4
	case 0x02, 0x0D, 0x0E: // string, JavaScript, symbol
		if len(src) < 4 {
			return nil, nil, errTruncated
		}
		strLen := int32(binary.LittleEndian.Uint32(src[:4]))
		if strLen < 1 {
			return nil, nil, errInvalidLength
		}
		if int(strLen) > len(src)-4 {
			return nil, nil, errTruncated
		}
		size = 4 + int(strLen)
	case 0x08: // bool
		size = 1
	case 0x07: // ObjectID
		size = 12
	case 0x1, 0x09, 0x11, 0x12: // double, datetime, timestamp, int64
		size = 8
	case 0x13: // decimal128
		size = 16
	case 0x0A, 0x06, 0xFF, 0x7F: // null, undefined, minkey, maxkey
		size = 0
	case 0x05: // binary
		if len(src) < 4 {
			return nil, nil, errTruncated
		}
		n := int32(binary.LittleEndian.Uint32(src[:4]))
		if n < 0 {
			return nil, nil, errInvalidLength
		}
		if int(n) > len(src)-5 {
			return nil, nil, errTruncated
		}
		size = 4 + 1 + int(n)
	case 0x03, 0x04: // document, array
		if len(src) < 4 {
			return nil, nil, errTruncated
		}
		n := int32(binary.LittleEndian.Uint32(src[:4]))
		if n < 5 {
			return nil, nil, errInvalidLength
		}
		if int(n) > len(src) {
			return nil, nil, errTruncated
		}
		size = int(n)
	default:
		return nil, nil, errUnsupportedType
	}

	if len(src) < size {
		return nil, nil, errTruncated
	}

	return src[:size], src[size:], nil
}

func readStringBytes(val []byte) ([]byte, error) {
	if len(val) < 5 {
		return nil, errInvalidStringBytes
	}
	if val[len(val)-1] != 0x00 {
		return nil, errInvalidStringBytes
	}
	return val[4 : len(val)-1], nil
}

func readBool(val []byte) (bool, error) {
	if len(val) != 1 || (val[0] != 0 && val[0] != 1) {
		return false, errInvalidBool
	}
	return val[0] == 1, nil
}

func readInt32(val []byte) (int32, error) {
	if len(val) != 4 {
		return 0, errInvalidInt32
	}

	return int32(binary.LittleEndian.Uint32(val)), nil
}

func readInt64(val []byte) (int64, error) {
	if len(val) != 8 {
		return 0, errInvalidInt64
	}

	return int64(binary.LittleEndian.Uint64(val)), nil
}

func readDouble(val []byte) (float64, error) {
	if len(val) != 8 {
		return 0.0, errInvalidDouble
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(val)), nil
}
