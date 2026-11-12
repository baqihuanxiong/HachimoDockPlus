package clock

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestSNTPValidatesResponseAndSupportsEraRollover(t *testing.T) {
	for _, year := range []int{2026, 2040} {
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		want := time.Date(year, 11, 12, 13, 0, 0, 0, time.UTC)
		go func() {
			var in [48]byte
			_, addr, e := conn.ReadFrom(in[:])
			if e != nil {
				return
			}
			var out [48]byte
			out[0] = 0x24
			out[1] = 2
			copy(out[24:32], in[40:48])
			binary.BigEndian.PutUint32(out[40:], uint32(want.Unix()+ntpEpoch))
			conn.WriteTo(out[:], addr)
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		got, err := Query(ctx, conn.LocalAddr().String())
		cancel()
		conn.Close()
		if err != nil || !got.Equal(want) {
			t.Fatalf("year=%d got=%v err=%v", year, got, err)
		}
	}
}

func TestSNTPRejectsUnrelatedResponse(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go func() {
		var in [48]byte
		_, addr, e := conn.ReadFrom(in[:])
		if e != nil {
			return
		}
		var out [48]byte
		out[0] = 0x24
		out[1] = 2
		conn.WriteTo(out[:], addr)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := Query(ctx, conn.LocalAddr().String()); err == nil {
		t.Fatal("unrelated NTP response accepted")
	}
}
