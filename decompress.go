package lzo

import (
	"errors"
	"io"
)

var (
	InputUnderrun      = errors.New("input underrun")
	LookBehindUnderrun = errors.New("lookbehind underrun")
)

var last1, last2 byte

func readAppend(in io.Reader, out *[]byte, n int) (err error) {
	sz := len(*out)
	*out = append(*out, make([]byte, n)...)
	_, err = io.ReadFull(in, (*out)[sz:])
	return
}

func read8(in io.Reader) (b byte, err error) {
	var buf [1]byte
	_, err = io.ReadFull(in, buf[:])
	b = buf[0]
	last2 = last1
	last1 = b
	return
}

func read16(in io.Reader) (b int, err error) {
	var buf [2]byte
	_, err = io.ReadFull(in, buf[:])
	b = int(buf[0]) + int(buf[1])*256
	last2 = buf[0]
	last1 = buf[1]
	return
}

func readMulti(in io.Reader, base int) (b int, err error) {
	var v byte
	for {
		if v, err = read8(in); err != nil {
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

// Decompress an input compress with LZO1X
func Decompress1X(in io.Reader, in_len int) (out []byte, err error) {
	var ip byte
	var t, m_pos int
	out = make([]byte, 0, in_len)

	if ip, err = read8(in); err != nil {
		return
	}
	if ip > 17 {
		t = int(ip) - 17
		if t < 4 {
			goto match_next
		}
		if err = readAppend(in, &out, t); err != nil {
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
		if t, err = readMulti(in, 15); err != nil {
			return
		}
	}
	if err = readAppend(in, &out, t+3); err != nil {
		return
	}
	// fmt.Println("readappend", t+3, string(out[len(out)-t-3:]))
first_literal_run:
	if ip, err = read8(in); err != nil {
		return
	}
	t = int(ip)
	if t >= 16 {
		goto match
	}
	m_pos = len(out) - (1 + m2_MAX_OFFSET)
	m_pos -= t >> 2
	if ip, err = read8(in); err != nil {
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
		if ip, err = read8(in); err != nil {
			return
		}
		m_pos -= int(ip) << 3
		// fmt.Println("m_pos t64", m_pos, t, int(ip))
		t = (t >> 5) - 1
		goto copy_match
	} else if t >= 32 {
		t &= 31
		if t == 0 {
			if t, err = readMulti(in, 31); err != nil {
				return
			}
		}
		m_pos = len(out) - 1
		var v16 int
		if v16, err = read16(in); err != nil {
			return
		}
		m_pos -= v16 >> 2
		// fmt.Println("m_pos t32", m_pos)
	} else if t >= 16 {
		m_pos = len(out)
		m_pos -= (t & 8) << 11
		t &= 7
		if t == 0 {
			if t, err = readMulti(in, 7); err != nil {
				return
			}
		}
		var v16 int
		if v16, err = read16(in); err != nil {
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
		if ip, err = read8(in); err != nil {
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
	t = int(last2) & 3
	if t == 0 {
		goto match_end
	}
match_next:
	// fmt.Println("read append finale:", t)
	if err = readAppend(in, &out, t); err != nil {
		return
	}
	if ip, err = read8(in); err != nil {
		return
	}
	goto match

match_end:
	if ip, err = read8(in); err != nil {
		return
	}
	goto begin_loop
}
