//go:build windows

package instance

import "testing"

func TestAcquireIsExclusive(t *testing.T) {
	name := `Local\ECC-Test-AcquireIsExclusive`

	first, ok, err := Acquire(name)
	if err != nil || !ok {
		t.Fatalf("first Acquire: ok=%t err=%v", ok, err)
	}
	defer first.Release()

	second, ok, err := Acquire(name)
	if err != nil {
		t.Fatalf("second Acquire returned error: %v", err)
	}
	if ok {
		second.Release()
		t.Fatal("second Acquire should fail while first guard is held")
	}

	first.Release()

	third, ok, err := Acquire(name)
	if err != nil || !ok {
		t.Fatalf("Acquire after release: ok=%t err=%v", ok, err)
	}
	third.Release()
}
