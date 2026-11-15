package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

type room struct {
	ID   any    `json:"id"`
	Name string `json:"name"`
	DIDs []any  `json:"dids"`
}
type home struct {
	ID    any    `json:"id"`
	UID   any    `json:"uid"`
	Name  string `json:"name"`
	DIDs  []any  `json:"dids"`
	Rooms []room `json:"roomlist"`
}

func scalar(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// Cloud IDs may be JSON strings or integers, including integers larger than
// float64 can represent exactly. Preserve them across pagination and caching.
func decodeMetadata(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	return d.Decode(out)
}

func (a *app) sync(ctx context.Context) (any, error) {
	raw, err := a.post(ctx, "/app/v2/homeroom/gethome", map[string]any{"limit": 150, "fetch_share": false, "fetch_share_dev": false, "plat_form": 0, "app_ver": 9})
	if err != nil {
		return nil, err
	}
	var homes struct {
		List  []home `json:"homelist"`
		More  bool   `json:"has_more"`
		MaxID any    `json:"max_id"`
	}
	if decodeMetadata(raw, &homes) != nil || homes.List == nil {
		return nil, errors.New("家庭清单响应无效")
	}
	type place struct{ Home, Room string }
	places := map[string]place{}
	add := func(h home) {
		for _, did := range h.DIDs {
			if _, exists := places[scalar(did)]; !exists {
				places[scalar(did)] = place{h.Name, "未分配房间"}
			}
		}
		for _, r := range h.Rooms {
			for _, did := range r.DIDs {
				places[scalar(did)] = place{h.Name, r.Name}
			}
		}
	}
	for _, h := range homes.List {
		add(h)
	}
	for page := 0; homes.More; page++ {
		if page >= 50 || homes.MaxID == nil {
			return nil, errors.New("家庭分页未完成，保留原清单")
		}
		raw, err = a.post(ctx, "/app/v2/homeroom/get_dev_room_page", map[string]any{"start_id": homes.MaxID, "limit": 150})
		if err != nil {
			return nil, err
		}
		var more struct {
			List  []home `json:"info"`
			More  bool   `json:"has_more"`
			MaxID any    `json:"max_id"`
		}
		if decodeMetadata(raw, &more) != nil || more.List == nil {
			return nil, errors.New("家庭分页响应无效")
		}
		for _, h := range more.List {
			found := false
			for _, base := range homes.List {
				if scalar(base.ID) == scalar(h.ID) {
					found = true
					h.Name = base.Name
					for i := range h.Rooms {
						h.Rooms[i].Name = "未命名房间"
						for _, r := range base.Rooms {
							if scalar(r.ID) == scalar(h.Rooms[i].ID) {
								h.Rooms[i].Name = r.Name
							}
						}
					}
					break
				}
			}
			if !found {
				return nil, errors.New("家庭清单已变化，请重新同步")
			}
			add(h)
		}
		if more.More && scalar(more.MaxID) == scalar(homes.MaxID) {
			return nil, errors.New("家庭分页游标未推进")
		}
		homes.More = more.More
		homes.MaxID = more.MaxID
	}
	dids := make([]string, 0, len(places))
	for did := range places {
		dids = append(dids, did)
	}
	sort.Strings(dids)
	next := cache{Devices: []device{}, Scenes: []scene{}, Specs: map[string]spec{}}
	for start := 0; start < len(dids); start += 150 {
		end := start + 150
		if end > len(dids) {
			end = len(dids)
		}
		var cursor any
		for page := 0; ; page++ {
			raw, err = a.post(ctx, "/app/v2/home/device_list_page", map[string]any{"limit": 200, "get_split_device": true, "dids": dids[start:end], "start_did": cursor})
			if err != nil {
				return nil, err
			}
			var batch struct {
				List []struct {
					DID    any    `json:"did"`
					Name   string `json:"name"`
					Model  string `json:"model"`
					URN    string `json:"spec_type"`
					Online bool   `json:"isOnline"`
				} `json:"list"`
				More bool `json:"has_more"`
				Next any  `json:"next_start_did"`
			}
			if decodeMetadata(raw, &batch) != nil || batch.List == nil {
				return nil, errors.New("设备清单响应无效")
			}
			for _, d := range batch.List {
				did := scalar(d.DID)
				if p, ok := places[did]; ok {
					next.Devices = append(next.Devices, device{DID: did, Name: d.Name, Model: d.Model, URN: d.URN, Home: p.Home, Room: p.Room, Online: d.Online})
				}
			}
			if !batch.More {
				break
			}
			if page >= 49 || batch.Next == nil || scalar(batch.Next) == scalar(cursor) {
				return nil, errors.New("设备分页未完成，保留原清单")
			}
			cursor = batch.Next
		}
	}
	warnings := []string{}
	for _, h := range homes.List {
		raw, err = a.post(ctx, "/app/appgateway/miot/appsceneservice/AppSceneService/GetManualSceneList", map[string]any{"home_id": h.ID, "owner_uid": h.UID, "source": "zkp", "get_type": 2})
		if err != nil {
			warnings = append(warnings, "部分场景未能同步")
			continue
		}
		var list []struct {
			ID     any    `json:"scene_id"`
			Name   string `json:"scene_name"`
			RoomID any    `json:"room_id"`
		}
		if decodeMetadata(raw, &list) != nil {
			warnings = append(warnings, "部分场景响应无法解析")
			continue
		}
		for _, s := range list {
			next.Scenes = append(next.Scenes, scene{ID: scalar(s.ID), Name: s.Name, Home: h.Name, HomeID: h.ID, OwnerUID: h.UID, SceneID: s.ID, RoomID: s.RoomID})
		}
	}
	sort.Slice(next.Devices, func(i, j int) bool { return next.Devices[i].DID < next.Devices[j].DID })
	if err = a.write("devices.json", next); err != nil {
		return nil, err
	}
	a.cache = next
	return map[string]any{"devices": len(next.Devices), "scenes": len(next.Scenes), "warnings": warnings}, nil
}
