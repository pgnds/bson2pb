package bson2pb_test

import (
	"math"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"

	bson2pb "github.com/pgnds/bson2pb"
	"github.com/pgnds/bson2pb/testpb"
)

func doc(d bson.D) bson.Raw {
	b, err := bson.Marshal(d)
	if err != nil {
		panic(err)
	}
	return b
}

func badBool() bson.Raw {
	b := doc(bson.D{{Key: "be_kind", Value: true}})
	b[len(b)-2] = 0x02
	return b
}

func TestConvert(t *testing.T) {
	cases := []struct {
		name    string
		raw     bson.Raw
		want    *testpb.EverythingEverywhereAllAtOnce
		wantErr string
	}{
		{
			name: "empty document",
			raw:  doc(bson.D{}),
			want: &testpb.EverythingEverywhereAllAtOnce{},
		},
		{
			name: "simple string",
			raw:  doc(bson.D{{Key: "alter_ego", Value: "Alice"}}),
			want: &testpb.EverythingEverywhereAllAtOnce{AlterEgo: "Alice"},
		},
		{
			name: "unknown key skipped",
			raw:  doc(bson.D{{Key: "alter_ego", Value: "Alice"}, {Key: "extra", Value: 1}}),
			want: &testpb.EverythingEverywhereAllAtOnce{AlterEgo: "Alice"},
		},
		{
			name: "empty string",
			raw:  doc(bson.D{{Key: "alter_ego", Value: ""}}),
			want: &testpb.EverythingEverywhereAllAtOnce{},
		},
		{
			name: "wrong case",
			raw:  doc(bson.D{{Key: "Alter_Ego", Value: "Alice"}}),
			want: &testpb.EverythingEverywhereAllAtOnce{},
		},
		{
			name:    "wrong type",
			raw:     doc(bson.D{{Key: "alter_ego", Value: 42}}),
			wantErr: "malformed",
		},
		{
			name:    "truncated bytes",
			raw:     doc(bson.D{{Key: "alter_ego", Value: "Alice"}})[:20],
			wantErr: "alter_ego",
		},
		{
			name:    "invalid utf8",
			raw:     doc(bson.D{{Key: "alter_ego", Value: "Al\xffce"}}),
			wantErr: "invalid utf8",
		},
		{
			name: "simple bool",
			raw:  doc(bson.D{{Key: "be_kind", Value: true}}),
			want: &testpb.EverythingEverywhereAllAtOnce{BeKind: true},
		},
		{
			name: "simple bool - false",
			raw:  doc(bson.D{{Key: "be_kind", Value: false}}),
			want: &testpb.EverythingEverywhereAllAtOnce{BeKind: false},
		},
		{
			name: "two fields",
			raw: doc(bson.D{
				{Key: "alter_ego", Value: "Alice"},
				{Key: "be_kind", Value: true},
			}),
			want: &testpb.EverythingEverywhereAllAtOnce{AlterEgo: "Alice", BeKind: true},
		},
		{
			name:    "bool - wrong type",
			raw:     doc(bson.D{{Key: "be_kind", Value: "yes"}}),
			wantErr: "malformed",
		},
		{
			name:    "bool into string field",
			raw:     doc(bson.D{{Key: "alter_ego", Value: true}}),
			wantErr: "malformed",
		},
		{
			name:    "bool invalid byte",
			raw:     badBool(),
			wantErr: "invalid bool",
		},
		{
			name:    "bool truncated",
			raw:     doc(bson.D{{Key: "be_kind", Value: true}})[:13],
			wantErr: "be_kind",
		},
		{
			name: "simple int32",
			raw:  doc(bson.D{{Key: "hot_dog_fingers", Value: 32}}),
			want: &testpb.EverythingEverywhereAllAtOnce{HotDogFingers: 32},
		},
		{
			name:    "int32 - wrong type",
			raw:     doc(bson.D{{Key: "hot_dog_fingers", Value: "32"}}),
			wantErr: "malformed",
		},
		{
			name:    "int32 - wrong type 2",
			raw:     doc(bson.D{{Key: "hot_dog_fingers", Value: int64(32)}}),
			wantErr: "malformed",
		},
		{
			name: "int32 - zero",
			raw:  doc(bson.D{{Key: "hot_dog_fingers", Value: 0}}),
			want: &testpb.EverythingEverywhereAllAtOnce{HotDogFingers: 0},
		},
		{
			name: "int32 - negative",
			raw:  doc(bson.D{{Key: "hot_dog_fingers", Value: -1}}),
			want: &testpb.EverythingEverywhereAllAtOnce{HotDogFingers: -1},
		},
		{
			name: "int32 - min",
			raw:  doc(bson.D{{Key: "hot_dog_fingers", Value: math.MinInt32}}),
			want: &testpb.EverythingEverywhereAllAtOnce{HotDogFingers: math.MinInt32},
		},
		{
			name: "int32 - max",
			raw:  doc(bson.D{{Key: "hot_dog_fingers", Value: math.MaxInt32}}),
			want: &testpb.EverythingEverywhereAllAtOnce{HotDogFingers: math.MaxInt32},
		},
		{
			name:    "int32 truncated",
			raw:     doc(bson.D{{Key: "hot_dog_fingers", Value: 123}})[:24],
			wantErr: "malformed value: truncated",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := bson2pb.Convert[testpb.EverythingEverywhereAllAtOnce](c.raw)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want error containing %q, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !proto.Equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
