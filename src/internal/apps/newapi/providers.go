package newapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Bailian uses a console token; MiMo exposes a read-only Token Plan API keyed by its inference credential.
func (c *Client) provider(ctx context.Context, p ProviderConfig) Balance {
	b := Balance{Name: p.Name}
	if b.Name == "" {
		b.Name = p.Kind
	}
	key, err := p.Service.Key()
	if err != nil {
		b.Error = "未配置有效凭证（密钥文件需 chmod 600）"
		return b
	}
	if p.Kind == "mimo-token-plan" {
		b, err = c.mimo(ctx, p, key)
	} else if p.Auth == "access-key" {
		b, err = c.aliyunWithAccessKey(ctx, p, key)
	} else {
		b, err = c.aliyun(ctx, p, key)
	}
	b.Name = p.Name
	if b.Name == "" {
		b.Name = p.Kind
	}
	if err != nil {
		b.Error = err.Error()
	}
	return b
}

func (c *Client) aliyun(ctx context.Context, p ProviderConfig, key string) (Balance, error) {
	region := p.Region
	if region == "" {
		region = "cn-beijing"
	}
	action := "BroadScopeAspnGateway"
	if region == "ap-southeast-1" {
		action = "IntlBroadScopeAspnGateway"
	}
	api := "zeldaHttp.apikeyMgr./tokenplan/personal/api/v2/usage"
	params, _ := json.Marshal(map[string]any{"Api": api, "V": "1.0", "Data": map[string]any{"cornerstoneParam": map[string]any{"protocol": "V2", "console": "ONE_CONSOLE", "productCode": "p_efm", "switchUserType": 3, "consoleSite": "BAILIAN_ALIYUN"}}})
	query := url.Values{"action": {action}, "product": {"sfm_bailian"}, "api": {api}}
	body := url.Values{"params": {string(params)}, "region": {region}}
	var response map[string]json.RawMessage
	err := c.request(ctx, "POST", strings.TrimRight(p.URL, "/")+"/cli/api.json?"+query.Encode(), "application/x-www-form-urlencoded", []byte(body.Encode()), map[string]string{"Authorization": "Bearer " + key}, &response)
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			return Balance{}, errAliyunSessionExpired
		}
		return Balance{}, err
	}
	// Bailian wraps quota in data.DataV2.data.data; reject any failed envelope.
	for depth := 0; depth < 7; depth++ {
		if v, ok := response["success"]; ok && string(v) == "false" {
			var code string
			_ = json.Unmarshal(response["errorCode"], &code)
			switch code {
			case "BailianGateway.Login.NotLogined":
				return Balance{}, errAliyunSessionExpired
			case "BailianGateway.Team.NotAuthorised":
				return Balance{}, errors.New("请在百炼账号管理中将 RAM 用户加入业务空间并授权")
			case "BailianGateway.AuthorityPolicies.NoPermission":
				return Balance{}, errors.New("百炼权限不足，请检查 RAM 用户的智能体操作权限")
			default:
				return Balance{}, errors.New("阿里额度查询失败，请检查 RAM 与百炼权限")
			}
		}
		if raw, ok := response["per1MonthPercentage"]; ok {
			var used *float64
			if json.Unmarshal(raw, &used) != nil || used == nil || *used < 0 || *used > 1 {
				return Balance{}, errors.New("阿里月度额度格式不兼容")
			}
			detail := fmt.Sprintf("月度剩余 %.1f%% · 已用 %.1f%%", (1-*used)*100, *used*100)
			var reset int64
			if json.Unmarshal(response["per1MonthResetTime"], &reset) == nil && reset > 0 {
				if reset > 1e12 {
					reset /= 1000
				}
				loc, _ := time.LoadLocation(c.cfg.Timezone)
				detail += " · 重置 " + time.Unix(reset, 0).In(loc).Format("01-02 15:04")
			}
			return Balance{Detail: detail, Value: fmt.Sprintf("%.1f%%", (1-*used)*100), Note: "月度剩余额度"}, nil
		}
		var nested map[string]json.RawMessage
		if raw, ok := response["DataV2"]; ok {
			err = json.Unmarshal(raw, &nested)
		} else if raw, ok := response["data"]; ok {
			err = json.Unmarshal(raw, &nested)
		} else {
			break
		}
		if err != nil || nested == nil {
			break
		}
		response = nested
	}
	return Balance{}, errors.New("阿里未返回月度额度，请检查控制台地区与个人版订阅")
}

func (c *Client) mimo(ctx context.Context, p ProviderConfig, key string) (Balance, error) {
	var response struct {
		Object   string `json:"object"`
		PlanCode string `json:"plan_code"`
		Quota    struct {
			Total     *int64 `json:"total_credits"`
			Used      *int64 `json:"used_credits"`
			Remaining *int64 `json:"remaining_credits"`
		} `json:"quota"`
		Period struct {
			End string `json:"end_at"`
		} `json:"period"`
	}
	if err := c.request(ctx, "GET", p.URL, "", nil, map[string]string{"api-key": key}, &response); err != nil {
		return Balance{}, err
	}
	q := response.Quota
	if response.Object != "token_plan" || q.Total == nil || q.Used == nil || q.Remaining == nil || *q.Total < 0 || *q.Used < 0 || *q.Remaining < 0 || *q.Remaining > *q.Total {
		return Balance{}, errors.New("MiMo 未返回有效 Token Plan Credits")
	}
	detail := fmt.Sprintf("剩余 %d / %d Credits", *q.Remaining, *q.Total)
	if *q.Total > 0 {
		detail += fmt.Sprintf(" · %.1f%%", float64(*q.Remaining)*100/float64(*q.Total))
	}
	if response.PlanCode != "" {
		detail += " · " + shortName(response.PlanCode)
	}
	if response.Period.End != "" {
		detail += " · 周期结束 " + shortName(response.Period.End)
	}
	note := "Credits 剩余"
	if *q.Total > 0 {
		note += fmt.Sprintf(" · %.1f%%", float64(*q.Remaining)*100/float64(*q.Total))
	}
	return Balance{Detail: detail, Value: CompactNumber(float64(*q.Remaining)), Note: note}, nil
}
