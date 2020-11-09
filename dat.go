package lzo

// Flags. Poorly documented, sometimes different implementations
// ascribe different meaning.
const (
	// The names are a guess; the original names in code are not helpful.
	fDataAdler     = 1
	fChecksumAdler = 2
	fExtraField    = 0x40
	fGMTDiff       = 0x80
	fDataCRC32     = 0x100
	fChecksumCRC32 = 0x200
	// no idea where these are documented.
	fHFilter = 0x800
	fHCRC32  = 0x1000
	fMASK    = 0x3fff
	fUnix    = 0x03000000
	fOSShift = 24
	fOSMask  = 0xff << fOSShift

	// character set for file name encoding [mostly unused]
	fCharSetNative = 0x00000000
	fCharSetShift  = 20
	fCharSetMask   = 0xf << fCharSetShift
)

// The LZOP program has a header:
// This doc looks wrong. Below is from source.
//Magic: 9 bytes (0x89, 0x4c, 0x5a, 0x4f, 0x00, 0x0d, 0x0a, 0x1a, 0x0a)
//Version: 2 bytes
//Lib version: 2 bytes
//Version needed: 2 bytes
//Method: 1 byte
//Level: 1 byte
//Flags: 4 byte
//Filter: 4 bytes (only if flags & F_H_FILTER)
//Mode: 4 bytes
//Mtime: 4 bytes
//GMTdiff: 4 bytes
//File name length: 1 byte
//Name: 0-255 bytes
//Checksum, original data: 4 bytes (CRC32 if flags & F_H_CRC32 else Adler32)
//Uncompressed size: 4 bytes
//Compressed size: 4 bytes
//Checksum, uncompressed data: 4 bytes
//Checksum, compressed data: 4 bytes (only if flags & F_ADLER32_C or flags & F_CRC32_C)
//
// The package assumes no header. Oops.

const magic = "\x89\x4c\x5a\x4f\x00\x0d\x0a\x1a\x0a"

// Header is the unmarshal'ed LZOP file header.
// This does not really match either docs or code we have
// found. Consider it advisory, and we need to work out its
// real structure. Further, like so much compression code,
// some members are optional, depending on flags in Flags,
// but they are in the middle (!), e.g. Filter.
type Header struct {
	Magic            [9]uint8
	Version          uint16
	LibVersion       uint16
	NeededVersion    uint16
	Method           byte
	Level            byte
	Flags            uint32
	Filter           uint32
	Mode             uint32
	Mtime            uint32
	GMTDiff          uint32
	Name             string
	DataCSUM         uint32
	UnCompressedSize uint32
	CompressedSize   uint32
	CompressedCSUM   uint32
	UncompressedCSUM uint32
}
