package node

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"gorm.io/gorm"
)

// Master certificate authority. Every child node trusts only this CA, and the master
// presents one client certificate issued by it; both live in the settings table.
const (
	keyCACrt     = "NodeCACrt"
	keyCAKey     = "NodeCAKey"
	keyClientCrt = "NodeClientCrt"
	keyClientKey = "NodeClientKey"
	certLifetime = 10 * 365 * 24 * time.Hour
)

type authority struct {
	caCrt     *x509.Certificate
	caKey     *ecdsa.PrivateKey
	caPEM     []byte
	clientCrt []byte
	clientKey []byte
}

var (
	caMu     sync.Mutex
	caCached *authority
)

func encodeKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
}

func decodeCert(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid certificate pem")
	}
	return x509.ParseCertificate(block.Bytes)
}

func decodeKey(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid key pem")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func serial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	return n
}

// loadAuthority returns the master CA, creating it together with the client certificate on first use.
func loadAuthority() (*authority, error) {
	caMu.Lock()
	defer caMu.Unlock()
	if caCached != nil {
		return caCached, nil
	}
	settings := repo.NewISettingRepo()
	readSetting := func(key string) (string, error) {
		v, err := settings.GetValueByKey(key)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("read setting %s failed: %w", key, err)
		}
		return v, nil
	}
	rawCACrt, err := readSetting(keyCACrt)
	if err != nil {
		return nil, err
	}
	rawCAKey, err := readSetting(keyCAKey)
	if err != nil {
		return nil, err
	}
	rawClientCrt, err := readSetting(keyClientCrt)
	if err != nil {
		return nil, err
	}
	rawClientKey, err := readSetting(keyClientKey)
	if err != nil {
		return nil, err
	}
	if len(rawCACrt) > 0 || len(rawCAKey) > 0 || len(rawClientCrt) > 0 || len(rawClientKey) > 0 {
		if len(rawCACrt) == 0 || len(rawCAKey) == 0 || len(rawClientCrt) == 0 || len(rawClientKey) == 0 {
			return nil, errors.New("incomplete master CA credentials in settings")
		}
		caKeyPlain, err := encrypt.StringDecrypt(rawCAKey)
		if err != nil || len(caKeyPlain) == 0 {
			return nil, fmt.Errorf("decrypt master CA key failed: %w", err)
		}
		clientKeyPlain, err := encrypt.StringDecrypt(rawClientKey)
		if err != nil || len(clientKeyPlain) == 0 {
			return nil, fmt.Errorf("decrypt master client key failed: %w", err)
		}
		caCert, err := decodeCert([]byte(rawCACrt))
		if err != nil {
			return nil, fmt.Errorf("parse master CA cert failed: %w", err)
		}
		caKey, err := decodeKey([]byte(caKeyPlain))
		if err != nil {
			return nil, fmt.Errorf("parse master CA key failed: %w", err)
		}
		if _, err := decodeCert([]byte(rawClientCrt)); err != nil {
			return nil, fmt.Errorf("parse master client cert failed: %w", err)
		}
		if _, err := decodeKey([]byte(clientKeyPlain)); err != nil {
			return nil, fmt.Errorf("parse master client key failed: %w", err)
		}
		caCached = &authority{
			caCrt:     caCert,
			caKey:     caKey,
			caPEM:     []byte(rawCACrt),
			clientCrt: []byte(rawClientCrt),
			clientKey: []byte(clientKeyPlain),
		}
		return caCached, nil
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "1Panel Master CA", Organization: []string{"1Panel"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(certLifetime),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	caCert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	a := &authority{caCrt: caCert, caKey: caKey, caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
	a.clientCrt, a.clientKey, err = a.issue("1panel-master", nil, x509.ExtKeyUsageClientAuth)
	if err != nil {
		return nil, err
	}
	caKeyPEM, err := encodeKey(caKey)
	if err != nil {
		return nil, err
	}
	save := func(key string, value []byte, secret bool) error {
		v := string(value)
		if secret {
			enc, err := encrypt.StringEncrypt(v)
			if err != nil {
				return err
			}
			v = enc
		}
		return settings.UpdateOrCreate(key, v)
	}
	for _, item := range []struct {
		key    string
		value  []byte
		secret bool
	}{{keyCACrt, a.caPEM, false}, {keyCAKey, caKeyPEM, true}, {keyClientCrt, a.clientCrt, false}, {keyClientKey, a.clientKey, true}} {
		if err := save(item.key, item.value, item.secret); err != nil {
			return nil, err
		}
	}
	caCached = a
	return a, nil
}

// issue signs a leaf certificate; hosts become IP or DNS subject alternative names.
func (a *authority) issue(cn string, hosts []string, usage x509.ExtKeyUsage) (crt, key []byte, err error) {
	leaf, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"1Panel"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(certLifetime),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else if h != "" {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, a.caCrt, &leaf.PublicKey, a.caKey)
	if err != nil {
		return nil, nil, err
	}
	key, err = encodeKey(leaf)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), key, nil
}
