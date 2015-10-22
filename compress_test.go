package lzo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func logn(n, b float64) float64 {
	return math.Log(n) / math.Log(b)
}

func humanateBytes(s uint64, base float64, sizes []string) string {
	if s < 10 {
		return fmt.Sprintf("%dB", s)
	}
	e := math.Floor(logn(float64(s), base))
	suffix := sizes[int(e)]
	val := math.Floor(float64(s)/math.Pow(base, e)*10+0.5) / 10
	f := "%.0f%s"
	if val < 10 {
		f = "%.1f%s"
	}

	return fmt.Sprintf(f, val, suffix)
}

func IBytes(s uint64) string {
	sizes := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	return humanateBytes(s, 1024, sizes)
}

func testCorpus(t *testing.T, arch string, cmpfunc func([]byte) []byte) (tdata int, tcmp int, tt time.Duration) {
	t.Log("Test corpus:", arch)
	f, err := os.Open(arch)
	if err != nil {
		t.Error(err)
		return
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Error(err)
		return
	}
	defer gz.Close()

	tgz := tar.NewReader(gz)
	for {
		head, err := tgz.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Error(err)
			return
		}

		data := make([]byte, head.Size)
		_, err = io.ReadFull(tgz, data)
		if err != nil {
			t.Error(err)
			return
		}

		t0 := time.Now()
		cmp := cmpfunc(data)
		tt += time.Now().Sub(t0)
		// t.Logf("File: %-20s Size: %-10v Compressed: %-10v Factor %0.1f%%", head.Name,
		// 	len(data), len(cmp), float32(len(data)-len(cmp))*100/float32(len(data)))

		data2, err := Decompress1X(bytes.NewReader(cmp), len(cmp), len(data))
		if err != nil {
			t.Error(err)
			continue
		}

		if !reflect.DeepEqual(data, data2) {
			t.Error("decompressed data doesn't match")
		}

		tdata += len(data)
		tcmp += len(cmp)
	}
	t.Logf("Total corpus stats: Size: %v, Compressed: %v, Factor: %0.1f%%, Elapsed: %v, Speed: %v/s",
		tdata, tcmp, float32(tdata-tcmp)*100/float32(tdata),
		tt, IBytes(uint64(float64(tdata)/tt.Seconds())))
	return
}

func testCorpora(t *testing.T, cmpfunc func([]byte) []byte) {
	archs, err := filepath.Glob("testdata/*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	tdata, tcmp := 0, 0
	var tt time.Duration
	for _, arch := range archs {
		d, c, t := testCorpus(t, arch, cmpfunc)
		tdata += d
		tcmp += c
		tt += t
	}

	t.Logf("Total stats: Size: %v, Compressed: %v, Factor: %0.1f%%, Elapsed: %v, Speed: %v/s",
		tdata, tcmp, float32(tdata-tcmp)*100/float32(tdata),
		tt, IBytes(uint64(float64(tdata)/tt.Seconds())))

}

func TestDecompInlen(t *testing.T) {
	data := bytes.Repeat([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 1000)
	cmp := Compress1X(data)

	for i := 1; i < 16; i++ {
		for j := -16; j < 16; j++ {
			_, err := Decompress1X(io.LimitReader(bytes.NewReader(cmp), int64(len(cmp)-i)), len(cmp)+j, 0)
			if err != io.EOF {
				t.Error("EOF expected for truncated input, found:", err)
			}
		}
	}

	for j := -16; j < 16; j++ {
		data2, err := Decompress1X(bytes.NewReader(cmp), len(cmp)+j, 0)
		if j < 0 && err != io.EOF {
			t.Error("EOF expected for truncated input, found:", err)
		}
		if j >= 0 {
			if err != nil {
				t.Error("error for normal decompression:", err, j)
			} else if !reflect.DeepEqual(data, data2) {
				t.Error("data doesn't match after decompression")
			}
		}
	}
}

func Test1(t *testing.T) {
	testCorpora(t, Compress1X)
}

func Test999(t *testing.T) {
	maxlevel := 9
	if testing.Short() {
		maxlevel = 5
	}
	for i := 1; i <= maxlevel; i++ {
		testCorpora(t, func(in []byte) []byte {
			return Compress1X999Level(in, i)
		})
	}
}

func BenchmarkComp(b *testing.B) {
	f, err := os.Open("testdata/large.tar.gz")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		b.Error(err)
		return
	}
	defer gz.Close()

	var buf bytes.Buffer
	io.Copy(&buf, gz)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Compress1X(buf.Bytes())
	}
}

func BenchmarkDecomp(b *testing.B) {
	f, err := os.Open("testdata/large.tar.gz")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		b.Error(err)
		return
	}
	defer gz.Close()

	var buf bytes.Buffer
	io.Copy(&buf, gz)

	cmp := Compress1X(buf.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decompress1X(bytes.NewReader(cmp), len(cmp), buf.Len())
	}
}
