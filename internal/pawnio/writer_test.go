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

type dutyFakeEC struct {
	read   byte
	duty   byte
	writes int
}

func (f *dutyFakeEC) ReadEC(_ context.Context, _ byte) (byte, error) { return f.read, nil }
func (f *dutyFakeEC) WriteEC(_ context.Context, _, _ byte) error     { f.writes++; return nil }
func (f *dutyFakeEC) ReadDuty(_ context.Context, _ byte) (byte, error) {
	return f.duty, nil
}
func (f *dutyFakeEC) Close() error { return nil }

func TestWriterAcceptsInvalidReadbackWhenDutyMatches(t *testing.T) {
	dev := &dutyFakeEC{read: 0x0B, duty: 0x5A}
	w := NewWriter(dev, log.New(io.Discard, "", 0))
	if !w.Write(context.Background(), "0x2C", "0x5A") {
		t.Fatal("Write should accept when actual duty matches target")
	}
	if dev.writes != 1 {
		t.Fatalf("writes=%d, want 1 (no unnecessary rewrite)", dev.writes)
	}
}

func TestWriterRejectsInvalidReadbackWhenDutyDiffers(t *testing.T) {
	dev := &dutyFakeEC{read: 0x0B, duty: 0x20}
	w := NewWriter(dev, log.New(io.Discard, "", 0))
	if w.Write(context.Background(), "0x2C", "0x5A") {
		t.Fatal("Write should reject when actual duty is far from target")
	}
	if dev.writes != 2 {
		t.Fatalf("writes=%d, want 2 (one rewrite)", dev.writes)
	}
}

func TestWriterDoesNotUseDutyForRelease(t *testing.T) {
	dev := &dutyFakeEC{read: 0x0B, duty: 0x64}
	w := NewWriter(dev, log.New(io.Discard, "", 0))
	if w.Write(context.Background(), "0x2C", "0xFF") {
		t.Fatal("release must require exact 0xFF readback")
	}
}
