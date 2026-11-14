// Package netease implements the web client's encrypted API without a Node proxy.
package netease

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/url"
)

func encrypt(data map[string]any) (url.Values, error) {
	plain, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	key := make([]byte, 16)
	if _, err = rand.Read(key); err != nil {
		return nil, err
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for i := range key {
		key[i] = alphabet[int(key[i])%len(alphabet)]
	}
	cbc := func(in, key []byte) string {
		block, _ := aes.NewCipher(key)
		pad := aes.BlockSize - len(in)%aes.BlockSize
		in = append(in, bytes.Repeat([]byte{byte(pad)}, pad)...)
		out := make([]byte, len(in))
		cipher.NewCBCEncrypter(block, []byte("0102030405060708")).CryptBlocks(out, in)
		return base64.StdEncoding.EncodeToString(out)
	}
	params := cbc([]byte(cbc(plain, []byte("0CoJUm6Qyw8W8jud"))), key)
	der, _ := base64.StdEncoding.DecodeString("MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDgtQn2JZ34ZC28NWYpAUd98iZ37BUrX/aKzmFbt7clFSs6sXqHauqKWqdtLkF2KexO40H1YTX8z2lSgBBOAxLsvaklV8k4cBFK9snQXE9/DDaFt6Rr7iVZMldczhC0JNgTz+SHXT6CBHuX3e9SdB1Ua44oncaTWz7OBGLbCiK45wIDAQAB")
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	public := parsed.(*rsa.PublicKey)
	for i, j := 0, len(key)-1; i < j; i, j = i+1, j-1 {
		key[i], key[j] = key[j], key[i]
	}
	number := new(big.Int).SetBytes(key)
	encoded := new(big.Int).Exp(number, big.NewInt(int64(public.E)), public.N).FillBytes(make([]byte, 128))
	return url.Values{"params": {params}, "encSecKey": {hex.EncodeToString(encoded)}}, nil
}
