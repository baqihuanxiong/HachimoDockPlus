package desktoplan

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"math/big"
	"net"
	"time"
)

const srpBytes = 384

// RFC 5054 3072-bit group used by ESP_NG_3072 and the official Desktop client.
const srpPrime = "ffffffffffffffffc90fdaa22168c234c4c6628b80dc1cd129024e088a67cc74020bbea63b139b22514a08798e3404ddef9519b3cd3a431b302b0a6df25f14374fe1356d6d51c245e485b576625e7ec6f44c42e9a637ed6b0bff5cb6f406b7edee386bfb5a899fa5ae9f24117c4b1fe649286651ece45b3dc2007cb8a163bf0598da48361c55d39a69163fa8fd24cf5f83655d23dca3ad961c62f356208552bb9ed529077096966d670c354e4abc9804f1746c08ca18217c32905e462e36ce3be39e772c180e86039b2783a2ec07a28fb5c55df06f4c52c9de2bcbf6955817183995497cea956ae515d2261898fa051015728e5a8aaac42dad33170d04507a33a85521abdf1cba64ecfb850458dbef0a8aea71575d060c7db3970f85a6e1e4c7abf5ae8cdb0933d71e8c94e04a25619dcee3d2261ad2ee6bf12ffa06d98a0864d87602733ec86a64521f2b18177b200cbbe117577a615d6c770988c0bad946e208e24fa074e5ab3143db5bfce0fd108e4b82d120a93ad2caffffffffffffffff"

func hash512(parts ...[]byte) []byte {
	h := sha512.New()
	for _, part := range parts {
		h.Write(part)
	}
	return h.Sum(nil)
}
func number(b []byte) *big.Int { return new(big.Int).SetBytes(b) }

func (s *Server) pair(conn net.Conn, transcript []byte) {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	s.mu.Lock()
	if s.binding.Key != "" || s.pairing {
		s.mu.Unlock()
		return
	}
	s.pairing = true
	generation := s.generation
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.pairing = false; s.mu.Unlock() }()
	publicA := make([]byte, srpBytes)
	if _, err := io.ReadFull(conn, publicA); err != nil {
		return
	}
	prime, _ := new(big.Int).SetString(srpPrime, 16)
	a := number(publicA)
	if publicA[0]&0x80 != 0 || a.Cmp(big.NewInt(2)) < 0 || a.Cmp(prime) >= 0 {
		return
	}
	private := make([]byte, 32)
	salt := make([]byte, 16)
	if _, err := rand.Read(private); err != nil {
		return
	}
	defer clear(private)
	if _, err := rand.Read(salt); err != nil {
		return
	}
	g := big.NewInt(5)
	paddedG := make([]byte, srpBytes)
	paddedG[srpBytes-1] = 5
	k := number(hash512(prime.Bytes(), paddedG))
	x := number(hash512(salt, hash512([]byte("Hachimodock:Hachimodock-auto-v1"))))
	v := new(big.Int).Exp(g, x, prime)
	b := number(private)
	publicB := new(big.Int).Mul(k, v)
	publicB.Add(publicB, new(big.Int).Exp(g, b, prime)).Mod(publicB, prime)
	encodedB := publicB.Bytes()
	if len(encodedB) == 0 {
		return
	}
	packet := append(append(append([]byte(nil), salt...), byte(len(encodedB)>>8), byte(len(encodedB))), encodedB...)
	if _, err := conn.Write(packet); err != nil {
		return
	}
	paddedB := make([]byte, srpBytes)
	publicB.FillBytes(paddedB)
	u := number(hash512(publicA, paddedB))
	if u.Sign() == 0 {
		return
	}
	shared := new(big.Int).Exp(v, u, prime)
	shared.Mul(shared, a).Mod(shared, prime)
	shared.Exp(shared, b, prime)
	secret := hash512(shared.Bytes())
	defer clear(secret)
	group := hash512(prime.Bytes())
	other := hash512(paddedG)
	for i := range group {
		group[i] ^= other[i]
	}
	proof := hash512(group, hash512([]byte("Hachimodock")), salt, publicA, encodedB, secret)
	received := make([]byte, 64)
	if _, err := io.ReadFull(conn, received); err != nil || subtle.ConstantTimeCompare(proof, received) != 1 {
		return
	}
	h := hmac.New(sha256.New, secret)
	h.Write(transcript)
	master := h.Sum(nil)
	defer clear(master)
	s.mu.Lock()
	if generation != s.generation || s.binding.Key != "" {
		s.mu.Unlock()
		return
	}
	next := binding{ID: s.binding.ID, Key: hex.EncodeToString(master[:16])}
	if err := s.save(next); err != nil {
		s.state.Error = "桌宠配对保存失败"
		s.mu.Unlock()
		return
	}
	s.binding = next
	s.state.Paired = true
	s.generation++
	s.mu.Unlock()
	// Persist before the proof: the desktop can reconcile a lost acknowledgement
	// using a read-only authenticated probe instead of repeating enrollment.
	_, _ = conn.Write(hash512(publicA, proof, secret))
}
