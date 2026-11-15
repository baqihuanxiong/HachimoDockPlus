package main

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const usage = `mijia — standalone Mi Home cloud CLI (JSON output)
  auth login                         create browser OAuth URL
  auth authorize --file PATH         import authorization result or callback URL
  auth status | auth logout
  devices refresh | devices list
  device spec DID
  device get DID [SIID PIID]         read all readable properties or one property
  device set DID SIID PIID JSON_VALUE
  device action DID SIID AIID JSON_ARRAY
  scenes list | scenes run ID
MIJIA_STATE_DIR overrides private state; otherwise HACHIMODOCK_CLI_STATE_DIR/mijia.
`

func main() {
	if len(os.Args) == 1 || os.Args[1] == "--help" || os.Args[1] == "help" {
		fmt.Print(usage)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	root, err := stateDirectory()
	var result any
	if err == nil {
		var a *app
		var closeFn func()
		a, closeFn, err = openApp(ctx, root)
		if err == nil {
			result, err = a.run(ctx, os.Args[1:])
			closeFn()
		}
	}
	if err != nil {
		json.NewEncoder(os.Stderr).Encode(map[string]any{"ok": false, "error": err.Error()})
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "data": result})
}

func (a *app) run(ctx context.Context, args []string) (any, error) {
	if len(args) < 2 {
		return nil, errors.New(usage)
	}
	switch strings.Join(args[:2], " ") {
	case "auth login":
		if len(args) != 2 {
			return nil, errors.New("用法：auth login")
		}
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		deviceID := "mico." + hex.EncodeToString(b)
		sum := sha1.Sum([]byte("d=" + deviceID))
		state := hex.EncodeToString(sum[:])
		if err := a.write("pending-login.json", pendingLogin{DeviceID: deviceID, State: state, ExpiresAt: time.Now().Unix() + 600}); err != nil {
			return nil, err
		}
		return map[string]any{"url": "https://account.xiaomi.com/oauth2/authorize?" + url.Values{"client_id": {clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"}, "device_id": {deviceID}, "state": {state}, "skip_confirm": {"false"}}.Encode(), "expires_in": 600, "next": "在浏览器授权后，将结果保存为文件，再运行 auth authorize --file PATH"}, nil
	case "auth authorize":
		if len(args) != 4 || args[2] != "--file" {
			return nil, errors.New("用法：auth authorize --file PATH")
		}
		f, err := os.Open(args[3])
		if err != nil {
			return nil, errors.New("无法读取授权结果文件")
		}
		payload, err := io.ReadAll(io.LimitReader(f, 65537))
		f.Close()
		if err != nil || len(payload) > 65536 {
			return nil, errors.New("授权结果文件过大或无法读取")
		}
		var pending pendingLogin
		if err = a.read("pending-login.json", &pending); err != nil || pending.ExpiresAt <= time.Now().Unix() {
			return nil, errors.New("请重新运行 auth login")
		}
		var authCode struct {
			Code  string `json:"code"`
			State string `json:"state"`
		}
		text := strings.TrimSpace(string(payload))
		u, e := url.Parse(text)
		if e == nil && u.Scheme != "" {
			authCode.Code = u.Query().Get("code")
			authCode.State = u.Query().Get("state")
		} else {
			decoded, e := base64.StdEncoding.DecodeString(text)
			if e != nil || json.Unmarshal(decoded, &authCode) != nil {
				return nil, errors.New("授权结果格式无效")
			}
		}
		if authCode.State != pending.State || authCode.Code == "" {
			return nil, errors.New("授权 state 不匹配或缺少 code")
		}
		auth, err := a.cloud.token(ctx, map[string]any{"client_id": clientID, "redirect_uri": redirectURI, "device_id": pending.DeviceID, "code": authCode.Code})
		if err != nil {
			return nil, err
		}
		// Clear the previous account's metadata before installing new credentials.
		a.cache = cache{Specs: map[string]spec{}}
		if err = a.write("devices.json", a.cache); err != nil {
			return nil, err
		}
		if err = a.write("credentials.json", auth); err != nil {
			return nil, err
		}
		a.auth = auth
		os.Remove(filepath.Join(a.root, "pending-login.json"))
		return map[string]bool{"connected": true}, nil
	case "auth status":
		return map[string]bool{"connected": a.auth.AccessToken != ""}, nil
	case "auth logout":
		for _, name := range []string{"credentials.json", "devices.json", "pending-login.json"} {
			if err := os.Remove(filepath.Join(a.root, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
		a.auth = credentials{}
		a.cache = cache{}
		return map[string]bool{"connected": false}, nil
	}
	if a.auth.AccessToken == "" {
		return nil, errors.New("请先完成账号授权")
	}
	switch strings.Join(args[:2], " ") {
	case "devices refresh":
		if len(args) != 2 {
			return nil, errors.New("用法：devices refresh")
		}
		return a.sync(ctx)
	case "devices list":
		return a.cache.Devices, nil
	case "scenes list":
		rows := []map[string]string{}
		for _, s := range a.cache.Scenes {
			rows = append(rows, map[string]string{"id": s.ID, "name": s.Name, "home": s.Home})
		}
		return rows, nil
	case "scenes run":
		if len(args) != 3 {
			return nil, errors.New("用法：scenes run ID")
		}
		for _, s := range a.cache.Scenes {
			if s.ID == args[2] {
				raw, err := a.post(ctx, "/app/appgateway/miot/appsceneservice/AppSceneService/NewRunScene", map[string]any{"owner_uid": s.OwnerUID, "home_id": s.HomeID, "scene_id": s.SceneID, "scene_type": 2, "room_id": s.RoomID})
				if err != nil {
					return nil, err
				}
				if string(raw) != "true" {
					return nil, errors.New("场景未明确受理，结果未知；不会重试")
				}
				return map[string]string{"status": "accepted", "message": "场景已受理，未逐设备核验"}, nil
			}
		}
		return nil, errors.New("场景不在账号清单中")
	case "device spec":
		if len(args) != 3 {
			return nil, errors.New("用法：device spec DID")
		}
		return a.describe(ctx, args[2])
	case "device get":
		if len(args) != 3 && len(args) != 5 {
			return nil, errors.New("用法：device get DID [SIID PIID]")
		}
		siid, piid := 0, 0
		if len(args) == 5 {
			var e1, e2 error
			siid, e1 = strconv.Atoi(args[3])
			piid, e2 = strconv.Atoi(args[4])
			if e1 != nil || e2 != nil || siid <= 0 || piid <= 0 {
				return nil, errors.New("能力 ID 必须为正整数")
			}
		}
		return a.readProperties(ctx, args[2], siid, piid)
	case "device set", "device action":
		return a.control(ctx, args)
	default:
		return nil, errors.New(usage)
	}
}

func (a *app) readProperties(ctx context.Context, did string, siid, piid int) (any, error) {
	s, err := a.describe(ctx, did)
	if err != nil {
		return nil, err
	}
	params := []map[string]any{}
	for _, p := range s.Properties {
		if hasAccess(p, "read") && (siid == 0 || p.SIID == siid && p.PIID == piid) {
			params = append(params, map[string]any{"did": did, "siid": p.SIID, "piid": p.PIID})
		}
	}
	if len(params) == 0 {
		return nil, errors.New("设备属性不存在或不可读")
	}
	result := []propertyResult{}
	succeeded := false
	for start := 0; start < len(params); start += 100 {
		end := start + 100
		if end > len(params) {
			end = len(params)
		}
		raw, err := a.post(ctx, "/app/v2/miotspec/prop/get", map[string]any{"datasource": 1, "params": params[start:end]})
		if err != nil {
			return nil, err
		}
		var batch []propertyResult
		if json.Unmarshal(raw, &batch) != nil {
			return nil, errors.New("属性响应无效")
		}
		if len(batch) == 0 {
			return nil, errors.New("设备未返回属性结果")
		}
		for i := range batch {
			r := &batch[i]
			if r.Code == nil {
				return nil, errors.New("属性结果缺少状态码")
			}
			if *r.Code == 0 {
				if r.Value == nil {
					return nil, errors.New("属性结果缺少有效读数")
				}
				r.Error = ""
				succeeded = true
			} else {
				// A failed optional property must not discard other readings.
				r.Value = nil
				r.Error = propertyReadError(*r)
			}
		}
		result = append(result, batch...)
	}
	if !succeeded {
		return nil, fmt.Errorf("未读到有效属性：%s（共 %d 项失败）", result[0].Error, len(result))
	}
	return result, nil
}

func (a *app) control(ctx context.Context, args []string) (any, error) {
	if len(args) != 6 {
		return nil, errors.New("用法：device set|action DID SIID PIID|AIID JSON_VALUE|JSON_ARRAY")
	}
	did := args[2]
	siid, e1 := strconv.Atoi(args[3])
	iid, e2 := strconv.Atoi(args[4])
	if e1 != nil || e2 != nil || siid <= 0 || iid <= 0 {
		return nil, errors.New("能力 ID 必须为正整数")
	}
	var value any
	if json.Unmarshal([]byte(args[5]), &value) != nil {
		return nil, errors.New("参数必须是有效 JSON")
	}
	s, err := a.describe(ctx, did)
	if err != nil {
		return nil, err
	}
	if args[1] == "action" {
		var cap *action
		for _, v := range s.Actions {
			if v.SIID == siid && v.AIID == iid {
				v := v
				cap = &v
				break
			}
		}
		if cap == nil {
			return nil, errors.New("设备未公开此动作")
		}
		values, ok := value.([]any)
		if !ok || len(values) != len(cap.In) || len(cap.Arguments) != len(cap.In) {
			return nil, errors.New("动作参数数量不符或参数定义缺失")
		}
		for i, v := range values {
			if err := validateValue(cap.Arguments[i], v); err != nil {
				return nil, err
			}
			if b, ok := v.(bool); ok {
				if b {
					values[i] = 1
				} else {
					values[i] = 0
				}
			}
		}
		raw, err := a.post(ctx, "/app/v2/miotspec/action", map[string]any{"params": map[string]any{"did": did, "siid": siid, "aiid": iid, "in": values}})
		if err != nil {
			return nil, err
		}
		if err = checkResult(raw); err != nil {
			return nil, err
		}
		return map[string]string{"status": "accepted", "message": "动作已受理，完成状态未核验"}, nil
	}
	var cap *property
	for _, v := range s.Properties {
		if v.SIID == siid && v.PIID == iid {
			v := v
			cap = &v
			break
		}
	}
	if cap == nil || !hasAccess(*cap, "write") {
		return nil, errors.New("设备属性不存在或不可写")
	}
	if err := validateValue(*cap, value); err != nil {
		return nil, err
	}
	param := map[string]any{"did": did, "siid": siid, "piid": iid, "value": value}
	raw, err := a.post(ctx, "/app/v2/miotspec/prop/set", map[string]any{"params": []any{param}})
	if err != nil {
		return nil, err
	}
	if err = checkResult(raw); err != nil {
		return nil, err
	}
	if hasAccess(*cap, "read") {
		read, err := a.post(ctx, "/app/v2/miotspec/prop/get", map[string]any{"datasource": 1, "params": []any{map[string]any{"did": did, "siid": siid, "piid": iid}}})
		if err == nil && checkResult(read) == nil {
			var rows []propertyResult
			if json.Unmarshal(read, &rows) == nil {
				want, _ := json.Marshal(value)
				for _, r := range rows {
					got, _ := json.Marshal(r.Value)
					if r.DID == did && r.SIID == siid && r.PIID == iid && string(want) == string(got) {
						return map[string]string{"status": "verified"}, nil
					}
				}
			}
		}
	}
	return map[string]string{"status": "accepted", "message": "命令已受理，设备最终状态未确认"}, nil
}
