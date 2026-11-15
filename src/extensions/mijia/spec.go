package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
)

type property struct {
	SIID   int       `json:"siid"`
	PIID   int       `json:"piid"`
	Name   string    `json:"name"`
	Type   string    `json:"type"`
	Format string    `json:"format"`
	Access []string  `json:"access"`
	Range  []float64 `json:"range,omitempty"`
	Values []struct {
		Value       any    `json:"value"`
		Description string `json:"description"`
	} `json:"values,omitempty"`
}
type action struct {
	SIID      int        `json:"siid"`
	AIID      int        `json:"aiid"`
	Name      string     `json:"name"`
	In        []int      `json:"in"`
	Arguments []property `json:"arguments"`
}
type spec struct {
	DID        string     `json:"did"`
	Name       string     `json:"name"`
	Properties []property `json:"properties"`
	Actions    []action   `json:"actions"`
}

var modelPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,160}$`)

func (a *app) findDevice(did string) (device, error) {
	for _, d := range a.cache.Devices {
		if d.DID == did {
			return d, nil
		}
	}
	return device{}, errors.New("设备不在已同步账号清单中，请先 devices refresh")
}
func (a *app) describe(ctx context.Context, did string) (spec, error) {
	d, err := a.findDevice(did)
	if err != nil {
		return spec{}, err
	}
	if s, ok := a.cache.Specs[did]; ok {
		return s, nil
	}
	urn := d.URN
	if urn == "" {
		if !modelPattern.MatchString(d.Model) {
			return spec{}, errors.New("设备型号无效")
		}
		raw, err := a.cloud.request(ctx, "GET", a.cloud.specURL+"/internal/urn-by-model-version?"+url.Values{"model": {d.Model}, "version": {"0"}}.Encode(), nil, nil)
		if err != nil {
			return spec{}, err
		}
		var v struct {
			URN string `json:"urn"`
		}
		if json.Unmarshal(raw, &v) != nil {
			return spec{}, errors.New("型号能力响应无效")
		}
		urn = v.URN
	}
	if !strings.HasPrefix(urn, "urn:miot-spec-v2:device:") || len(urn) > 512 {
		return spec{}, errors.New("设备尚未提供标准 MIOT 能力")
	}
	raw, err := a.cloud.request(ctx, "GET", a.cloud.specURL+"/miot-spec-v2/instance?"+url.Values{"type": {urn}}.Encode(), nil, nil)
	if err != nil {
		return spec{}, err
	}
	var definition struct {
		Services []struct {
			IID         int    `json:"iid"`
			Description string `json:"description"`
			Properties  []struct {
				IID         int       `json:"iid"`
				Description string    `json:"description"`
				Type        string    `json:"type"`
				Format      string    `json:"format"`
				Access      []string  `json:"access"`
				Range       []float64 `json:"value-range"`
				Values      []struct {
					Value       any    `json:"value"`
					Description string `json:"description"`
				} `json:"value-list"`
			} `json:"properties"`
			Actions []struct {
				IID         int    `json:"iid"`
				Description string `json:"description"`
				In          []int  `json:"in"`
			} `json:"actions"`
		} `json:"services"`
	}
	if json.Unmarshal(raw, &definition) != nil || definition.Services == nil {
		return spec{}, errors.New("设备能力响应无效")
	}
	s := spec{DID: did, Name: d.Name, Properties: []property{}, Actions: []action{}}
	for _, service := range definition.Services {
		for _, p := range service.Properties {
			s.Properties = append(s.Properties, property{SIID: service.IID, PIID: p.IID, Name: service.Description + " · " + p.Description, Type: p.Type, Format: p.Format, Access: p.Access, Range: p.Range, Values: p.Values})
		}
		for _, v := range service.Actions {
			ac := action{SIID: service.IID, AIID: v.IID, Name: service.Description + " · " + v.Description, In: v.In, Arguments: []property{}}
			for _, piid := range v.In {
				for _, p := range s.Properties {
					if p.SIID == service.IID && p.PIID == piid {
						ac.Arguments = append(ac.Arguments, p)
					}
				}
			}
			s.Actions = append(s.Actions, ac)
		}
	}
	a.cache.Specs[did] = s
	if err = a.write("devices.json", a.cache); err != nil {
		return spec{}, err
	}
	return s, nil
}

func hasAccess(p property, mode string) bool {
	for _, v := range p.Access {
		if v == mode {
			return true
		}
	}
	return false
}
func validateValue(p property, value any) error {
	valid := false
	switch p.Format {
	case "bool":
		_, valid = value.(bool)
	case "string":
		text, ok := value.(string)
		valid = ok && len(text) <= 4096
	case "float":
		_, valid = value.(float64)
	case "uint8", "uint16", "uint32", "uint64", "int8", "int16", "int32", "int64":
		n, ok := value.(float64)
		valid = ok && math.Trunc(n) == n
		if strings.HasPrefix(p.Format, "uint") {
			valid = valid && n >= 0
		}
		bounds := map[string][2]float64{"uint8": {0, 255}, "uint16": {0, 65535}, "uint32": {0, 4294967295}, "int8": {-128, 127}, "int16": {-32768, 32767}, "int32": {-2147483648, 2147483647}, "int64": {-9007199254740991, 9007199254740991}, "uint64": {0, 9007199254740991}}
		if b, ok := bounds[p.Format]; ok {
			valid = valid && n >= b[0] && n <= b[1]
		}
	}
	if !valid {
		return errors.New("参数类型与设备定义不符")
	}
	if len(p.Values) > 0 {
		match := false
		want, _ := json.Marshal(value)
		for _, v := range p.Values {
			b, _ := json.Marshal(v.Value)
			if string(b) == string(want) {
				match = true
			}
		}
		if !match {
			return errors.New("参数不在设备枚举选项中")
		}
	}
	if len(p.Range) >= 2 {
		n, ok := value.(float64)
		if !ok || n < p.Range[0] || n > p.Range[1] {
			return errors.New("参数超出设备范围")
		}
		if len(p.Range) > 2 && p.Range[2] > 0 {
			steps := (n - p.Range[0]) / p.Range[2]
			if math.Abs(steps-math.Round(steps)) > 1e-6 {
				return errors.New("参数不符合设备步进")
			}
		}
	}
	return nil
}

type propertyResult struct {
	DID   string `json:"did"`
	SIID  int    `json:"siid"`
	PIID  int    `json:"piid"`
	Code  *int   `json:"code"`
	Value any    `json:"value"`
}

func checkResult(raw json.RawMessage) error {
	var rows []struct {
		Code *int `json:"code"`
	}
	if json.Unmarshal(raw, &rows) != nil {
		var row struct {
			Code *int `json:"code"`
		}
		if json.Unmarshal(raw, &row) != nil {
			return errors.New("设备响应格式无效")
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return errors.New("设备未返回执行结果")
	}
	for _, row := range rows {
		if row.Code == nil {
			return errors.New("设备结果缺少状态码")
		}
		if *row.Code != 0 {
			return fmt.Errorf("设备返回失败代码 %d；不会自动重试", *row.Code)
		}
	}
	return nil
}
