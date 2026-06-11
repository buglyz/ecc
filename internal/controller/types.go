package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type Point [2]float64

func (p Point) Temp() float64 {
	return p[0]
}

func (p Point) Speed() float64 {
	return p[1]
}

func (p *Point) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 2 {
		return fmt.Errorf("curve point must have exactly 2 values")
	}
	temp, err := numberFromJSON(raw[0])
	if err != nil {
		return err
	}
	speed, err := numberFromJSON(raw[1])
	if err != nil {
		return err
	}
	*p = Point{temp, speed}
	return nil
}

func numberFromJSON(data json.RawMessage) (float64, error) {
	var number float64
	if err := json.Unmarshal(data, &number); err == nil {
		return number, nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(text, 64)
}

type Strategy struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type Preset struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Curve    []Point `json:"curve"`
	Strategy string  `json:"strategy"`
}

type Temps struct {
	CPU *float64 `json:"cpu"`
	GPU *float64 `json:"gpu"`
}

type Latest struct {
	CPU         *float64  `json:"cpu"`
	GPU         *float64  `json:"gpu"`
	TargetTemp  *float64  `json:"target_temp"`
	Speed       *int      `json:"speed"`
	Mode        string    `json:"mode"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastECWrite time.Time `json:"last_ec_write"`
}

type HistorySample struct {
	Time       time.Time `json:"time"`
	CPU        *float64  `json:"cpu"`
	GPU        *float64  `json:"gpu"`
	TargetTemp *float64  `json:"target_temp"`
	Speed      int       `json:"speed"`
}

type SensorReader interface {
	Read() Temps
	Close() error
}

type FanWriter interface {
	Write(ctx context.Context, register string, valueHex string) bool
}
