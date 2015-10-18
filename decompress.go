package lzo

import (
	"errors"
	"io"
)

var (
	InputUnderrun      = errors.New("input underrun")
	LookBehindUnderrun = errors.New("lookbehind underrun")
)

type reader struct {
	r   io.Reader
	len int
	buf [4096]byte
	cur []byte
	Err error
}

func newReader(r io.Reader, inlen int) *reader {
	return &reader{r: r, len: inlen}
}

func (in *reader) Rebuffer0() {
	in.cur = in.buf[:]
	cur := in.cur[:]
	if len(cur) > in.len {
		cur = cur[:in.len]
	}
	n, err := in.r.Read(cur)
	if err != nil {
		in.Err = err
	} else {
		in.cur = in.cur[:n]
		in.len -= n
	}
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
	if len(cur) > in.len {
		cur = cur[:in.len]
	}
	n, err := in.r.Read(cur)
	if err != nil {
		in.Err = err
		in.cur[0] = 0xFF // break out of multi loop, if any
	} else {
		in.cur = in.cur[:len(rb)+n]
		in.len -= n
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
			in.Rebuffer0()
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
		in.Rebuffer0()
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
	var t, m_pos int
	var last2 byte

	out = make([]byte, 0, out_len)

	in := newReader(r, in_len)
	in.Rebuffer()
	ip := in.ReadU8()
	if ip > 17 {
		t = int(ip) - 17
		if t < 4 {
			goto match_next
		}
		in.ReadAppend(&out, t)
		// fmt.Println("begin:", string(out))
		goto first_literal_run
	}

begin_loop:
	if in.Err != nil {
		err = in.Err
		return
	}
	t = int(ip)
	if t >= 16 {
		goto match
	}
	if t == 0 {
		t = in.ReadMulti(15)
	}
	in.ReadAppend(&out, t+3)
	// fmt.Println("readappend", t+3, string(out[len(out)-t-3:]))
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
	// fmt.Println("m_pos flr", m_pos, len(out), "\n", string(out))
	if m_pos < 0 {
		err = LookBehindUnderrun
		return
	}
	copyMatch(&out, m_pos, 3)
	goto match_done

match:
	in.Rebuffer()
	t = int(ip)
	last2 = ip
	if t >= 64 {
		m_pos = len(out) - 1
		m_pos -= (t >> 2) & 7
		ip = in.ReadU8()
		m_pos -= int(ip) << 3
		// fmt.Println("m_pos t64", m_pos, t, int(ip))
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
		// fmt.Println("m_pos t32", m_pos)
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
			// fmt.Println("FINEEEEE", t, v16, m_pos)
			return
		}
		m_pos -= 0x4000
		last2 = byte(v16 & 0xFF)
		// fmt.Println("m_pos t16", m_pos)
	} else {
		m_pos = len(out) - 1
		m_pos -= t >> 2
		ip = in.ReadU8()
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
	t = int(last2 & 3)
	if t == 0 {
		goto match_end
	}
match_next:
	// fmt.Println("read append finale:", t)
	in.ReadAppend(&out, t)
	ip = in.ReadU8()
	goto match

match_end:
	ip = in.ReadU8()
	goto begin_loop
}
