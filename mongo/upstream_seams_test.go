package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestUpstreamOrderedBSONCarriersAndMarshalValueSeams(t *testing.T) {
	document := bson.D{
		{Key: "first", Value: 1},
		{Key: "second", Value: 2},
		{Key: "first", Value: 3},
		{Key: "array", Value: bson.A{int32(7), int32(3), int32(11)}},
	}
	encoded, err := bson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	elements, err := bson.Raw(encoded).Elements()
	if err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"first", "second", "first", "array"}
	if len(elements) != len(wantKeys) {
		t.Fatalf("element count = %d, want %d", len(elements), len(wantKeys))
	}
	for index, want := range wantKeys {
		if got := elements[index].Key(); got != want {
			t.Fatalf("element %d key = %q, want %q", index, got, want)
		}
	}

	values, err := elements[3].Value().Array().Values()
	if err != nil {
		t.Fatal(err)
	}
	wantValues := []int32{7, 3, 11}
	for index, want := range wantValues {
		if got := values[index].Int32(); got != want {
			t.Fatalf("array value %d = %d, want %d", index, got, want)
		}
	}

	valueType, _, err := bson.MarshalValue("literal")
	if err != nil || valueType != bson.TypeString {
		t.Fatalf("MarshalValue(string) = (%v, %v), want TypeString nil", valueType, err)
	}
}
