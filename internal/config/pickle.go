package config

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/buglyz/ecc/internal/controller"
)

type pickleParser struct {
	data  []byte
	pos   int
	stack []any
	marks []int
	memo  map[int]any
}

func parsePickleDict(data []byte) (out map[string]any, err error) {
	// 截断/损坏的 pickle 可能让某些 opcode 处理器读越界（readByte/stack 切片）。
	// 这里兜底 recover，把 panic 转成普通 error，避免启动时 config.Load 崩溃整程序。
	defer func() {
		if r := recover(); r != nil {
			out = nil
			err = fmt.Errorf("pickle parse panic: %v", r)
		}
	}()
	p := &pickleParser{data: data, memo: map[int]any{}}
	value, err := p.parse()
	if err != nil {
		return nil, err
	}
	out, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("pickle root is not dict")
	}
	return out, nil
}

func (p *pickleParser) parse() (any, error) {
	for p.pos < len(p.data) {
		op := p.readByte()
		switch op {
		case 0x80:
			p.pos++
		case 0x95:
			if p.pos+8 > len(p.data) {
				return nil, errors.New("short FRAME")
			}
			p.pos += 8
		case 0x94:
			if len(p.stack) > 0 {
				p.memo[len(p.memo)] = p.stack[len(p.stack)-1]
			}
		case '}':
			p.stack = append(p.stack, map[string]any{})
		case ']':
			p.stack = append(p.stack, []any{})
		case '(':
			p.marks = append(p.marks, len(p.stack))
		case 'e':
			if err := p.appendItems(); err != nil {
				return nil, err
			}
		case 'u':
			if err := p.setItems(); err != nil {
				return nil, err
			}
		case 's':
			if err := p.setItem(); err != nil {
				return nil, err
			}
		case 'K':
			p.stack = append(p.stack, int(p.readByte()))
		case 'J':
			if p.pos+4 > len(p.data) {
				return nil, errors.New("short BININT")
			}
			value := int(int32(binary.LittleEndian.Uint32(p.data[p.pos : p.pos+4])))
			p.pos += 4
			p.stack = append(p.stack, value)
		case 'M':
			if p.pos+2 > len(p.data) {
				return nil, errors.New("short BININT2")
			}
			value := int(binary.LittleEndian.Uint16(p.data[p.pos : p.pos+2]))
			p.pos += 2
			p.stack = append(p.stack, value)
		case 'G':
			if p.pos+8 > len(p.data) {
				return nil, errors.New("short BINFLOAT")
			}
			bits := binary.BigEndian.Uint64(p.data[p.pos : p.pos+8])
			p.pos += 8
			p.stack = append(p.stack, math.Float64frombits(bits))
		case 'N':
			p.stack = append(p.stack, nil)
		case ')':
			p.stack = append(p.stack, []any{})
		case 0x85:
			if len(p.stack) < 1 {
				return nil, errors.New("TUPLE1 needs one item")
			}
			item := p.stack[len(p.stack)-1]
			p.stack[len(p.stack)-1] = []any{item}
		case 0x86:
			if len(p.stack) < 2 {
				return nil, errors.New("TUPLE2 needs two items")
			}
			items := []any{p.stack[len(p.stack)-2], p.stack[len(p.stack)-1]}
			p.stack = append(p.stack[:len(p.stack)-2], items)
		case 0x87:
			if len(p.stack) < 3 {
				return nil, errors.New("TUPLE3 needs three items")
			}
			items := []any{p.stack[len(p.stack)-3], p.stack[len(p.stack)-2], p.stack[len(p.stack)-1]}
			p.stack = append(p.stack[:len(p.stack)-3], items)
		case 't':
			if err := p.makeTuple(); err != nil {
				return nil, err
			}
		case 0x88:
			p.stack = append(p.stack, true)
		case 0x89:
			p.stack = append(p.stack, false)
		case 0x8c:
			text, err := p.readShortString()
			if err != nil {
				return nil, err
			}
			p.stack = append(p.stack, text)
		case 'X':
			text, err := p.readBinUnicode()
			if err != nil {
				return nil, err
			}
			p.stack = append(p.stack, text)
		case 'q':
			idx := int(p.readByte())
			if len(p.stack) > 0 {
				p.memo[idx] = p.stack[len(p.stack)-1]
			}
		case 'r':
			if p.pos+4 > len(p.data) {
				return nil, errors.New("short LONG_BINPUT")
			}
			idx := int(binary.LittleEndian.Uint32(p.data[p.pos : p.pos+4]))
			p.pos += 4
			if len(p.stack) > 0 {
				p.memo[idx] = p.stack[len(p.stack)-1]
			}
		case 'h':
			idx := int(p.readByte())
			p.stack = append(p.stack, p.memo[idx])
		case 'j':
			if p.pos+4 > len(p.data) {
				return nil, errors.New("short LONG_BINGET")
			}
			idx := int(binary.LittleEndian.Uint32(p.data[p.pos : p.pos+4]))
			p.pos += 4
			p.stack = append(p.stack, p.memo[idx])
		case '.':
			if len(p.stack) == 0 {
				return nil, errors.New("empty pickle stack")
			}
			return p.stack[len(p.stack)-1], nil
		default:
			return nil, fmt.Errorf("unsupported pickle opcode 0x%x", op)
		}
	}
	return nil, errors.New("pickle missing STOP")
}

func (p *pickleParser) makeTuple() error {
	mark, err := p.popMark()
	if err != nil {
		return err
	}
	items := append([]any(nil), p.stack[mark:]...)
	p.stack = append(p.stack[:mark], items)
	return nil
}

func (p *pickleParser) appendItems() error {
	mark, err := p.popMark()
	if err != nil {
		return err
	}
	if mark == 0 {
		return errors.New("APPENDS missing list")
	}
	items := append([]any(nil), p.stack[mark:]...)
	list, ok := p.stack[mark-1].([]any)
	if !ok {
		return errors.New("APPENDS target is not list")
	}
	list = append(list, items...)
	p.stack = append(p.stack[:mark-1], list)
	return nil
}

func (p *pickleParser) setItems() error {
	mark, err := p.popMark()
	if err != nil {
		return err
	}
	if mark == 0 {
		return errors.New("SETITEMS missing dict")
	}
	dict, ok := p.stack[mark-1].(map[string]any)
	if !ok {
		return errors.New("SETITEMS target is not dict")
	}
	items := p.stack[mark:]
	if len(items)%2 != 0 {
		return errors.New("SETITEMS needs key/value pairs")
	}
	for i := 0; i < len(items); i += 2 {
		key, ok := items[i].(string)
		if !ok {
			return errors.New("SETITEMS key is not string")
		}
		dict[key] = items[i+1]
	}
	p.stack = append(p.stack[:mark-1], dict)
	return nil
}

func (p *pickleParser) setItem() error {
	if len(p.stack) < 3 {
		return errors.New("SETITEM needs dict/key/value")
	}
	value := p.stack[len(p.stack)-1]
	key, ok := p.stack[len(p.stack)-2].(string)
	if !ok {
		return errors.New("SETITEM key is not string")
	}
	dict, ok := p.stack[len(p.stack)-3].(map[string]any)
	if !ok {
		return errors.New("SETITEM target is not dict")
	}
	dict[key] = value
	p.stack = p.stack[:len(p.stack)-2]
	p.stack[len(p.stack)-1] = dict
	return nil
}

func (p *pickleParser) popMark() (int, error) {
	if len(p.marks) == 0 {
		return 0, errors.New("missing mark")
	}
	mark := p.marks[len(p.marks)-1]
	p.marks = p.marks[:len(p.marks)-1]
	if mark < 0 || mark > len(p.stack) {
		return 0, errors.New("mark out of range")
	}
	return mark, nil
}

func (p *pickleParser) readShortString() (string, error) {
	length := int(p.readByte())
	if p.pos+length > len(p.data) {
		return "", errors.New("short SHORT_BINUNICODE")
	}
	text := string(p.data[p.pos : p.pos+length])
	p.pos += length
	return text, nil
}

func (p *pickleParser) readBinUnicode() (string, error) {
	if p.pos+4 > len(p.data) {
		return "", errors.New("short BINUNICODE")
	}
	length := int(binary.LittleEndian.Uint32(p.data[p.pos : p.pos+4]))
	p.pos += 4
	if length < 0 || p.pos+length > len(p.data) || bytes.IndexByte(p.data[p.pos:p.pos+length], 0) >= 0 {
		return "", errors.New("invalid BINUNICODE")
	}
	text := string(p.data[p.pos : p.pos+length])
	p.pos += length
	return text, nil
}

func (p *pickleParser) readByte() byte {
	if p.pos >= len(p.data) {
		// 越界时安全返回 0 并仍推进 pos，使主循环 p.pos<len(p.data) 自然终止，
		// 避免截断/损坏数据触发 index out of range panic（recover 也是兜底）。
		p.pos++
		return 0
	}
	value := p.data[p.pos]
	p.pos++
	return value
}

func legacyCurveFromValues(values map[string]any) ([]controller.Point, bool) {
	lowT, ok := intFromAny(values["low_t"])
	if !ok {
		return nil, false
	}
	lowS, ok := intFromAny(values["low_s"])
	if !ok {
		return nil, false
	}
	maxT, ok := intFromAny(values["max_t"])
	if !ok {
		return nil, false
	}
	maxS, ok := intFromAny(values["max_s"])
	if !ok {
		return nil, false
	}
	return makeLegacyCurve(lowT, lowS, maxT, maxS), true
}

func curveFromAny(value any) ([]controller.Point, bool) {
	items, ok := value.([]any)
	if !ok || len(items) != controller.CurvePoints {
		return nil, false
	}
	curve := make([]controller.Point, 0, len(items))
	for _, item := range items {
		pair, ok := item.([]any)
		if !ok || len(pair) != 2 {
			return nil, false
		}
		temp, ok := floatFromAny(pair[0])
		if !ok {
			return nil, false
		}
		speed, ok := floatFromAny(pair[1])
		if !ok {
			return nil, false
		}
		curve = append(curve, controller.Point{temp, speed})
	}
	return curve, true
}

func presetsFromAny(value any) (map[string]PresetConfig, bool) {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	presets := make(map[string]PresetConfig, len(raw))
	for key, item := range raw {
		presetMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		curve, ok := curveFromAny(presetMap["curve"])
		if !ok {
			continue
		}
		strategy, ok := stringFromAny(presetMap["strategy"])
		if !ok {
			strategy = controller.DefaultStrategy
		}
		presets[key] = PresetConfig{Curve: curve, Strategy: strategy}
	}
	return presets, len(presets) > 0
}

func boolFromAny(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case int:
		return v != 0, true
	case int64:
		return v != 0, true
	}
	return false, false
}

func intFromAny(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(v)
		return n, err == nil
	}
	return 0, false
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case string:
		n, err := strconv.ParseFloat(v, 64)
		return n, err == nil
	}
	return 0, false
}

func stringFromAny(value any) (string, bool) {
	v, ok := value.(string)
	return v, ok
}
