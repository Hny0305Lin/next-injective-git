package chain

import (
	"fmt"
	"strings"
)

const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

func decodeBech32Address(value string) ([]byte, error) {
	if value == "" || strings.ToLower(value) != value {
		return nil, fmt.Errorf("invalid bech32 address %q", value)
	}
	separator := strings.LastIndexByte(value, '1')
	if separator <= 0 || separator+7 > len(value) {
		return nil, fmt.Errorf("invalid bech32 address %q", value)
	}
	prefix := value[:separator]
	if prefix != "inj" {
		return nil, fmt.Errorf("unsupported bech32 address prefix %q", prefix)
	}
	encoded := value[separator+1:]
	values := make([]byte, len(encoded))
	for i, char := range encoded {
		index := strings.IndexByte(bech32Charset, byte(char))
		if index < 0 {
			return nil, fmt.Errorf("invalid bech32 character %q", char)
		}
		values[i] = byte(index)
	}
	if !bech32VerifyChecksum(prefix, values) {
		return nil, fmt.Errorf("invalid bech32 checksum for %q", value)
	}
	converted, err := bech32ConvertBits(values[:len(values)-6], 5, 8, false)
	if err != nil {
		return nil, err
	}
	if len(converted) != 20 {
		return nil, fmt.Errorf("bech32 address payload has %d bytes, want 20", len(converted))
	}
	return converted, nil
}

func encodeBech32Address(prefix string, payload []byte) string {
	converted, err := bech32ConvertBits(payload, 8, 5, true)
	if err != nil {
		return ""
	}
	values := append(converted, bech32Checksum(prefix, converted)...)
	var builder strings.Builder
	builder.WriteString(prefix)
	builder.WriteByte('1')
	for _, value := range values {
		builder.WriteByte(bech32Charset[value])
	}
	return builder.String()
}

func bech32HrpExpand(prefix string) []byte {
	values := make([]byte, 0, len(prefix)*2+1)
	for _, char := range prefix {
		values = append(values, byte(char>>5))
	}
	values = append(values, 0)
	for _, char := range prefix {
		values = append(values, byte(char&31))
	}
	return values
}

func bech32Polymod(values []byte) uint32 {
	generators := [...]uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	var checksum uint32 = 1
	for _, value := range values {
		top := checksum >> 25
		checksum = (checksum&0x1ffffff)<<5 ^ uint32(value)
		for i := 0; i < 5; i++ {
			if (top>>i)&1 != 0 {
				checksum ^= generators[i]
			}
		}
	}
	return checksum
}

func bech32Checksum(prefix string, values []byte) []byte {
	input := append(bech32HrpExpand(prefix), values...)
	input = append(input, make([]byte, 6)...)
	mod := bech32Polymod(input) ^ 1
	checksum := make([]byte, 6)
	for i := range checksum {
		checksum[i] = byte((mod >> (5 * (5 - i))) & 31)
	}
	return checksum
}

func bech32VerifyChecksum(prefix string, values []byte) bool {
	return bech32Polymod(append(bech32HrpExpand(prefix), values...)) == 1
}

func bech32ConvertBits(data []byte, fromBits, toBits uint, pad bool) ([]byte, error) {
	if fromBits == 0 || toBits == 0 || fromBits > 8 || toBits > 8 {
		return nil, fmt.Errorf("invalid bech32 bit conversion %d -> %d", fromBits, toBits)
	}
	maxValue := byte((1 << toBits) - 1)
	accumulator := uint(0)
	bits := uint(0)
	var result []byte
	for _, value := range data {
		if value>>fromBits != 0 {
			return nil, fmt.Errorf("bech32 value %d exceeds %d bits", value, fromBits)
		}
		accumulator = (accumulator << fromBits) | uint(value)
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			result = append(result, byte((accumulator>>bits)&uint(maxValue)))
		}
	}
	if pad {
		if bits > 0 {
			result = append(result, byte((accumulator<<(toBits-bits))&uint(maxValue)))
		}
	} else {
		if bits >= fromBits {
			return nil, fmt.Errorf("invalid bech32 padding")
		}
		if ((accumulator << (toBits - bits)) & uint(maxValue)) != 0 {
			return nil, fmt.Errorf("non-zero bech32 padding")
		}
	}
	return result, nil
}
