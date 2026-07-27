package live

import "testing"

func TestProtocolRejectsMalformedAndReservedFrames(t *testing.T) {
	if _, e := ParseClientFrame([]byte(`{"v":1,"type":"subscribe","channel":"_tiny"}`), 1024); e == nil {
		t.Fatal("reserved channel accepted")
	}
	if _, e := ParseClientFrame([]byte(`{"v":1,"type":"publish","channel":"x","event":"e","payload":`), 1024); e == nil {
		t.Fatal("malformed frame accepted")
	}
	if _, e := ParseClientFrame([]byte(`{"v":1,"type":"subscribe_kv","prefix":"x"}`), 1024); e != nil {
		t.Fatal(e)
	}
}
