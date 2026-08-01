package live

import "testing"

func TestProtocolRejectsMalformedAndReservedFrames(t *testing.T) {
	if _, e := ParseClientFrame([]byte(`{"v":1,"type":"subscribe","channel":"_tinker"}`), 1024); e == nil {
		t.Fatal("reserved channel accepted")
	}
	if _, e := ParseClientFrame([]byte(`{"v":1,"type":"publish","channel":"x","event":"e","payload":`), 1024); e == nil {
		t.Fatal("malformed frame accepted")
	}
	if _, e := ParseClientFrame([]byte(`{"v":1,"type":"subscribe_kv","prefix":"x"}`), 1024); e != nil {
		t.Fatal(e)
	}
	if _, e := ParseClientFrame([]byte(`{"v":1,"type":"unsubscribe_kv","prefix":"x"}`), 1024); e != nil {
		t.Fatal(e)
	}
	if _, e := ParseClientFrame([]byte(`{"v":1,"type":"subscribe_collection","collection":"tasks"}`), 1024); e != nil {
		t.Fatal(e)
	}
	if _, e := ParseClientFrame([]byte(`{"v":1,"type":"subscribe_collection","collection":"2026-tasks"}`), 1024); e != nil {
		t.Fatalf("documented digit-leading collection was rejected: %v", e)
	}
	if _, e := ParseClientFrame([]byte(`{"v":1,"type":"unsubscribe_collection","collection":"tasks"}`), 1024); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{
		`{"v":1,"type":"subscribe_collection","collection":""}`,
		`{"v":1,"type":"subscribe_collection","collection":"Tasks"}`,
		`{"v":1,"type":"subscribe_collection","collection":"_private"}`,
	} {
		if _, e := ParseClientFrame([]byte(raw), 1024); e == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
