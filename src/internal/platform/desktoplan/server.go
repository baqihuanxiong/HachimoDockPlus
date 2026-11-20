// Package desktoplan implements HachimoDock Desktop's paired LAN presentation
// transport. Protocol/SRP adaptation: HachimoDock, Copyright (C) 2026 Yizheng
// Wang, upstream LICENSE applies to derived portions. See THIRD_PARTY_NOTICES.md.
package desktoplan

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const TCPPort, DiscoveryPort = 29461, 29462
const maxBody = 16384

type Options struct {
	Path                      string
	Address, DiscoveryAddress string
	Apply                     func([]byte, time.Time) error
	Probe                     func(time.Time)
	Clear                     func()
}
type binding struct {
	ID  string `json:"id"`
	Key string `json:"key,omitempty"`
}
type State struct {
	Running, Paired bool
	ID, Error       string
}
type Server struct {
	mu          sync.Mutex
	options     Options
	binding     binding
	boot        string
	state       State
	pairing     bool
	generation  uint64
	listener    net.Listener
	udp         *net.UDPConn
	connections map[net.Conn]bool
	wg          sync.WaitGroup
	cancel      context.CancelFunc
}

func New(options Options) *Server {
	if options.Address == "" {
		options.Address = ":29461"
	}
	if options.DiscoveryAddress == "" {
		options.DiscoveryAddress = ":29462"
	}
	return &Server{options: options, connections: make(map[net.Conn]bool)}
}
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func validHex(s string, bytes int) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == bytes && s == strings.ToLower(s)
}

func (s *Server) load() error {
	data, err := os.ReadFile(s.options.Path)
	if errors.Is(err, os.ErrNotExist) {
		id, err := randomHex(6)
		if err != nil {
			return err
		}
		s.binding.ID = "p4-" + id // Required by Desktop's current discovery parser.
		return s.save(s.binding)
	}
	if err != nil {
		return err
	}
	st, err := os.Lstat(s.options.Path)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || len(data) > 4096 || json.Unmarshal(data, &s.binding) != nil || !strings.HasPrefix(s.binding.ID, "p4-") || !validHex(strings.TrimPrefix(s.binding.ID, "p4-"), 6) || s.binding.Key != "" && !validHex(s.binding.Key, 16) {
		return errors.New("桌宠配对记录无效，请检查私有数据文件")
	}
	return nil
}
func (s *Server) save(next binding) error {
	dir := filepath.Dir(s.options.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".lan-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(next)
	if err == nil {
		err = f.Sync()
	}
	closed := f.Close()
	if err == nil {
		err = closed
	}
	if err == nil {
		err = os.Rename(f.Name(), s.options.Path)
	}
	if err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (s *Server) Start(parent context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Running {
		return nil
	}
	fail := func(err error) error { s.state.Error = err.Error(); return err }
	if err := s.load(); err != nil {
		return fail(err)
	}
	boot, err := randomHex(8)
	if err != nil {
		return fail(err)
	}
	s.boot = boot
	listener, err := net.Listen("tcp4", s.options.Address)
	if err != nil {
		return fail(errors.New("桌宠 Wi-Fi 服务端口不可用"))
	}
	address, err := net.ResolveUDPAddr("udp4", s.options.DiscoveryAddress)
	if err != nil {
		listener.Close()
		return fail(err)
	}
	udp, err := net.ListenUDP("udp4", address)
	if err != nil {
		listener.Close()
		return fail(errors.New("桌宠 Wi-Fi 发现端口不可用"))
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel, s.listener, s.udp = cancel, listener, udp
	s.state = State{Running: true, Paired: s.binding.Key != "", ID: s.binding.ID}
	s.wg.Add(3)
	go s.accept(ctx, listener)
	go s.discover(ctx, udp)
	go func() {
		defer s.wg.Done()
		<-ctx.Done()
		s.mu.Lock()
		listener.Close()
		udp.Close()
		for conn := range s.connections {
			conn.Close()
		}
		s.state.Running = false
		s.generation++
		s.mu.Unlock()
	}()
	return nil
}
func (s *Server) State() State { s.mu.Lock(); defer s.mu.Unlock(); return s.state }
func (s *Server) Close() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		s.wg.Wait()
	}
}
func (s *Server) Forget() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.binding.ID == "" {
		return nil
	}
	next := binding{ID: s.binding.ID}
	if err := s.save(next); err != nil {
		return errors.New("解除配对保存失败")
	}
	s.binding = next
	s.state.Paired = false
	s.generation++
	for conn := range s.connections {
		conn.Close()
	}
	if s.options.Clear != nil {
		s.options.Clear()
	}
	return nil
}

// Match the upstream same-subnet admission rule. There is no Internet listener
// API or unauthenticated presentation path, even when bound on all interfaces.
func localPeer(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, address := range addresses {
		if network, ok := address.(*net.IPNet); ok && !network.IP.IsLoopback() && network.Contains(ip) {
			return true
		}
	}
	return false
}
func (s *Server) accept(ctx context.Context, listener net.Listener) {
	defer s.wg.Done()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if ctx.Err() != nil || len(s.connections) >= 4 || !localPeer(conn.RemoteAddr().(*net.TCPAddr).IP) {
			s.mu.Unlock()
			conn.Close()
			continue
		}
		s.connections[conn] = true
		s.wg.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.wg.Done()
			defer func() { conn.Close(); s.mu.Lock(); delete(s.connections, conn); s.mu.Unlock() }()
			s.handle(conn)
		}()
	}
}
func (s *Server) discover(ctx context.Context, conn *net.UDPConn) {
	defer s.wg.Done()
	buffer := make([]byte, 512)
	for ctx.Err() == nil {
		n, peer, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		const prefix = "HACHI-DISCOVER-1 "
		if !localPeer(peer.IP) || n != len(prefix)+32 || string(buffer[:len(prefix)]) != prefix {
			continue
		}
		nonce := string(buffer[len(prefix):n])
		if !validHex(nonce, 16) {
			continue
		}
		s.mu.Lock()
		paired := s.binding.Key != ""
		ready := !paired && !s.pairing
		id := s.binding.ID
		s.mu.Unlock()
		data, _ := json.Marshal(map[string]any{"protocol": 2, "pairing": "auto-unpaired-v1", "nonce": nonce, "id": id, "ready": ready, "paired": paired})
		_, _ = conn.WriteToUDP(data, peer)
	}
}

func sessionAEAD(master, transcript []byte) (cipher.AEAD, error) {
	h := hmac.New(sha256.New, master)
	h.Write(transcript)
	block, err := aes.NewCipher(h.Sum(nil)[:16])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (s *Server) handle(conn net.Conn) {
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	transcript := make([]byte, 6+32)
	copy(transcript, "P4LAN1")
	if _, err := io.ReadFull(conn, transcript[6:22]); err != nil {
		return
	}
	if _, err := rand.Read(transcript[22:]); err != nil {
		return
	}
	if _, err := conn.Write(transcript[22:]); err != nil {
		return
	}
	header := make([]byte, 3)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	mode, size := header[0], int(header[1])<<8|int(header[2])
	if mode == 5 && size == srpBytes {
		s.pair(conn, transcript)
		return
	}
	if mode != 0 && mode != 2 && mode != 6 || (mode == 0 || mode == 6) && size != 0 || mode == 2 && (size == 0 || size > maxBody) {
		return
	}
	s.mu.Lock()
	master, _ := hex.DecodeString(s.binding.Key)
	generation := s.generation
	s.mu.Unlock()
	if len(master) != 16 {
		return
	}
	aead, err := sessionAEAD(master, transcript)
	clear(master)
	if err != nil {
		return
	}
	ciphertext := make([]byte, size+aead.Overhead())
	if _, err = io.ReadFull(conn, ciphertext); err != nil {
		return
	}
	nonce := make([]byte, 12)
	body, err := aead.Open(nil, nonce, ciphertext, header)
	if err != nil {
		return
	}
	defer clear(body)
	s.mu.Lock()
	if generation != s.generation || s.binding.Key == "" {
		s.mu.Unlock()
		return
	}
	if mode == 2 && s.options.Apply != nil {
		if err = s.options.Apply(body, time.Now()); err != nil {
			s.mu.Unlock()
			return
		}
	}
	if (mode == 0 || mode == 2) && s.options.Probe != nil {
		s.options.Probe(time.Now())
	}
	nonce[0] = 1
	var response []byte
	aad := append([]byte(nil), header...)
	if mode == 6 {
		// Do not claim controls, firmware update, components, microphone, or
		// wired trust: this service only accepts Agent presentation snapshots.
		response, _ = json.Marshal(map[string]any{"firmware": "linux-go-lvgl-0.1.0", "boardDeviceId": s.binding.ID, "bootId": s.boot, "wifiConnected": true, "clockSynchronized": time.Now().Year() >= 2024, "httpsSuccesses": 0, "httpsFailures": 0})
		aad = append(aad, byte(len(response)>>8), byte(len(response)))
	}
	s.mu.Unlock()
	sealed := aead.Seal(nil, nonce, response, aad)
	if mode == 6 {
		_, err = conn.Write(aad[3:])
		if err != nil {
			return
		}
	}
	_, _ = conn.Write(sealed)
}
