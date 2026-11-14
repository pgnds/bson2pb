package bson2pb_test

import (
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

func TestConvert(t *testing.T) {
	cases := []struct {
		name    string
		raw     bson.Raw
		want    *testpb.Hello
		wantErr string
	}{
		{
			name: "empty document",
			raw:  doc(bson.D{}),
			want: &testpb.Hello{},
		},
		{
			name: "simple name",
			raw:  doc(bson.D{{Key: "name", Value: "Alice"}}),
			want: &testpb.Hello{Name: "Alice"},
		},
		{
			name: "unknown key skipped",
			raw:  doc(bson.D{{Key: "name", Value: "Alice"}, {Key: "extra", Value: 1}}),
			want: &testpb.Hello{Name: "Alice"},
		},
		{
			name: "empty string",
			raw:  doc(bson.D{{Key: "name", Value: ""}}),
			want: &testpb.Hello{},
		},
		{
			name: "wrong case",
			raw:  doc(bson.D{{Key: "Name", Value: "Alice"}}),
			want: &testpb.Hello{},
		},
		{
			name:    "wrong type",
			raw:     doc(bson.D{{Key: "name", Value: 42}}),
			wantErr: "malformed",
		},
		{
			name:    "truncated bytes",
			raw:     doc(bson.D{{Key: "name", Value: "Alice"}})[:15],
			wantErr: "name",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := &testpb.Hello{}
			err := bson2pb.Convert(c.raw, got)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want error containing %q, got %v", c.wantErr, err)
				}
				return
			}
			if !proto.Equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
