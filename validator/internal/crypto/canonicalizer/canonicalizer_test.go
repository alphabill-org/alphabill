package canonicalizer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type (
	ObjectNativeTypes struct {
		Number uint64 `hsh:"idx=1,size=8"`
		Slice  []byte `hsh:"idx=2"`
	}
	ObjectNestedTypes struct {
		Nested1     *ObjectNativeTypes   `hsh:"idx=1"`
		Nested2     *ObjectNativeTypes   `hsh:"idx=2"`
		NestedSlice []*ObjectNativeTypes `hsh:"idx=3"`
	}
	ValueStruct1 struct {
		Inner ValueStruct2 `hsh:"idx=1"`
	}
	ValueStruct2 struct {
		Inner uint64 `hsh:"idx=1,size=8"`
	}
)

func (o *ObjectNativeTypes) Canonicalize() ([]byte, error) {
	return Canonicalize(o)
}

func (o *ObjectNestedTypes) Canonicalize() ([]byte, error) {
	return Canonicalize(o)
}

func (o ValueStruct1) Canonicalize() ([]byte, error) {
	return Canonicalize(o)
}

func (o ValueStruct2) Canonicalize() ([]byte, error) {
	return Canonicalize(o)
}

var (
	objectNativeTypes = &ObjectNativeTypes{
		Number: 0x0102030405060708,
		Slice:  []byte("ObjectNativeTypes::Slice"),
	}

	objectNestedTypes = &ObjectNestedTypes{
		Nested1: objectNativeTypes,
		Nested2: &ObjectNativeTypes{
			Number: 1,
			Slice:  []byte("embedded::ObjectNativeTypes::Slice"),
		},
		NestedSlice: []*ObjectNativeTypes{objectNativeTypes, objectNativeTypes, objectNativeTypes},
	}
	valueStruct2 = ValueStruct2{
		Inner: 0x0102030405060708,
	}
	valueStruct1 = ValueStruct1{
		Inner: valueStruct2,
	}
)

func TestValueStruct(t *testing.T) {
	RegisterTemplate(valueStruct1)
	RegisterTemplate(valueStruct2)

	expected := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	actual, err := Canonicalize(valueStruct2)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestNativeTypes(t *testing.T) {
	RegisterTemplate(objectNativeTypes)

	expected := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	withoutFieldBin, err := Canonicalize(objectNativeTypes, OptionExcludeField("Slice"))
	require.NoError(t, err)
	require.Equal(t, expected, withoutFieldBin)

	expected = append(expected, []byte("ObjectNativeTypes::Slice")...)
	allFieldsBin, err := objectNativeTypes.Canonicalize()
	require.NoError(t, err)
	require.Equal(t, expected, allFieldsBin)
}

func TestNestedTypes(t *testing.T) {
	RegisterTemplate(objectNestedTypes)
	RegisterTemplate(objectNativeTypes)

	bin, err := objectNestedTypes.Canonicalize()
	require.NoError(t, err)

	nativeExp := append(
		[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		[]byte("ObjectNativeTypes::Slice")...,
	)
	nativeEmbExp := append(
		[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		[]byte("embedded::ObjectNativeTypes::Slice")...,
	)
	expected := append(nativeExp, nativeEmbExp...)
	expected = append(expected, nativeExp...)
	expected = append(expected, nativeExp...)
	expected = append(expected, nativeExp...)
	require.Equal(t, expected, bin)
}

func TestByteSize(t *testing.T) {
	type TestByteSize struct {
		Field1 uint64 `hsh:"idx=1,size=1"`
		Field2 uint64 `hsh:"idx=2,size=4"`
		Field3 uint64 `hsh:"idx=3,size=8"`
		Field4 uint64 `hsh:"idx=4"`
	}
	object := TestByteSize{1, 2, 3, 4}

	RegisterTemplate(&object)

	bin, err := Canonicalize(&object)
	require.NoError(t, err)
	require.Equal(t, bin, []byte{
		0x01,
		0x00, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03,
		0x04,
	})
}

func TestNotRegistered(t *testing.T) {
	type TestNotRegistered struct {
		Field1 uint64 `hsh:"idx=1,size=1"`
		Field2 uint64 `hsh:"idx=2,size=4"`
		Field3 uint64 `hsh:"idx=3,size=8"`
		Field4 uint64 `hsh:"idx=4"`
	}
	object := TestNotRegistered{1, 2, 3, 4}

	bin, err := Canonicalize(&object)
	require.Error(t, err)
	require.Empty(t, bin)
}

func TestInvalidByteSize(t *testing.T) {
	type TestInvalidByteSize struct {
		Field1 uint64 `hsh:"idx=1,size=1"`
		Field2 uint64 `hsh:"idx=2,size=2"`
		Field3 uint64 `hsh:"idx=3,size=3"`
	}
	object := TestInvalidByteSize{1, 2, 3}

	RegisterTemplate(&object)

	bin, err := Canonicalize(&object)
	require.Error(t, err)
	require.Empty(t, bin)
}

func TestDuplicateIndices(t *testing.T) {
	type TestDuplicateIndices struct {
		Field1 uint64 `hsh:"idx=1,size=1"`
		Field2 uint64 `hsh:"idx=1,size=4"`
	}
	object := TestDuplicateIndices{1, 2}

	require.Panics(t, func() { RegisterTemplate(object) })
}

func TestObjectWithoutHshTag(t *testing.T) {
	type TestObjectWithoutHshTag struct {
		Field1 uint64 `hsh:"idx=1,size=1"`
		Field2 uint64
	}
	object := TestObjectWithoutHshTag{1, 2}

	require.Panics(t, func() { RegisterTemplate(object) })
}

func TestNestedNonPointer(t *testing.T) {
	type (
		TestNestedNonPointer struct {
			Field1 ObjectNativeTypes `hsh:"idx=1"`
		}
	)
	object := TestNestedNonPointer{*objectNativeTypes}

	RegisterTemplate(&object)

	bin, err := Canonicalize(&object)
	require.NoError(t, err)
	require.NotEmpty(t, bin)
}
