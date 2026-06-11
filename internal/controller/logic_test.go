package controller

import "testing"

func TestInterpolateCurveMatchesPythonBehavior(t *testing.T) {
	curve := []Point{{40, 30}, {55, 40}, {70, 60}, {80, 85}, {90, 100}}
	cases := []struct {
		temp float64
		want float64
	}{
		{35, 30},
		{40, 30},
		{47.5, 35},
		{70, 60},
		{75, 72.5},
		{95, 100},
	}
	for _, tc := range cases {
		if got := InterpolateCurve(curve, tc.temp); got != tc.want {
			t.Fatalf("InterpolateCurve(%v)=%v, want %v", tc.temp, got, tc.want)
		}
	}
}

func TestCombineTempsStrategies(t *testing.T) {
	cpu, gpu := 70.0, 50.0
	cases := []struct {
		strategy string
		want     float64
	}{
		{"weighted", 64},
		{"max", 70},
		{"cpu", 70},
		{"gpu", 50},
		{"unknown", 64},
	}
	for _, tc := range cases {
		got := CombineTemps(tc.strategy, &cpu, &gpu)
		if got == nil || *got != tc.want {
			t.Fatalf("CombineTemps(%s)=%v, want %v", tc.strategy, got, tc.want)
		}
	}
	if got := CombineTemps("weighted", nil, nil); got != nil {
		t.Fatalf("CombineTemps(nil,nil)=%v, want nil", *got)
	}
}

func TestPointRejectsWrongArity(t *testing.T) {
	var p Point
	if err := p.UnmarshalJSON([]byte(`[40]`)); err == nil {
		t.Fatal("expected invalid curve point arity to fail")
	}
}
