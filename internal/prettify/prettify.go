package prettify

import "fmt"

var sizes = []byte("BKMGT")

func Bytes(value int64) string {
	base := 1024.0
	i := 0
	b := float64(value)

	for b >= base && i < len(sizes) {
		b /= base
		i++
	}

	if i == 0 {
		return fmt.Sprintf("%dB", value)
	}
	return fmt.Sprintf("%.2f%ciB", b, sizes[i])
}
