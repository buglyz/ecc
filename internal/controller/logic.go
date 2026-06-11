package controller

import (
	"math"
	"sort"
)

func InterpolateCurve(curve []Point, temp float64) float64 {
	points := append([]Point(nil), curve...)
	sort.Slice(points, func(i, j int) bool { return points[i].Temp() < points[j].Temp() })
	if len(points) == 0 {
		return 0
	}
	if temp <= points[0].Temp() {
		return points[0].Speed()
	}
	if temp >= points[len(points)-1].Temp() {
		return points[len(points)-1].Speed()
	}
	for i := 0; i < len(points)-1; i++ {
		t1, s1 := points[i].Temp(), points[i].Speed()
		t2, s2 := points[i+1].Temp(), points[i+1].Speed()
		if t1 <= temp && temp <= t2 {
			if t2 == t1 {
				return s2
			}
			return s1 + (temp-t1)/(t2-t1)*(s2-s1)
		}
	}
	return points[len(points)-1].Speed()
}

func CombineTemps(strategy string, cpu, gpu *float64) *float64 {
	if cpu == nil && gpu == nil {
		return nil
	}
	switch strategy {
	case "cpu":
		if cpu != nil {
			return copyFloat(cpu)
		}
		return copyFloat(gpu)
	case "gpu":
		if gpu != nil {
			return copyFloat(gpu)
		}
		return copyFloat(cpu)
	case "max":
		if cpu == nil {
			return copyFloat(gpu)
		}
		if gpu == nil {
			return copyFloat(cpu)
		}
		v := math.Max(*cpu, *gpu)
		return &v
	default:
		if cpu == nil {
			return copyFloat(gpu)
		}
		if gpu == nil {
			return copyFloat(cpu)
		}
		v := (*cpu-*gpu)*CPUWeight + *gpu
		return &v
	}
}

func ClampSpeed(speed float64) int {
	return int(math.Max(CurveSpeedMin, math.Min(CurveSpeedMax, math.Round(speed))))
}

func ValidStrategy(strategy string) bool {
	for _, item := range Strategies {
		if item.Key == strategy {
			return true
		}
	}
	return false
}

func copyFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}
