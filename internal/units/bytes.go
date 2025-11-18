package units

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

var (
	bytesFormat          = regexp.MustCompile(`^(\d+(\.\d+)?)\s*((([KMGT])(i?))?B)?$`)
	units                = []byte("BKMGT")
	ErrInvalidByteFormat = errors.New("not a valid bytes format. Must match" + bytesFormat.String())
)

type Bytes struct {
	Bytes int64
}

func (b *Bytes) UnmarshalYAML(value *yaml.Node) error {
	val, err := DecodeBytes(value.Value)
	if err != nil {
		return err
	}

	*b = val
	return nil
}

func DecodeBytes(value string) (Bytes, error) {
	groups := bytesFormat.FindStringSubmatch(value)
	if groups == nil {
		return Bytes{}, ErrInvalidByteFormat
	}

	// No unit
	if groups[3] == "" {
		val, err := strconv.Atoi(groups[1])
		if err != nil {
			return Bytes{}, fmt.Errorf("%s: %w", ErrInvalidByteFormat, err)
		}

		return Bytes{int64(val)}, nil
	}

	base := 1024
	if groups[6] == "" {
		base = 1000
	}

	exponent := bytes.IndexByte(units, []byte(groups[3])[0])

	mul := 1
	for range exponent {
		mul *= base
	}

	val, err := strconv.Atoi(groups[1])
	if err == nil {
		return Bytes{int64(val * mul)}, nil
	}
	valf, err := strconv.ParseFloat(groups[1], 64)
	if err == nil {
		return Bytes{int64(valf * float64(mul))}, nil
	}

	return Bytes{}, fmt.Errorf("%s: %w", ErrInvalidByteFormat, err)
}

func (b Bytes) String() string {
	return PrettyBytes(b.Bytes)
}

func PrettyBytes[T int64 | uint64](b T) string {
	base := 1024.0
	i := 0
	v := float64(b)

	for v >= base && i < len(units) {
		v /= base
		i++
	}

	if i == 0 {
		return fmt.Sprintf("%dB", b)
	}
	return fmt.Sprintf("%.2f%ciB", v, units[i])
}
