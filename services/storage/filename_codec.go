// services/storage/filename_codec.go
// Turns one segment of an object key into a file name the host OS accepts, and back.
//
// Object keys are built from message IDs, and a TCP message ID contains the sender's address
// ("tcp_127.0.0.1:52244_<nanos>"). A colon is a normal file-name character on Linux and macOS but invalid on
// Windows, where writing such a file fails: on a Windows install the raw content of every TCP/MLLP message was
// silently never stored. The codec lets LocalDriver store those objects on Windows without changing anything on
// the platforms where the names were already valid (existing stores stay readable).

package storage

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// fileNameCodec maps a key segment (never contains "/") to a file name and back.
// Encode must be injective and Decode its exact inverse.
type fileNameCodec interface {
	Encode(segment string) string
	Decode(name string) string
}

// codecForOS returns the codec the given GOOS needs.
func codecForOS(goos string) fileNameCodec {
	if goos == "windows" {
		return windowsFileNames{}
	}
	return identityFileNames{}
}

// hostFileNames is the codec for the OS this binary runs on.
func hostFileNames() fileNameCodec { return codecForOS(runtime.GOOS) }

// identityFileNames leaves names as they are (Linux, macOS).
type identityFileNames struct{}

func (identityFileNames) Encode(segment string) string { return segment }
func (identityFileNames) Decode(name string) string    { return name }

// windowsFileNames percent-encodes what Windows file names cannot hold:
//
//	< > : " | ? * \  and control characters, plus "%" itself (so decoding is exact);
//	a trailing "." or space (Windows strips them, so two keys would collide);
//	a leading character of a reserved device name (CON, PRN, AUX, NUL, COM1-9, LPT1-9), with or without extension.
type windowsFileNames struct{}

const windowsUnsafe = `<>:"|?*\%`

func (windowsFileNames) Encode(segment string) string {
	if segment == "" {
		return segment
	}
	var b strings.Builder
	runes := []rune(segment)
	for i, r := range runes {
		trailing := i == len(runes)-1 && (r == '.' || r == ' ')
		deviceStart := i == 0 && isReservedDeviceName(segment)
		// Everything that needs escaping is ASCII, so wider characters (names with accents, CJK) pass through.
		if r < 0x20 || strings.ContainsRune(windowsUnsafe, r) || trailing || deviceStart {
			fmt.Fprintf(&b, "%%%02X", r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (windowsFileNames) Decode(name string) string {
	if !strings.Contains(name, "%") {
		return name
	}
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		if name[i] == '%' && i+2 < len(name) {
			if v, err := strconv.ParseUint(name[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(name[i])
	}
	return b.String()
}

// isReservedDeviceName reports whether a segment is a Windows device name, ignoring any extension and case.
func isReservedDeviceName(segment string) bool {
	base := segment
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(base) == 4 {
		up := strings.ToUpper(base)
		return (strings.HasPrefix(up, "COM") || strings.HasPrefix(up, "LPT")) && up[3] >= '1' && up[3] <= '9'
	}
	return false
}
