// Package clock supplies an explicit SNTP bootstrap command for boards without
// a battery-backed RTC. Normal application requests never disable TLS checks.
package clock

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"time"
)

const ntpEpoch = 2208988800

func Query(ctx context.Context, address string) (time.Time, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", address)
	if err != nil {
		return time.Time{}, errors.New("NTP 服务器连接失败")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(4 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	var packet [48]byte
	packet[0] = 0x23
	now := time.Now()
	binary.BigEndian.PutUint32(packet[40:], uint32(now.Unix()+ntpEpoch))
	binary.BigEndian.PutUint32(packet[44:], uint32((uint64(now.Nanosecond())<<32)/1_000_000_000))
	if _, err = conn.Write(packet[:]); err != nil {
		return time.Time{}, errors.New("NTP 请求发送失败")
	}
	var response [512]byte
	n, err := conn.Read(response[:])
	if err != nil {
		return time.Time{}, errors.New("NTP 查询超时或失败")
	}
	if n < 48 || response[0]&7 != 4 || response[0]>>6 == 3 || response[1] == 0 || response[1] > 15 || string(response[24:32]) != string(packet[40:48]) {
		return time.Time{}, errors.New("NTP 响应无效")
	}
	seconds := int64(binary.BigEndian.Uint32(response[40:44])) - ntpEpoch
	if seconds < 1704067200 {
		seconds += 1 << 32
	} // Resolve the 2036 NTP era rollover.
	fraction := uint64(binary.BigEndian.Uint32(response[44:48]))
	result := time.Unix(seconds, int64(fraction*1_000_000_000>>32)).UTC()
	if result.Year() < 2024 || result.Year() > 2100 {
		return time.Time{}, errors.New("NTP 返回异常时间")
	}
	return result, nil
}
