package releases

import "testing"

func FuzzParseManifest(f *testing.F) {
	f.Add([]byte("version: 1\nname: demo\n"))
	f.Add([]byte("access:\n  mode: public"))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = ParseManifest(b) })
}
