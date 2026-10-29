package pawnio

import (
	"context"
	"io"
	"log"
	"testing"
)

type fakeEC struct {
	regs        map[byte]byte
	badReadback byte
}

func (f *fakeEC) ReadEC(_ context.Context, reg byte) (byte, error) {
	if f.badReadback != 0 && reg == 0x2C {
		return f.badReadback, nil
	}
	return f.regs[reg], nil
}

func (f *fakeEC) WriteEC(_ context.Context, reg, val byte) error {
	f.regs[reg] = val
	return nil
}

func (f *fakeEC) Close() error { return nil }

func TestWriterWritesAndVerifies(t *testing.T) {
	dev := &fakeEC{regs: map[byte]byte{0x2C: 0x42}}
	w := NewWriter(dev, log.New(io.Discard, "", 0))

	if !w.Write(context.Background(), "0x2C", "0x64") {
		t.Fatal("Write returned false, want true")
	}
	if dev.regs[0x2C] != 0x64 {
		t.Fatalf("reg 0x2C=%#x, want 0x64", dev.regs[0x2C])
	}
}

func TestWriterRejectsBadReadback(t *testing.T) {
	dev := &fakeEC{regs: map[byte]byte{0x2C: 0x42}, badReadback: 0x42}
	w := NewWriter(dev, log.New(io.Discard, "", 0))

	if w.Write(context.Background(), "0x2C", "0x64") {
		t.Fatal("Write returned true for mismatched readback")
	}
}

func TestWriterRejectsInvalidInput(t *testing.T) {
	w := NewWriter(&fakeEC{regs: map[byte]byte{}}, log.New(io.Discard, "", 0))
	if w.Write(context.Background(), "nope", "0x64") {
		t.Fatal("Write accepted invalid register")
	}
	if w.Write(context.Background(), "0x2C", "0x100") {
		t.Fatal("Write accepted out-of-range value")
	}
}
