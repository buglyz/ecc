package ui

import (
	"testing"

	"github.com/buglyz/ecc/internal/controller"
)

func TestValidateCurve(t *testing.T) {
	valid := []controller.Point{
		{40, 30},
		{55, 40},
		{70, 60},
		{80, 85},
		{90, 100},
	}
	if err := ValidateCurve(valid); err != nil {
		t.Fatalf("expected valid curve, got error: %v", err)
	}

	invalidLen := []controller.Point{
		{40, 30},
		{55, 40},
	}
	if err := ValidateCurve(invalidLen); err == nil {
		t.Fatal("expected error for invalid curve length")
	}

	invalidTempOrder := []controller.Point{
		{40, 30},
		{35, 40},
		{70, 60},
		{80, 85},
		{90, 100},
	}
	if err := ValidateCurve(invalidTempOrder); err == nil {
		t.Fatal("expected error for non-increasing temperature")
	}

	outOfRangeSpeed := []controller.Point{
		{40, 30},
		{55, 120},
		{70, 60},
		{80, 85},
		{90, 100},
	}
	if err := ValidateCurve(outOfRangeSpeed); err == nil {
		t.Fatal("expected error for speed > 100")
	}
}

func TestFormatHelpers(t *testing.T) {
	var temp float64 = 65.4
	if s := formatTemp(&temp); s != "65.4°C" {
		t.Fatalf("expected 65.4°C, got %s", s)
	}
	if s := formatTemp(nil); s != "--" {
		t.Fatalf("expected --, got %s", s)
	}

	var speed int = 50
	if s := formatSpeed(&speed); s != "50%" {
		t.Fatalf("expected 50%%, got %s", s)
	}
	if s := formatSpeed(nil); s != "--" {
		t.Fatalf("expected --, got %s", s)
	}

	var rpm uint16 = 2500
	if s := formatRPM(&rpm); s != "实际: 2500 RPM" {
		t.Fatalf("expected 实际: 2500 RPM, got %s", s)
	}
	if s := formatRPM(nil); s != "RPM: 不可用" {
		t.Fatalf("expected RPM: 不可用, got %s", s)
	}
}
