package desktoplan

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func startTestServer(t *testing.T, path string, apply func([]byte, time.Time) error) *Server {
	t.Helper()
	s := New(Options{Path: path, Address: "127.0.0.1:0", DiscoveryAddress: "127.0.0.1:0", Apply: apply})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}
func testLink(address string) (net.Conn, []byte, error) {
	c, err := net.DialTimeout("tcp4", address, time.Second)
	if err != nil {
		return nil, nil, err
	}
	c.SetDeadline(time.Now().Add(5 * time.Second))
	transcript := make([]byte, 38)
	copy(transcript, "P4LAN1")
	rand.Read(transcript[6:22])
	if _, err = c.Write(transcript[6:22]); err == nil {
		_, err = io.ReadFull(c, transcript[22:])
	}
	return c, transcript, err
}
func testExchange(address string, key []byte, mode byte, body []byte, corrupt bool) ([]byte, error) {
	c, transcript, err := testLink(address)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	aead, err := sessionAEAD(key, transcript)
	if err != nil {
		return nil, err
	}
	header := []byte{mode, byte(len(body) >> 8), byte(len(body))}
	nonce := make([]byte, 12)
	sealed := aead.Seal(nil, nonce, body, header)
	if corrupt {
		sealed[len(sealed)-1] ^= 1
	}
	if _, err = c.Write(append(header, sealed...)); err != nil {
		return nil, err
	}
	aad := append([]byte(nil), header...)
	size := 0
	if mode == 6 {
		length := make([]byte, 2)
		if _, err = io.ReadFull(c, length); err != nil {
			return nil, err
		}
		size = int(length[0])<<8 | int(length[1])
		aad = append(aad, length...)
	}
	if size > 576 {
		return nil, io.ErrShortBuffer
	}
	response := make([]byte, size+16)
	if _, err = io.ReadFull(c, response); err != nil {
		return nil, err
	}
	nonce[0] = 1
	return aead.Open(nil, nonce, response, aad)
}

// Same client-side SRP wire contract as Desktop. This exercises enrollment,
// not just matching helpers: no binding is committed before proof verification.
func testEnroll(t *testing.T, address string, badProof bool) []byte {
	t.Helper()
	c, transcript, err := testLink(address)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	prime, _ := new(big.Int).SetString(srpPrime, 16)
	g := big.NewInt(5)
	var a, A *big.Int
	var publicA []byte
	for {
		priv := make([]byte, 32)
		rand.Read(priv)
		priv[0] |= 0x80
		a = number(priv)
		A = new(big.Int).Exp(g, a, prime)
		publicA = A.Bytes()
		if len(publicA) == 384 && publicA[0] < 128 {
			break
		}
	}
	c.Write([]byte{5, 1, 128})
	c.Write(publicA)
	salt := make([]byte, 16)
	if _, err = io.ReadFull(c, salt); err != nil {
		t.Fatal(err)
	}
	length := make([]byte, 2)
	io.ReadFull(c, length)
	size := int(length[0])<<8 | int(length[1])
	if size < 1 || size > 384 {
		t.Fatal("bad B size")
	}
	publicB := make([]byte, size)
	if _, err = io.ReadFull(c, publicB); err != nil {
		t.Fatal(err)
	}
	paddedG := make([]byte, 384)
	paddedG[383] = 5
	paddedB := make([]byte, 384)
	copy(paddedB[384-size:], publicB)
	x := number(hash512(salt, hash512([]byte("Hachimodock:Hachimodock-auto-v1"))))
	k := number(hash512(prime.Bytes(), paddedG))
	u := number(hash512(publicA, paddedB))
	base := new(big.Int).Exp(g, x, prime)
	base.Mul(k, base)
	base.Sub(number(publicB), base)
	base.Mod(base, prime)
	exponent := new(big.Int).Mul(u, x)
	exponent.Add(exponent, a)
	shared := new(big.Int).Exp(base, exponent, prime)
	secret := hash512(shared.Bytes())
	group := hash512(prime.Bytes())
	gh := hash512(paddedG)
	for i := range group {
		group[i] ^= gh[i]
	}
	proof := hash512(group, hash512([]byte("Hachimodock")), salt, publicA, publicB, secret)
	if badProof {
		proof[0] ^= 1
	}
	c.Write(proof)
	response := make([]byte, 64)
	_, err = io.ReadFull(c, response)
	if badProof {
		if err == nil {
			t.Fatal("bad SRP proof accepted")
		}
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(response) != hex.EncodeToString(hash512(publicA, proof, secret)) {
		t.Fatal("bad server proof")
	}
	// Derivation uses the same session KDF, but with the 64-byte SRP secret.
	// Obtain the expected persisted master key independently of server state.
	h := hmac.New(sha256.New, secret)
	h.Write(transcript)
	return append([]byte(nil), h.Sum(nil)[:16]...)
}

func TestDiscoveryEnrollmentAuthenticatedSnapshotsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lan.json")
	applied := make(chan string, 4)
	s := startTestServer(t, path, func(b []byte, _ time.Time) error { applied <- string(b); return nil })
	address := s.listener.Addr().String()
	u, err := net.DialUDP("udp4", nil, s.udp.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	u.SetDeadline(time.Now().Add(time.Second))
	u.Write([]byte("HACHI-DISCOVER-1 0123456789abcdef0123456789abcdef"))
	b := make([]byte, 512)
	n, err := u.Read(b)
	if err != nil {
		t.Fatal(err)
	}
	var discovery map[string]any
	json.Unmarshal(b[:n], &discovery)
	if discovery["ready"] != true || discovery["nonce"] != "0123456789abcdef0123456789abcdef" {
		t.Fatal(discovery)
	}
	testEnroll(t, address, true)
	if s.State().Paired {
		t.Fatal("failed proof saved pairing")
	}
	key := testEnroll(t, address, false)
	if _, err = testExchange(address, key, 0, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err = testExchange(address, key, 2, []byte(`{"source":"codex","status":{"state":"working"}}`), true); err == nil {
		t.Fatal("tampered GCM accepted")
	}
	select {
	case <-applied:
		t.Fatal("tampered snapshot applied")
	default:
	}
	if _, err = testExchange(address, key, 2, []byte(`{"source":"codex","status":{"state":"working"}}`), false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("snapshot missing")
	}
	status, err := testExchange(address, key, 6, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	json.Unmarshal(status, &v)
	if v["boardDeviceId"] != s.State().ID || v["wirelessOta"] == true || v["controlTrusted"] == true {
		t.Fatal(v)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal("insecure pairing file")
	}
	id := s.State().ID
	s.Close()
	restarted := startTestServer(t, path, nil)
	if restarted.State().ID != id {
		t.Fatal("identity changed")
	}
	if _, err = testExchange(restarted.listener.Addr().String(), key, 0, nil, false); err != nil {
		t.Fatal("pairing lost on restart", err)
	}
	if err = restarted.Forget(); err != nil {
		t.Fatal(err)
	}
	if _, err = testExchange(restarted.listener.Addr().String(), key, 0, nil, false); err == nil {
		t.Fatal("forgotten key accepted")
	}
}

func TestCloseCancelsPartialHandshake(t *testing.T) {
	s := startTestServer(t, filepath.Join(t.TempDir(), "lan.json"), nil)
	c, err := net.Dial("tcp4", s.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write([]byte{1})
	before := time.Now()
	s.Close()
	if time.Since(before) > time.Second {
		t.Fatal("close waited for IO timeout")
	}
}
