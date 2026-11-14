package netease

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

var mainlandPhone = regexp.MustCompile(`^1[3-9][0-9]{9}$`)
var smsCaptcha = regexp.MustCompile(`^[0-9]{4,6}$`)
var ErrLoginRestricted = errors.New("网易云暂时限制登录（8821），请稍后重试")

func ValidatePhone(phone string) error {
	if !mainlandPhone.MatchString(phone) {
		return errors.New("请输入 11 位中国大陆手机号")
	}
	return nil
}
func ValidateCaptcha(captcha string) error {
	if !smsCaptcha.MatchString(captcha) {
		return errors.New("请输入 4～6 位短信验证码")
	}
	return nil
}
func smsError(code int, err error) error {
	switch code {
	case 400:
		return errors.New("手机号或验证码格式错误")
	case 405, 415:
		return errors.New("短信请求过于频繁，请稍后再试")
	case 501:
		return errors.New("该手机号尚未注册网易云音乐")
	case 502:
		return errors.New("短信验证码错误，请重新输入")
	case 503:
		return errors.New("短信验证码已失效，请重新获取")
	}
	return err
}
func (c *Client) SendSMS(ctx context.Context, phone string) error {
	if err := ValidatePhone(phone); err != nil {
		return err
	}
	var result struct{ Data *bool }
	code, err := c.request(ctx, "/api/sms/captcha/sent", map[string]any{"ctcode": "86", "secrete": "music_middleuser_pclogin", "cellphone": phone}, &result)
	if err != nil {
		if code == 502 {
			return errors.New("短信未能发送，请稍后再试")
		}
		return smsError(code, err)
	}
	if result.Data != nil && !*result.Data {
		return errors.New("短信发送失败，请稍后重试")
	}
	return nil
}
func (c *Client) LoginSMS(ctx context.Context, phone, captcha string) (Profile, error) {
	if err := ValidatePhone(phone); err != nil {
		return Profile{}, err
	}
	if err := ValidateCaptcha(captcha); err != nil {
		return Profile{}, err
	}
	var result struct{ Profile Profile }
	code, err := c.request(ctx, "/api/w/login/cellphone", map[string]any{"type": "1", "https": "true", "phone": phone, "countrycode": "86", "captcha": captcha, "remember": "true", "secureCaptcha": ""}, &result)
	if err != nil {
		return Profile{}, smsError(code, err)
	}
	if result.Profile.ID == 0 {
		result.Profile, err = c.Account(ctx)
		if err != nil {
			return Profile{}, err
		}
	}
	if result.Profile.ID == 0 || !c.authenticated() {
		return Profile{}, errors.New("网易云未返回有效登录凭据，请重试")
	}
	if ctx.Err() != nil {
		return Profile{}, ctx.Err()
	}
	if err = c.save(); err != nil {
		return Profile{}, errors.New("登录成功，但保存凭据失败")
	}
	return result.Profile, nil
}
func (c *Client) authenticated() bool {
	for _, cookie := range c.http.Jar.Cookies(origin) {
		if cookie.Name == "MUSIC_U" && strings.TrimSpace(cookie.Value) != "" {
			return true
		}
	}
	return false
}
