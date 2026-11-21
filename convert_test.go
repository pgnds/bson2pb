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

func badOldBinary() bson.Raw {
	b := doc(bson.D{{Key: "googly_eye_pixels", Value: bson.Binary{Subtype: 0x02, Data: []byte("abc")}}})
	b[len(b)-8] = 0x09
	return b
}

type convertCase struct {
	name    string
	raw     bson.Raw
	want    *testpb.EverythingEverywhereAllAtOnce
	wantErr string
}

func runConvertCases(t *testing.T, cases []convertCase) {
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

func TestConvert(t *testing.T) {
	cases := []convertCase{
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
		{
			name: "uint32",
			raw:  doc(bson.D{{Key: "laundry_loads", Value: 32}}),
			want: &testpb.EverythingEverywhereAllAtOnce{LaundryLoads: 32},
		},
		{
			name: "uint32 - zero",
			raw:  doc(bson.D{{Key: "laundry_loads", Value: 0}}),
			want: &testpb.EverythingEverywhereAllAtOnce{LaundryLoads: 0},
		},
		{
			name: "uint32 - max",
			raw:  doc(bson.D{{Key: "laundry_loads", Value: math.MaxInt32}}),
			want: &testpb.EverythingEverywhereAllAtOnce{LaundryLoads: math.MaxInt32},
		},
		{
			name:    "uint32 - negative",
			raw:     doc(bson.D{{Key: "laundry_loads", Value: -1}}),
			wantErr: "negative unsigned",
		},
		{
			name:    "uint32 - wrong type",
			raw:     doc(bson.D{{Key: "laundry_loads", Value: int64(32)}}),
			wantErr: "malformed",
		},
		{
			name: "fixed32",
			raw:  doc(bson.D{{Key: "sentient_rocks", Value: 32}}),
			want: &testpb.EverythingEverywhereAllAtOnce{SentientRocks: 32},
		},
		{
			name: "fixed32 - zero",
			raw:  doc(bson.D{{Key: "sentient_rocks", Value: 0}}),
			want: &testpb.EverythingEverywhereAllAtOnce{SentientRocks: 0},
		},
		{
			name: "fixed32 - max",
			raw:  doc(bson.D{{Key: "sentient_rocks", Value: math.MaxInt32}}),
			want: &testpb.EverythingEverywhereAllAtOnce{SentientRocks: math.MaxInt32},
		},
		{
			name:    "fixed32 - negative",
			raw:     doc(bson.D{{Key: "sentient_rocks", Value: -1}}),
			wantErr: "negative unsigned",
		},
		{
			name:    "fixed32 - wrong type",
			raw:     doc(bson.D{{Key: "sentient_rocks", Value: int64(32)}}),
			wantErr: "malformed",
		},
	}

	runConvertCases(t, cases)
}

func TestConvertInt64(t *testing.T) {
	cases := []convertCase{
		{
			name: "simple",
			raw:  doc(bson.D{{Key: "universes", Value: int64(64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Universes: 64},
		},
		{
			name:    "string value",
			raw:     doc(bson.D{{Key: "universes", Value: "64"}}),
			wantErr: "malformed",
		},
		{
			name:    "int32 value",
			raw:     doc(bson.D{{Key: "universes", Value: int32(64)}}),
			wantErr: "malformed",
		},
		{
			name: "zero",
			raw:  doc(bson.D{{Key: "universes", Value: int64(0)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Universes: 0},
		},
		{
			name: "negative",
			raw:  doc(bson.D{{Key: "universes", Value: int64(-1)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Universes: -1},
		},
		{
			name: "min",
			raw:  doc(bson.D{{Key: "universes", Value: int64(math.MinInt64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Universes: math.MinInt64},
		},
		{
			name: "max",
			raw:  doc(bson.D{{Key: "universes", Value: int64(math.MaxInt64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Universes: math.MaxInt64},
		},
		{
			name:    "truncated",
			raw:     doc(bson.D{{Key: "universes", Value: int64(123)}})[:19],
			wantErr: "malformed value: truncated",
		},
		{
			name: "sint64",
			raw:  doc(bson.D{{Key: "verse_jumps", Value: int64(64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{VerseJumps: 64},
		},
		{
			name: "sint64 negative",
			raw:  doc(bson.D{{Key: "verse_jumps", Value: int64(-1)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{VerseJumps: -1},
		},
		{
			name: "sint64 min",
			raw:  doc(bson.D{{Key: "verse_jumps", Value: int64(math.MinInt64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{VerseJumps: math.MinInt64},
		},
		{
			name: "sint64 max",
			raw:  doc(bson.D{{Key: "verse_jumps", Value: int64(math.MaxInt64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{VerseJumps: math.MaxInt64},
		},
		{
			name: "sfixed64",
			raw:  doc(bson.D{{Key: "everything_bagel", Value: int64(64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{EverythingBagel: 64},
		},
		{
			name: "sfixed64 negative",
			raw:  doc(bson.D{{Key: "everything_bagel", Value: int64(-1)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{EverythingBagel: -1},
		},
		{
			name: "sfixed64 min",
			raw:  doc(bson.D{{Key: "everything_bagel", Value: int64(math.MinInt64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{EverythingBagel: math.MinInt64},
		},
		{
			name: "sfixed64 max",
			raw:  doc(bson.D{{Key: "everything_bagel", Value: int64(math.MaxInt64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{EverythingBagel: math.MaxInt64},
		},
		{
			name: "uint64",
			raw:  doc(bson.D{{Key: "tax_receipts", Value: int64(64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{TaxReceipts: 64},
		},
		{
			name: "uint64 zero",
			raw:  doc(bson.D{{Key: "tax_receipts", Value: int64(0)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{TaxReceipts: 0},
		},
		{
			name: "uint64 max",
			raw:  doc(bson.D{{Key: "tax_receipts", Value: int64(math.MaxInt64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{TaxReceipts: math.MaxInt64},
		},
		{
			name:    "uint64 negative",
			raw:     doc(bson.D{{Key: "tax_receipts", Value: int64(-1)}}),
			wantErr: "negative unsigned",
		},
		{
			name:    "uint64 int32 value",
			raw:     doc(bson.D{{Key: "tax_receipts", Value: int32(64)}}),
			wantErr: "malformed",
		},
		{
			name: "fixed64",
			raw:  doc(bson.D{{Key: "alpha_verses", Value: int64(64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{AlphaVerses: 64},
		},
		{
			name: "fixed64 zero",
			raw:  doc(bson.D{{Key: "alpha_verses", Value: int64(0)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{AlphaVerses: 0},
		},
		{
			name: "fixed64 max",
			raw:  doc(bson.D{{Key: "alpha_verses", Value: int64(math.MaxInt64)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{AlphaVerses: math.MaxInt64},
		},
		{
			name:    "fixed64 negative",
			raw:     doc(bson.D{{Key: "alpha_verses", Value: int64(-1)}}),
			wantErr: "negative unsigned",
		},
		{
			name:    "fixed64 int32 value",
			raw:     doc(bson.D{{Key: "alpha_verses", Value: int32(64)}}),
			wantErr: "malformed",
		},
	}

	runConvertCases(t, cases)
}

func TestConvertDouble(t *testing.T) {
	cases := []convertCase{
		{
			name: "simple",
			raw:  doc(bson.D{{Key: "improbability", Value: 1.5}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Improbability: 1.5},
		},
		{
			name: "negative",
			raw:  doc(bson.D{{Key: "improbability", Value: -2.5}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Improbability: -2.5},
		},
		{
			name: "zero",
			raw:  doc(bson.D{{Key: "improbability", Value: 0.0}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Improbability: 0.0},
		},
		{
			name: "max",
			raw:  doc(bson.D{{Key: "improbability", Value: math.MaxFloat64}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Improbability: math.MaxFloat64},
		},
		{
			name: "min",
			raw:  doc(bson.D{{Key: "improbability", Value: math.SmallestNonzeroFloat64}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Improbability: math.SmallestNonzeroFloat64},
		},
		{
			name: "+Inf",
			raw:  doc(bson.D{{Key: "improbability", Value: math.Inf(1)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Improbability: math.Inf(1)},
		},
		{
			name: "-Inf",
			raw:  doc(bson.D{{Key: "improbability", Value: math.Inf(-1)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Improbability: math.Inf(-1)},
		},
		{
			name: "NaN",
			raw:  doc(bson.D{{Key: "improbability", Value: math.NaN()}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Improbability: math.NaN()},
		},
		{
			name:    "string",
			raw:     doc(bson.D{{Key: "improbability", Value: "1.5"}}),
			wantErr: "malformed",
		},
		{
			name:    "int64",
			raw:     doc(bson.D{{Key: "improbability", Value: int64(1)}}),
			wantErr: "malformed",
		},
		{
			name:    "int32",
			raw:     doc(bson.D{{Key: "improbability", Value: int32(1)}}),
			wantErr: "malformed",
		},
		{
			name:    "truncated",
			raw:     doc(bson.D{{Key: "improbability", Value: 1.5}})[:23],
			wantErr: "malformed value: truncated",
		},
		{
			name: "float",
			raw:  doc(bson.D{{Key: "raccacoonie_rating", Value: 1.5}}),
			want: &testpb.EverythingEverywhereAllAtOnce{RaccacoonieRating: 1.5},
		},
		{
			name: "float negative",
			raw:  doc(bson.D{{Key: "raccacoonie_rating", Value: -2.5}}),
			want: &testpb.EverythingEverywhereAllAtOnce{RaccacoonieRating: -2.5},
		},
		{
			name: "float zero",
			raw:  doc(bson.D{{Key: "raccacoonie_rating", Value: 0.0}}),
			want: &testpb.EverythingEverywhereAllAtOnce{RaccacoonieRating: 0.0},
		},
		{
			name: "float rounded",
			raw:  doc(bson.D{{Key: "raccacoonie_rating", Value: 0.1}}),
			want: &testpb.EverythingEverywhereAllAtOnce{RaccacoonieRating: 0.1},
		},
		{
			name: "float max",
			raw:  doc(bson.D{{Key: "raccacoonie_rating", Value: math.MaxFloat32}}),
			want: &testpb.EverythingEverywhereAllAtOnce{RaccacoonieRating: math.MaxFloat32},
		},
		{
			name: "float min",
			raw:  doc(bson.D{{Key: "raccacoonie_rating", Value: math.SmallestNonzeroFloat32}}),
			want: &testpb.EverythingEverywhereAllAtOnce{RaccacoonieRating: math.SmallestNonzeroFloat32},
		},
		{
			name: "float +Inf",
			raw:  doc(bson.D{{Key: "raccacoonie_rating", Value: math.Inf(1)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{RaccacoonieRating: float32(math.Inf(1))},
		},
		{
			name: "float -Inf",
			raw:  doc(bson.D{{Key: "raccacoonie_rating", Value: math.Inf(-1)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{RaccacoonieRating: float32(math.Inf(-1))},
		},
		{
			name: "float NaN",
			raw:  doc(bson.D{{Key: "raccacoonie_rating", Value: math.NaN()}}),
			want: &testpb.EverythingEverywhereAllAtOnce{RaccacoonieRating: float32(math.NaN())},
		},
		{
			name:    "float overflow",
			raw:     doc(bson.D{{Key: "raccacoonie_rating", Value: 1e39}}),
			wantErr: "float overflow",
		},
		{
			name:    "float negative overflow",
			raw:     doc(bson.D{{Key: "raccacoonie_rating", Value: -1e39}}),
			wantErr: "float overflow",
		},
	}

	runConvertCases(t, cases)

	t.Run("negative zero", func(t *testing.T) {
		raw := doc(bson.D{{Key: "improbability", Value: math.Copysign(0, -1)}})
		got, err := bson2pb.Convert[testpb.EverythingEverywhereAllAtOnce](raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f := got.GetImprobability(); f != 0 || !math.Signbit(f) {
			t.Errorf("got %v, want -0", f)
		}
	})

	t.Run("float negative zero", func(t *testing.T) {
		raw := doc(bson.D{{Key: "raccacoonie_rating", Value: math.Copysign(0, -1)}})
		got, err := bson2pb.Convert[testpb.EverythingEverywhereAllAtOnce](raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f := got.GetRaccacoonieRating(); f != 0 || !math.Signbit(float64(f)) {
			t.Errorf("got %v, want -0", f)
		}
	})
}

func TestConvertNested(t *testing.T) {
	cases := []convertCase{
		{
			name: "simple",
			raw: doc(bson.D{{
				Key: "home_verse", Value: bson.D{
					{Key: "name", Value: "Rock Verse"},
					{Key: "jump_count", Value: 3},
				}},
			}),
			want: &testpb.EverythingEverywhereAllAtOnce{
				HomeVerse: &testpb.Verse{Name: "Rock Verse", JumpCount: 3},
			},
		},
		{
			name: "empty",
			raw:  doc(bson.D{{Key: "home_verse", Value: bson.D{}}}),
			want: &testpb.EverythingEverywhereAllAtOnce{HomeVerse: &testpb.Verse{}},
		},
		{
			name:    "wrong type",
			raw:     doc(bson.D{{Key: "home_verse", Value: "Rock Verse"}}),
			wantErr: "want document",
		},
		{
			name: "wrong type in sub-document",
			raw: doc(bson.D{{
				Key: "home_verse", Value: bson.D{
					{Key: "name", Value: "Rock Verse"},
					{Key: "jump_count", Value: "3"},
				}},
			}),
			wantErr: `field "home_verse": field "jump_count"`,
		},
	}

	runConvertCases(t, cases)
}

func TestConvertBytes(t *testing.T) {
	uuid := []byte{0x9b, 0x2c, 0x3e, 0x41, 0x7a, 0x55, 0x4c, 0x1d, 0x8f, 0x02, 0x6e, 0xb3, 0x90, 0x11, 0xc4, 0x5a}

	cases := []convertCase{
		{
			name: "simple",
			raw:  doc(bson.D{{Key: "googly_eye_pixels", Value: []byte("abc")}}),
			want: &testpb.EverythingEverywhereAllAtOnce{GooglyEyePixels: []byte("abc")},
		},
		{
			name: "empty",
			raw:  doc(bson.D{{Key: "googly_eye_pixels", Value: []byte{}}}),
			want: &testpb.EverythingEverywhereAllAtOnce{GooglyEyePixels: []byte{}},
		},
		{
			name: "non-utf8",
			raw:  doc(bson.D{{Key: "googly_eye_pixels", Value: []byte("\xff\xfe")}}),
			want: &testpb.EverythingEverywhereAllAtOnce{GooglyEyePixels: []byte("\xff\xfe")},
		},
		{
			name: "uuid-subtype",
			raw:  doc(bson.D{{Key: "googly_eye_pixels", Value: bson.Binary{Subtype: 0x04, Data: uuid}}}),
			want: &testpb.EverythingEverywhereAllAtOnce{GooglyEyePixels: uuid},
		},
		{
			name: "old-binary",
			raw:  doc(bson.D{{Key: "googly_eye_pixels", Value: bson.Binary{Subtype: 0x02, Data: uuid}}}),
			want: &testpb.EverythingEverywhereAllAtOnce{GooglyEyePixels: uuid},
		},
		{
			name:    "wrong type",
			raw:     doc(bson.D{{Key: "googly_eye_pixels", Value: "alice"}}),
			wantErr: "want binary",
		},
		{
			name:    "corrupt old binary",
			raw:     badOldBinary(),
			wantErr: "invalid binary",
		},
	}

	runConvertCases(t, cases)
}

func TestConvertEnum(t *testing.T) {
	cases := []convertCase{
		{
			name: "simple",
			raw:  doc(bson.D{{Key: "mood", Value: int32(testpb.Mood_MOOD_KIND)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Mood: testpb.Mood_MOOD_KIND},
		},
		{
			name: "zero",
			raw:  doc(bson.D{{Key: "mood", Value: int32(0)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{},
		},
		{
			name: "negative",
			raw:  doc(bson.D{{Key: "mood", Value: int32(-1)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Mood: testpb.Mood_MOOD_BAGEL},
		},
		{
			name: "undeclared value",
			raw:  doc(bson.D{{Key: "mood", Value: int32(42)}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Mood: testpb.Mood(42)},
		},
		{
			name: "string value",
			raw:  doc(bson.D{{Key: "mood", Value: "MOOD_KIND"}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Mood: testpb.Mood_MOOD_KIND},
		},
		{
			name:    "int64 value",
			raw:     doc(bson.D{{Key: "mood", Value: int64(1)}}),
			wantErr: "malformed",
		},
		{
			name: "string zero",
			raw:  doc(bson.D{{Key: "mood", Value: "MOOD_UNSPECIFIED"}}),
			want: &testpb.EverythingEverywhereAllAtOnce{},
		},
		{
			name: "string negative",
			raw:  doc(bson.D{{Key: "mood", Value: "MOOD_BAGEL"}}),
			want: &testpb.EverythingEverywhereAllAtOnce{Mood: testpb.Mood_MOOD_BAGEL},
		},
		{
			name:    "unknown name",
			raw:     doc(bson.D{{Key: "mood", Value: "MOOD_NOPE"}}),
			wantErr: "unknown enum name",
		},
		{
			name:    "wrong case",
			raw:     doc(bson.D{{Key: "mood", Value: "mood_kind"}}),
			wantErr: "unknown enum name",
		},
		{
			name:    "empty string",
			raw:     doc(bson.D{{Key: "mood", Value: ""}}),
			wantErr: "unknown enum name",
		},
	}

	runConvertCases(t, cases)
}
