package lzo

import (
	"bufio"
	"errors"
	"io"
)

var (
	InputUnderrun      = errors.New("input underrun")
	LookBehindUnderrun = errors.New("lookbehind underrun")
)

type reader struct {
	io.ByteReader
	Last1, Last2 byte
}

func newReader(r io.Reader) *reader {
	// If ByteReader is not implemented, wrap it in a bufio
	in, ok := r.(io.ByteReader)
	if !ok {
		in = bufio.NewReader(r)
	}
	return &reader{ByteReader: in}
}

func (in *reader) ReadAppend(out *[]byte, n int) (err error) {
	var ch byte
	for i := 0; i < n; i++ {
		ch, err = in.ReadByte()
		if err != nil {
			return
		}
		*out = append(*out, ch)
	}
	return
}

func (in *reader) ReadByte() (b byte, err error) {
	b, err = in.ByteReader.ReadByte()
	in.Last2 = in.Last1
	in.Last1 = b
	return
}

func (in *reader) ReadU16() (b int, err error) {
	in.Last2, err = in.ByteReader.ReadByte()
	if err != nil {
		return
	}
	in.Last1, err = in.ByteReader.ReadByte()
	if err != nil {
		return
	}
	b = int(in.Last2) + int(in.Last1)*256
	return
}

func (in *reader) ReadMulti(base int) (b int, err error) {
	var v byte
	for {
		if v, err = in.ReadByte(); err != nil {
			return
		}
		if v == 0 {
			b += 255
		} else {
			b += int(v) + base
			return
		}
	}
}

func copyMatch(out *[]byte, m_pos int, n int) {
	if m_pos+n > len(*out) {
		// fmt.Println("copy match WITH OVERLAP!")
		for i := 0; i < n; i++ {
			*out = append(*out, (*out)[m_pos])
			m_pos++
		}
	} else {
		*out = append(*out, (*out)[m_pos:m_pos+n]...)
		// fmt.Println("copy match:", string((*out)[m_pos:m_pos+n]))
	}
}

// Decompress an input compressed with LZO1X. If r does not also implement
// io.ByteReader, the decompressor may read more data than necessary from r.
// out_len is optional; if it's not zero, it is used as a hint to
// preallocate the output buffer.
func Decompress1X(r io.Reader, in_len int, out_len int) (out []byte, err error) {
	var ip byte
	var t, m_pos int

	in := newReader(r)

	out = make([]byte, 0, out_len)
	if ip, err = in.ReadByte(); err != nil {
		return
	}
	if ip > 17 {
		t = int(ip) - 17
		if t < 4 {
			goto match_next
		}
		if err = in.ReadAppend(&out, t); err != nil {
			return
		}
		// fmt.Println("begin:", string(out))
		goto first_literal_run
	}

begin_loop:
	t = int(ip)
	if t >= 16 {
		goto match
	}
	if t == 0 {
		if t, err = in.ReadMulti(15); err != nil {
			return
		}
	}
	if err = in.ReadAppend(&out, t+3); err != nil {
		return
	}
	// fmt.Println("readappend", t+3, string(out[len(out)-t-3:]))
first_literal_run:
	if ip, err = in.ReadByte(); err != nil {
		return
	}
	t = int(ip)
	if t >= 16 {
		goto match
	}
	m_pos = len(out) - (1 + m2_MAX_OFFSET)
	m_pos -= t >> 2
	if ip, err = in.ReadByte(); err != nil {
		return
	}
	m_pos -= int(ip) << 2
	// fmt.Println("m_pos flr", m_pos, len(out), "\n", string(out))
	if m_pos < 0 {
		err = LookBehindUnderrun
		return
	}
	copyMatch(&out, m_pos, 3)
	goto match_done

match:
	t = int(ip)
	if t >= 64 {
		m_pos = len(out) - 1
		m_pos -= (t >> 2) & 7
		if ip, err = in.ReadByte(); err != nil {
			return
		}
		m_pos -= int(ip) << 3
		// fmt.Println("m_pos t64", m_pos, t, int(ip))
		t = (t >> 5) - 1
		goto copy_match
	} else if t >= 32 {
		t &= 31
		if t == 0 {
			if t, err = in.ReadMulti(31); err != nil {
				return
			}
		}
		m_pos = len(out) - 1
		var v16 int
		if v16, err = in.ReadU16(); err != nil {
			return
		}
		m_pos -= v16 >> 2
		// fmt.Println("m_pos t32", m_pos)
	} else if t >= 16 {
		m_pos = len(out)
		m_pos -= (t & 8) << 11
		t &= 7
		if t == 0 {
			if t, err = in.ReadMulti(7); err != nil {
				return
			}
		}
		var v16 int
		if v16, err = in.ReadU16(); err != nil {
			return
		}
		m_pos -= v16 >> 2
		if m_pos == len(out) {
			// fmt.Println("FINEEEEE", t, v16, m_pos)
			return
		}
		m_pos -= 0x4000
		// fmt.Println("m_pos t16", m_pos)
	} else {
		m_pos = len(out) - 1
		m_pos -= t >> 2
		if ip, err = in.ReadByte(); err != nil {
			return
		}
		m_pos -= int(ip) << 2
		// fmt.Println("m_pos tX", m_pos)
		copyMatch(&out, m_pos, 2)
		goto match_done
	}

copy_match:
	if m_pos < 0 {
		err = LookBehindUnderrun
		return
	}
	copyMatch(&out, m_pos, t+2)

match_done:
	t = int(in.Last2) & 3
	if t == 0 {
		goto match_end
	}
match_next:
	// fmt.Println("read append finale:", t)
	if err = in.ReadAppend(&out, t); err != nil {
		return
	}
	if ip, err = in.ReadByte(); err != nil {
		return
	}
	goto match

match_end:
	if ip, err = in.ReadByte(); err != nil {
		return
	}
	goto begin_loop
}
