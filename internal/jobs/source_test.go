package jobs

import (
	"strings"
	"testing"
)

func TestClassifyFailedSelectAndPayload(t *testing.T) {
	if classify(nil, 2, 1) != "delayed" || classify(nil, 1, 1) != "ready" {
		t.Fatal("classify")
	}
	r := int64(1)
	if classify(&r, 1, 2) != "reserved" {
		t.Fatal("reserved")
	}
	if got := buildFailedSelect(map[string]bool{"id": true, "uuid": true}); !strings.Contains(got, "id,uuid") {
		t.Fatal(got)
	}
	if _, e := DecodePayload("bad"); e == nil {
		t.Fatal("bad payload accepted")
	}
	if p, e := DecodePayload(`{"displayName":"Job"}`); e != nil || p.DisplayName != "Job" {
		t.Fatalf("payload=%#v err=%v", p, e)
	}
}
