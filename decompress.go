package lzo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
)

type reader struct {
	r   io.Reader
	len int
	buf [4096]byte
	cur []byte
	Err error
}

var (
	InputUnderrun      = errors.New("input underrun")
	LookBehindUnderrun = errors.New("lookbehind underrun")
	Verbose            = true
	Debug              = func(string, ...interface{}) {}
)

func (h *Header) String() string {
	return fmt.Sprintf("Magic %#02x Version %#04x LibVersion %#04x NeededVersion %#04x Method %#02x Level %#02x Flags %#08x Filter %#08x Mode %#08x Mtime %#08x GMTDiff %#08x Name %q DataCSUM %#08x UnCompressedSize %#08x CompressedSize %#08x CompressedCSUM %#08x UncompressedCSUM %#04x",
		h.Magic,
		h.Version,
		h.LibVersion,
		h.NeededVersion,
		h.Method,
		h.Level,
		h.Flags,
		h.Filter,
		h.Mode,
		h.Mtime,
		h.GMTDiff,
		h.Name,
		h.DataCSUM,
		h.UnCompressedSize,
		h.CompressedSize,
		h.CompressedCSUM,
		h.UncompressedCSUM)
}

// readHeader decompresses a Header from an io.Reader. The docs on this format may not exist.
// Here is a typical header generated for a linux kernel used in a FIT.
// 00000000  89 4c 5a 4f 00 0d 0a 1a  0a
//           10 40
//           20 a0
//           09 40
//           01  |.LZO......@ ..@.|
// 00000010  05
//           03 00 00 01   // flags.
//                         no filter
//           00 00 81 ed // Mode
//           5f 6e 4d cf // Mtime
//           00 00 00  |........._nM....|
// 00000020  00 // GMTIME
// namelen 0e 76 6d 6c 69 6e 75  78 2e 62 69 6e 61 72 79  |..vmlinux.binary|
// past the name.
// From here on, whatever it is, it us a uint32
// 00000030  a9 d5 0a 8f
//           00 04 00 00
//           00 03 06 bb
//           e8 97 4a 8a  |..............J.|
// 00000040  00 28 48 8d // is this optional? *yes this may be the optional part. *
// There will be at most this much, but this may be too much.
//            25 51 3f c0  00 e8 d4 00 00 00 48 8d  |.(H.%Q?.......H.|
func readHeader(r io.Reader) (*Header, error) {
	var h Header

	if _, err := r.Read(h.Magic[:]); err != nil {
		return nil, err
	}

	if string(h.Magic[:]) != magic {
		return nil, fmt.Errorf("Magic is %#02x, should be %#02x", h.Magic, magic)
	}
	var n byte
	// This would be so easy were the variable data not in the middle. but ...
	// We could use reflect here, but ... eh. It's not that complex.
	for _, f := range []interface{}{&h.Version, &h.LibVersion, &h.NeededVersion, &h.Method, &h.Level, &h.Flags, &h.Filter, &h.Mode, &h.Mtime, &h.GMTDiff, &n, &h.DataCSUM, &h.UnCompressedSize, &h.CompressedSize, &h.UnCompressedCSUM, &h.CompressedCSUM} {
		switch f {
		case &h.Filter:
			if (h.Flags & fHFilter) == 0 {
				continue
			}
		case &n:
			b := make([]byte, n)
			if _, err := r.Read(b); err != nil {
				return nil, fmt.Errorf("Error reading file name: %v", err)
			}
			Debug("Name is %d bytes", n)
			name := make([]byte, n)
			if _, err := r.Read(b); err != nil {
				return nil, fmt.Errorf("Reading file name: %v", err)
			}
			h.Name = string(name)
			Debug("Name is %q", name)
		case &h.CompressedCSUM:
			if (h.Flags & (fChecksumAdler | fChecksumCRC32)) == 0 {
				continue
			}
		}
		if err := binary.Read(r, binary.BigEndian, f); err != nil {
			return nil, err
		}
	}
	return &h, nil
}

// DecompressLZOP decompresses a byte slice as compressed by the standard lzop(1) program.
func DecompressLZOP(r io.Reader) (out []byte, err error) {
	// This was kind of a nice idea. But the docs are so useless, and different libraries
	// don't agree. It's awesome.
	h, err := readHeader(r)
	if err != nil {
		return nil, err
	}
	Debug("Header: %s", h)
	return Decompress1X(r, 0, 0)
}

func newReader(r io.Reader, inlen int) *reader {
	if inlen == 0 {
		inlen = -1
	}
	in := &reader{r: r, len: inlen}
	in.Rebuffer()
	return in
}

// Read more data from the underlying reader and put it into the buffer.
// Also makes sure there is always at least 32 bytes in the buffer, so that
// in the main loop we can avoid checking for the end of buffer.
func (in *reader) Rebuffer() {
	const RBUF_WND = 32
	var rbuf [RBUF_WND]byte

	if len(in.cur) > RBUF_WND || in.len == 0 {
		return
	}

	rb := rbuf[:len(in.cur)]
	copy(rb, in.cur)
	in.cur = in.buf[:]
	copy(in.cur, rb)

	cur := in.cur[len(rb):]
	if in.len >= 0 && len(cur) > in.len {
		cur = cur[:in.len]
	}
	n, err := in.r.Read(cur)
	if Verbose {
		n := len(cur)
		dotdotdot := ""
		if len(cur) > 64 {
			n = 32
			dotdotdot = "..."
		}
		Debug("rebuffer: read %#02x bytes: %q%s", n, cur[:n], dotdotdot)
	}
	if err != nil {
		// If EOF is returned, treat it as error only if there are no further
		// bytes in the window. Otherwise, let's postpone because those bytes
		// could contain the terminator.
		if err != io.EOF || len(rb) == 0 {
			in.Err = err
			in.cur = nil
		}
	}
	in.cur = in.cur[:len(rb)+n]
	if in.len >= 0 {
		in.len -= n
	}
	if Verbose {
		Debug("Rebuffer: %d bytes left", in.len)
	}
}

func (in *reader) ReadAppend(out *[]byte, n int) {
	for n > 0 {
		m := len(in.cur)
		if m > n {
			m = n
		}
		*out = append(*out, in.cur[:m]...)
		in.cur = in.cur[m:]
		n -= m
		if len(in.cur) == 0 {
			in.Rebuffer()
			if len(in.cur) == 0 {
				in.Err = io.EOF
				return
			}
		}
	}
	return
}

func (in *reader) ReadU8() (ch byte) {
	ch = in.cur[0]
	in.cur = in.cur[1:]
	return
}

func (in *reader) ReadU16() int {
	b0 := in.cur[0]
	b1 := in.cur[1]
	in.cur = in.cur[2:]
	return int(b0) + int(b1)<<8
}

func (in *reader) ReadMulti(base int) (b int) {
	for {
		for i := 0; i < len(in.cur); i++ {
			v := in.cur[i]
			if v == 0 {
				b += 255
			} else {
				b += int(v) + base
				in.cur = in.cur[i+1:]
				return
			}
		}
		in.cur = in.cur[0:0]
		in.Rebuffer()
		if len(in.cur) == 0 {
			in.Err = io.EOF
			return
		}
	}
}

func copyMatch(out *[]byte, m_pos int, n int) {
	if m_pos+n > len(*out) {
		if Verbose {
			Debug("copy match WITH OVERLAP!")
		}
		for i := 0; i < n; i++ {
			*out = append(*out, (*out)[m_pos])
			m_pos++
		}
	} else {
		if Verbose {
			Debug("copy match: %d %d %d", len(*out), m_pos, m_pos+n)
		}
		*out = append(*out, (*out)[m_pos:m_pos+n]...)
	}
}

// Decompress an input compressed with LZO1X.
//
// LZO1X has a stream terminator marker, so the decompression will always stop
// when this marker is found.
//
// If inLen is not zero, it is expected to match the length of the compressed
// input stream, and it is used to limit reads from the underlying reader; if
// inLen is smaller than the real stream, the decompression will abort with an
// error; if inLen is larger than the real stream, or if it is zero, the
// decompression will succeed but more bytes than necessary might be read
// from the underlying reader. If the reader returns EOF before the termination
// marker is found, the decompression aborts and EOF is returned.
//
// outLen is optional; if it's not zero, it is used as a hint to preallocate the
// output buffer to increase performance of the decompression.
func Decompress1X(r io.Reader, inLen int, outLen int) (out []byte, err error) {
	var t, m_pos int
	var last2 byte

	defer func() {
		// To gain performance, we don't do any bounds checking while reading
		// the input, so if the decompressor reads past the end of the input
		// stream, a runtime error is raised. This saves about 7% of performance
		// as the reading functions are very hot in the decompressor.
		Debug("panic:")
		if r := recover(); r != nil {
			Debug("recover: %v %T", r, r)
			if re, ok := r.(runtime.Error); ok {
				if strings.HasPrefix(re.Error(), "runtime error: index out of range") {
					err = io.EOF
					return
				}
				if strings.HasPrefix(re.Error(), "runtime error: slice bounds out of range") {
					err = io.EOF
					return
				}
				Debug("%v does not match a string we know", r)
			}
			Debug("Don't know what to do with this type")
			panic(r)
		}
	}()

	out = make([]byte, 0, outLen)

	in := newReader(r, inLen)
	ip := in.ReadU8()
	if ip > 17 {
		t = int(ip) - 17
		if t < 4 {
			goto match_next
		}
		in.ReadAppend(&out, t)
		if Verbose {
			Debug("begin: %q", string(out))
		}
		goto first_literal_run
	}

begin_loop:
	t = int(ip)
	if t >= 16 {
		goto match
	}
	if t == 0 {
		t = in.ReadMulti(15)
	}
	in.ReadAppend(&out, t+3)
	if Verbose {
		Debug("readappend %d %q", t+3, string(out[len(out)-t-3:]))
	}
first_literal_run:
	ip = in.ReadU8()
	last2 = ip
	t = int(ip)
	if t >= 16 {
		goto match
	}
	m_pos = len(out) - (1 + m2_MAX_OFFSET)
	m_pos -= t >> 2
	ip = in.ReadU8()
	m_pos -= int(ip) << 2
	if Verbose {
		Debug("m_pos flr", m_pos, len(out), "\n", string(out))
	}
	if m_pos < 0 {
		//err = LookBehindUnderrun
		err = fmt.Errorf("Lookbehind overrun1 at offset %d, m_pos %#02x", ip, m_pos)
		return
	}
	copyMatch(&out, m_pos, 3)
	goto match_done

match:
	in.Rebuffer()
	if in.Err != nil {
		err = in.Err
		return
	}
	t = int(ip)
	last2 = ip
	if t >= 64 {
		m_pos = len(out) - 1
		m_pos -= (t >> 2) & 7
		ip = in.ReadU8()
		m_pos -= int(ip) << 3
		if Verbose {
			Debug("m_pos t64 %d %d %d", m_pos, t, int(ip))
		}
		t = (t >> 5) - 1
		goto copy_match
	} else if t >= 32 {
		t &= 31
		if t == 0 {
			t = in.ReadMulti(31)
		}
		m_pos = len(out) - 1
		v16 := in.ReadU16()
		m_pos -= v16 >> 2
		last2 = byte(v16 & 0xFF)
		if Verbose {
			Debug("m_pos t32 %d", m_pos)
		}
	} else if t >= 16 {
		m_pos = len(out)
		m_pos -= (t & 8) << 11
		t &= 7
		if t == 0 {
			t = in.ReadMulti(7)
		}
		v16 := in.ReadU16()
		m_pos -= v16 >> 2
		if m_pos == len(out) {
			if Verbose {
				Debug("END %d %d %d", t, v16, m_pos)
			}
			return
		}
		m_pos -= 0x4000
		last2 = byte(v16 & 0xFF)
		if Verbose {
			Debug("m_pos t16", m_pos)
		}
	} else {
		m_pos = len(out) - 1
		m_pos -= t >> 2
		ip = in.ReadU8()
		m_pos -= int(ip) << 2
		if m_pos < 0 {
			err = fmt.Errorf("Lookbehind overrun2 at offset %d, m_pos %#02x", ip, m_pos)
			return
		}
		if Verbose {
			Debug("m_pos tX", m_pos)
		}
		copyMatch(&out, m_pos, 2)
		goto match_done
	}

copy_match:
	if m_pos < 0 {
		err = fmt.Errorf("Lookbehind overrun3 at offset %d, m_pos %#02x", ip, m_pos)
		return
	}
	copyMatch(&out, m_pos, t+2)

match_done:
	t = int(last2 & 3)
	if t == 0 {
		goto match_end
	}
match_next:
	if Verbose {
		Debug("read append finale:%d", t)
	}
	in.ReadAppend(&out, t)
	ip = in.ReadU8()
	goto match

match_end:
	ip = in.ReadU8()
	goto begin_loop
}
