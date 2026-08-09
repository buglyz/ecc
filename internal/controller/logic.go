package controller

import "math"

// InterpolateCurve 在 temperature→speed 曲线上线性插值。
//
// 调用方保证（config.Normalize 的 normalizeCurve 恒定满足）：len 恒为 CurvePoints、
// 温度严格递增。该不变量下走零分配快路径；非此形态的输入（如单元测试里的畸形
// 曲线）退回复制+排序的防御路径，行为与旧实现逐位一致。
// 需先验证不变量时用 curveSorted；快路径直接复用入参 slice，不做防御拷贝。
func InterpolateCurve(curve []Point, temp float64) float64 {
	if len(curve) == 0 {
		return 0
	}
	points := curve
	// 快路径：温度严格递增。归一化后的曲线必满足；不满足走防御路径。
	// 严格递增隐含已排序，且别掉进 t2==t1 的退化分支。
	if len(points) < 2 {
		return points[0].Speed()
	}
	sorted := true
	for i := 1; i < len(points); i++ {
		if points[i].Temp() <= points[i-1].Temp() {
			sorted = false
			break
		}
	}
	if !sorted {
		points = append([]Point(nil), curve...)
		sortAscending(points)
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

func sortAscending(points []Point) {
	for i := 1; i < len(points); i++ {
		for j := i; j > 0 && points[j].Temp() < points[j-1].Temp(); j-- {
			points[j], points[j-1] = points[j-1], points[j]
		}
	}
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
