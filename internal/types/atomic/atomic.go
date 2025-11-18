package atomic

import (
	"encoding/json"
	syncatomic "sync/atomic"
)

type Uint64 struct {
	syncatomic.Uint64
}

func (u *Uint64) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.Load())
}

func (u *Uint64) UnmarshalJSON(data []byte) error {
	var v uint64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	u.Store(v)
	return nil
}
