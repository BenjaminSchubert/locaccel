package units

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"syscall"

	"gopkg.in/yaml.v3"
)

var (
	percentFormat         = regexp.MustCompile(`^(\d+(\.\d+)?)\s*%$`)
	ErrInvalidQuotaFormat = errors.New(
		"not a valid quota. Must match " + percentFormat.String() + " or " + bytesFormat.String(),
	)
)

type DiskQuota struct {
	bytes   Bytes
	percent float64
}

func NewDiskQuotaInBytes(bytes Bytes) DiskQuota {
	return DiskQuota{bytes, 0}
}

func NewDiskQuotaInPercent(percent float64) DiskQuota {
	return DiskQuota{Bytes{}, percent}
}

func (d *DiskQuota) UnmarshalYAML(value *yaml.Node) error {
	val, err := DecodeBytes(value.Value)
	if err == nil {
		d.bytes = val
		d.percent = 0
		return nil
	}

	groups := percentFormat.FindStringSubmatch(value.Value)
	if groups == nil {
		return ErrInvalidQuotaFormat
	}

	valf, err := strconv.ParseFloat(groups[1], 64)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidQuotaFormat, err)
	}

	d.bytes = Bytes{}
	d.percent = valf
	return nil
}

func (d DiskQuota) MarshalYAML() (any, error) {
	if d.percent == 0.0 {
		return d.bytes.String(), nil
	}
	return fmt.Sprintf("%.f%%", d.percent), nil
}

func (d DiskQuota) Bytes(path string) (Bytes, error) {
	if d.percent != 0.0 {
		fs := syscall.Statfs_t{}
		if err := syscall.Statfs(path, &fs); err != nil {
			return Bytes{}, err
		}

		// uint64 -> int64 might overflow, which will get negative bytes.
		return Bytes{int64(fs.Blocks) * fs.Bsize}, nil //nolint:gosec
	}

	return d.bytes, nil
}
