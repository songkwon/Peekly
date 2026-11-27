package capture

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type authority struct {
	cert   *x509.Certificate
	key    *ecdsa.PrivateKey
	public []byte
	mu     sync.Mutex
	leaves map[string]tls.Certificate
}

type Certificate struct {
	PEM         string `json:"pem"`
	Fingerprint string `json:"fingerprint"`
}

func newSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func openAuthority(dir string) (*authority, error) {
	path := filepath.Join(dir, "authority.pem")
	bundle, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		bundle, err = createRoot()
		if err != nil {
			return nil, err
		}
		// Publish the complete bundle atomically. Another instance may win the race.
		temp, err := os.CreateTemp(dir, "authority-*")
		if err != nil {
			return nil, err
		}
		defer os.Remove(temp.Name())
		_, writeErr := temp.Write(bundle)
		if err := errors.Join(writeErr, temp.Close()); err != nil {
			return nil, err
		}
		if err = os.Link(temp.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		bundle, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(bundle)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid Peekly root certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	keyBlock, _ := pem.Decode(rest)
	if keyBlock == nil || keyBlock.Type != "EC PRIVATE KEY" {
		return nil, errors.New("invalid Peekly root key")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	publicKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	now := time.Now()
	if !ok || !publicKey.Equal(&key.PublicKey) || !cert.IsCA || now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return nil, errors.New("Peekly root certificate is expired or does not match its key")
	}
	return &authority{cert: cert, key: key, public: pem.EncodeToMemory(block), leaves: make(map[string]tls.Certificate)}, nil
}

func createRoot() ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := newSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	root := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "Peekly Local Root CA", Organization: []string{"Peekly"}},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true, IsCA: true, MaxPathLenZero: true,
	}
	raw, err := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	keyRaw, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyRaw})...), nil
}

func (a *authority) info() Certificate {
	sum := sha256.Sum256(a.cert.Raw)
	hexBytes := make([]string, len(sum))
	for i, b := range sum {
		hexBytes[i] = strings.ToUpper(hex.EncodeToString([]byte{b}))
	}
	return Certificate{PEM: string(a.public), Fingerprint: strings.Join(hexBytes, ":")}
}

func (a *authority) leaf(host string) (tls.Certificate, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if cached, ok := a.leaves[host]; ok && cached.Leaf.NotAfter.After(now.Add(time.Hour)) {
		return cached, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := newSerial()
	if err != nil {
		return tls.Certificate{}, err
	}
	leaf := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: host},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(7 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		leaf.IPAddresses = []net.IP{ip}
	} else {
		leaf.DNSNames = []string{host}
	}
	raw, err := x509.CreateCertificate(rand.Reader, leaf, a.cert, &key.PublicKey, a.key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("sign leaf for %s: %w", host, err)
	}
	parsed, err := x509.ParseCertificate(raw)
	if err != nil {
		return tls.Certificate{}, err
	}
	if len(a.leaves) >= 256 {
		clear(a.leaves)
	}
	result := tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key, Leaf: parsed}
	a.leaves[host] = result
	return result, nil
}
