package lzo

import (
	"bytes"
	"io/ioutil"
	"reflect"
	"testing"
)

func Test1(t *testing.T) {
	data, err := ioutil.ReadFile("testdata/sipref.txt")
	if err != nil {
		t.Fatal(err)
	}

	cmp := Compress1X(data)
	t.Logf("input: %d, output: %d", len(data), len(cmp))

	data2, err := Decompress1X(bytes.NewReader(cmp), len(cmp))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(data, data2) {
		t.Fatal("decompressed data doesn't match")
	}
}

func Test999(t *testing.T) {
	data, err := ioutil.ReadFile("testdata/sipref.txt")
	if err != nil {
		t.Fatal(err)
	}

	cmp := Compress999X(data)
	t.Logf("input: %d, output: %d", len(data), len(cmp))

	data2, err := Decompress1X(bytes.NewReader(cmp), len(cmp))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(data, data2) {
		t.Fatal("decompressed data doesn't match")
	}
}
