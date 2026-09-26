// Package strictjson validates untrusted JSON before JCS canonicalization.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	jcs "github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"io"
	"strconv"
	"unicode/utf8"
)

const MaxDepth = 16

var ErrInvalid = errors.New("invalid JSON: encoding, duplicate field, depth, size or value")

// Canonical returns UTF-8 RFC 8785 bytes, without BOM or trailing newline.
func Canonical(data []byte, limit int) ([]byte, error) {
	if len(data) == 0 || len(data) > limit || !utf8.Valid(data) || !validEscapes(data) {
		return nil, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := value(d, 0); err != nil {
		return nil, ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	// The library accepts container roots; wrapping also supports RFC scalar roots.
	wrapped := append([]byte{'['}, data...)
	wrapped = append(wrapped, ']')
	out, err := jcs.Transform(wrapped)
	if err != nil {
		return nil, ErrInvalid
	}
	return out[1 : len(out)-1], nil
}
func Decode(data []byte, limit int, target any) error {
	if _, err := Canonical(data, limit); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return ErrInvalid
	}
	return nil
}
func value(d *json.Decoder, depth int) error {
	if depth > MaxDepth {
		return ErrInvalid
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); ok {
		switch delim {
		case '{':
			keys := map[string]bool{}
			for d.More() {
				t, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := t.(string)
				if !ok || keys[key] {
					return ErrInvalid
				}
				keys[key] = true
				if err := value(d, depth+1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := value(d, depth+1); err != nil {
					return err
				}
			}
		default:
			return ErrInvalid
		}
		_, err = d.Token()
	}
	return err
}

// Reject escaped lone surrogates before encoding/json replaces them by U+FFFD.
func validEscapes(b []byte) bool {
	inString := false
	for i := 0; i < len(b); i++ {
		if b[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || b[i] != 0x5c {
			continue
		}
		i++
		if i >= len(b) {
			return false
		}
		if b[i] != 'u' {
			continue
		}
		if i+4 >= len(b) {
			return false
		}
		n, err := strconv.ParseUint(string(b[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(b) || b[i+1] != 0x5c || b[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(b[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return !inString
}
