package newapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Usage struct {
	USD             float64
	Success, Failed int
}

func (u Usage) Calls() int { return u.Success + u.Failed }

type Model struct {
	Name  string
	Calls int
}
type UserUsage struct {
	ID   int
	Name string
	USD  float64
}
type Balance struct{ Name, Detail, Error, Value, Note string }
type Dashboard struct {
	Today, Week, Month        Usage
	Models                    []Model
	Users                     []UserUsage
	Balances                  []Balance
	TotalUSD                  float64
	Success, Failed, LogCount int
	Partial                   bool
	Updated                   time.Time
}
type ResetResult struct {
	Total, Done, Failed int
	Error               string
}
type Service interface {
	Fetch(context.Context) (Dashboard, error)
	Reset(context.Context, float64, func(ResetResult)) ResetResult
}
type Client struct {
	cfg  Config
	http *http.Client
	key  string
}

func NewClient(c Config) (*Client, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	key, err := c.Service.Key()
	if err != nil {
		return nil, err
	}
	return &Client{cfg: c, key: key, http: &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(), Timeout: time.Duration(c.TimeoutSeconds) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) Close() error { c.http.CloseIdleConnections(); return nil }

// Never include remote response bodies, URLs or credentials in UI errors.
func (c *Client) request(ctx context.Context, method, endpoint, contentType string, body []byte, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("请求地址无效")
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("网络请求失败或超时")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errors.New("鉴权失败，请更新凭证并重新进入")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("接口 HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return errors.New("读取响应失败")
	}
	if len(b) > 8<<20 {
		return errors.New("接口响应过大")
	}
	if json.Unmarshal(b, out) != nil {
		return errors.New("接口 JSON 格式不兼容")
	}
	return nil
}

func (c *Client) api(ctx context.Context, method, path string, payload any, out any) error {
	var b []byte
	if payload != nil {
		var err error
		b, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	var response struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	err := c.request(ctx, method, strings.TrimRight(c.cfg.URL, "/")+path, "application/json", b, map[string]string{"Authorization": "Bearer " + c.key, "New-Api-User": strconv.Itoa(c.cfg.AdminID)}, &response)
	if err != nil {
		return err
	}
	if !response.Success {
		return errors.New("NewAPI 拒绝请求，请检查管理员权限及接口版本")
	}
	if out != nil && json.Unmarshal(response.Data, out) != nil {
		return errors.New("NewAPI 数据格式不兼容")
	}
	return nil
}

type logEntry struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	CreatedAt int64  `json:"created_at"`
	Type      int    `json:"type"`
	Username  string `json:"username"`
	Model     string `json:"model_name"`
	Quota     int64  `json:"quota"`
	Channel   int    `json:"channel"`
}
type user struct {
	ID    int    `json:"id"`
	Role  int    `json:"role"`
	Quota *int64 `json:"quota"`
}
type page[T any] struct {
	Items []T  `json:"items"`
	Total *int `json:"total"`
}

// Walk pages to completion; fail on repeated IDs or truncated pages rather than silently dropping users.
func walk[T any](ctx context.Context, c *Client, path string, limit int, id func(T) int, visit func(T)) (bool, error) {
	seen := map[int]bool{}
	offset := 0
	expectedTotal := -1
	for p := 1; ; p++ {
		var result page[T]
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		if err := c.api(ctx, "GET", fmt.Sprintf("%s%sp=%d&page_size=100", path, sep, p), nil, &result); err != nil {
			return false, err
		}
		if result.Total == nil || *result.Total < 0 || len(result.Items) == 0 && offset < *result.Total {
			return false, errors.New("分页数据不完整")
		}
		if expectedTotal < 0 {
			expectedTotal = *result.Total
		}
		if *result.Total != expectedTotal || offset+len(result.Items) > expectedTotal {
			return false, errors.New("分页数量发生变化，请刷新后重试")
		}
		for _, v := range result.Items {
			key := id(v)
			if key < 1 || seen[key] {
				return false, errors.New("分页发生变化，请刷新后重试")
			}
			seen[key] = true
			visit(v)
			offset++
			if offset >= limit && offset < *result.Total {
				return true, nil
			}
		}
		if offset >= *result.Total {
			return false, nil
		}
	}
}

func (c *Client) quotaUnit(ctx context.Context) (float64, error) {
	var s struct {
		Quota float64 `json:"quota_per_unit"`
	}
	if err := c.api(ctx, "GET", "/api/status", nil, &s); err != nil {
		return 0, err
	}
	if s.Quota <= 0 || math.IsNaN(s.Quota) || math.IsInf(s.Quota, 0) {
		return 0, errors.New("服务未返回有效 quota_per_unit，无法换算美元")
	}
	return s.Quota, nil
}

func (c *Client) Fetch(ctx context.Context) (Dashboard, error) {
	return c.fetchAt(ctx, time.Now())
}

// Calendar metrics and rolling seven-day rankings share a single bounded log scan.
func (c *Client) fetchAt(ctx context.Context, sampled time.Time) (Dashboard, error) {
	d := Dashboard{Updated: sampled}
	unit, err := c.quotaUnit(ctx)
	if err != nil {
		return d, err
	}
	loc, _ := time.LoadLocation(c.cfg.Timezone)
	now := sampled.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	week := today.AddDate(0, 0, -(int(today.Weekday())+6)%7)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	ranking := today.AddDate(0, 0, -6)
	start := month
	for _, boundary := range []time.Time{week, ranking} {
		if boundary.Before(start) {
			start = boundary
		}
	}
	models := map[string]*Model{}
	users := map[int]*UserUsage{}
	path := fmt.Sprintf("/api/log/?type=0&start_timestamp=%d&end_timestamp=%d", start.Unix(), now.Unix())
	d.Partial, err = walk(ctx, c, path, c.cfg.MaxLogs, func(l logEntry) int { return l.ID }, func(l logEntry) {
		if l.CreatedAt < start.Unix() || l.CreatedAt > now.Unix() || (l.Type != 2 && l.Type != 5) {
			return
		}
		d.LogCount++
		usd := 0.0
		if l.Type == 2 {
			usd = float64(l.Quota) / unit
		}
		for _, period := range []struct {
			start time.Time
			usage *Usage
		}{{today, &d.Today}, {week, &d.Week}, {month, &d.Month}} {
			if l.CreatedAt >= period.start.Unix() {
				period.usage.USD += usd
				if l.Type == 2 {
					period.usage.Success++
				} else {
					period.usage.Failed++
				}
			}
		}
		if l.CreatedAt < ranking.Unix() {
			return
		}
		if l.Type == 2 {
			d.Success++
			d.TotalUSD += usd
		} else {
			d.Failed++
		}
		m := models[l.Model]
		if m == nil {
			m = &Model{Name: l.Model}
			models[l.Model] = m
		}
		m.Calls++
		if l.Type != 2 {
			return
		}
		u := users[l.UserID]
		if u == nil {
			u = &UserUsage{ID: l.UserID, Name: l.Username}
			users[l.UserID] = u
		}
		u.USD += usd
	})
	if err != nil {
		return Dashboard{}, err
	}
	for _, v := range models {
		d.Models = append(d.Models, *v)
	}
	sort.Slice(d.Models, func(i, j int) bool {
		if d.Models[i].Calls == d.Models[j].Calls {
			return d.Models[i].Name < d.Models[j].Name
		}
		return d.Models[i].Calls > d.Models[j].Calls
	})
	if len(d.Models) > c.cfg.TopN {
		d.Models = d.Models[:c.cfg.TopN]
	}
	for _, v := range users {
		d.Users = append(d.Users, *v)
	}
	sort.Slice(d.Users, func(i, j int) bool {
		if d.Users[i].USD == d.Users[j].USD {
			return d.Users[i].ID < d.Users[j].ID
		}
		return d.Users[i].USD > d.Users[j].USD
	})
	if len(d.Users) > c.cfg.TopN {
		d.Users = d.Users[:c.cfg.TopN]
	}
	for _, p := range c.cfg.Providers {
		if ctx.Err() != nil {
			return Dashboard{}, ctx.Err()
		}
		d.Balances = append(d.Balances, c.provider(ctx, p))
	}
	return d, nil
}

func rate(success, failed int) float64 {
	if success+failed == 0 {
		return 0
	}
	return float64(success) * 100 / float64(success+failed)
}

func (c *Client) Reset(ctx context.Context, usd float64, progress func(ResetResult)) ResetResult {
	r := ResetResult{}
	unit, err := c.quotaUnit(ctx)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	q := math.Round(usd * unit)
	if math.IsNaN(q) || math.IsInf(q, 0) || q < 0 || q > 2147483647 {
		r.Error = "重置额度超出安全范围"
		return r
	}
	var self user
	if err = c.api(ctx, "GET", "/api/user/self", nil, &self); err != nil {
		r.Error = err.Error()
		return r
	}
	if self.ID != c.cfg.AdminID || self.Role < 10 {
		r.Error = "管理员身份不匹配，未执行重置"
		return r
	}
	var users []user
	partial, err := walk(ctx, c, "/api/user/?sort_by=id&sort_order=asc", 1000000, func(u user) int { return u.ID }, func(u user) { users = append(users, u) })
	if err != nil {
		r.Error = err.Error()
		return r
	}
	if partial {
		r.Error = "用户列表超过上限，未执行重置"
		return r
	}
	r.Total = len(users)
	// Check all roles before the first write; never call a partial run "all users".
	for _, u := range users {
		if self.Role != 100 && self.Role <= u.Role {
			r.Error = "重置所有用户需要 root 管理员权限"
			return r
		}
	}
	for _, u := range users {
		if ctx.Err() != nil {
			r.Error = "执行已中止；已发送请求可能已生效，请核对余额"
			return r
		}
		payload := map[string]any{"id": u.ID, "action": "add_quota", "mode": "override", "value": int64(q)}
		if err = c.api(ctx, "POST", "/api/user/manage", payload, nil); err != nil {
			r.Failed++
			r.Error = fmt.Sprintf("用户 #%d：%s；本次结果需核对，未继续执行", u.ID, err)
			progress(r)
			return r
		}
		// A successful HTTP envelope alone does not prove a quota change.
		var actual user
		if err = c.api(ctx, "GET", "/api/user/"+strconv.Itoa(u.ID), nil, &actual); err != nil || actual.ID != u.ID || actual.Quota == nil || *actual.Quota != int64(q) {
			r.Failed++
			r.Error = fmt.Sprintf("用户 #%d 余额校验失败（可能存在并发消耗），请核对", u.ID)
			progress(r)
			return r
		}
		r.Done++
		progress(r)
	}
	return r
}
