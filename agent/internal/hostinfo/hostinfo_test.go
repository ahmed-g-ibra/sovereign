package hostinfo

import (
	"runtime"
	"testing"
)

func TestLoad1(t *testing.T) {
	v, ok := Load1()
	switch runtime.GOOS {
	case "linux", "darwin":
		if !ok {
			t.Fatalf("Load1 unavailable on %s, where it should work", runtime.GOOS)
		}
		if v < 0 || v > 1024 {
			t.Errorf("load average %v is not plausible", v)
		}
	default:
		if ok {
			t.Errorf("unexpected implementation on %s", runtime.GOOS)
		}
	}
}
