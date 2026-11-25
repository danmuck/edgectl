package activity

import (
	"strconv"
	"testing"
)

// Oldest-first order, kind filter, limit, the retention bound and copy-on-read.
func TestRecentOrderFilterLimitAndBound(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	for i := 0; i < Capacity+5; i++ {
		kind := KindAudit
		if i%2 == 1 {
			kind = KindCustody
		}
		Add(kind, uint64(i), map[string]string{"n": strconv.Itoa(i)})
	}
	all := Recent(0, "")
	if len(all) != Capacity || all[0].TimestampMS != 5 || all[len(all)-1].TimestampMS != Capacity+4 {
		t.Fatalf("bound/order: len=%d first=%d last=%d", len(all), all[0].TimestampMS, all[len(all)-1].TimestampMS)
	}
	last := Recent(3, KindCustody)
	if len(last) != 3 || last[2].TimestampMS != Capacity+3 || last[0].Kind != KindCustody {
		t.Fatalf("filter/limit: %+v", last)
	}
	last[2].Fields["n"] = "changed"
	if again := Recent(1, KindCustody); again[0].Fields["n"] == "changed" {
		t.Fatalf("Recent must return copies")
	}
}
