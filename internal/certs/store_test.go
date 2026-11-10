package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"
)

func TestDescribe(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	subject := pkix.Name{CommonName: "ldap.example.org", Organization: []string{"Example"},
		ExtraNames: []pkix.AttributeTypeAndValue{{Type: asn1.ObjectIdentifier{2, 5, 4, 42}, Value: "Ann"}}}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(255), Subject: subject, DNSNames: []string{"ldap.example.org"},
		NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Describe(der)
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "givenName=Ann,CN=ldap.example.org,O=Example" && c.Subject != "CN=ldap.example.org,givenName=Ann,O=Example" {
		t.Errorf("subject = %q", c.Subject)
	}
	if !c.SelfSigned || c.KeyType != "ECDSA" || c.Serial != "ff" || len(c.SHA256) != 64 || c.DNSNames[0] != "ldap.example.org" {
		t.Errorf("%+v", c)
	}
	if _, err := Describe([]byte("junk")); err == nil {
		t.Error("junk accepted")
	}
}

func TestListParsing(t *testing.T) {
	for line, want := range map[string][2]string{
		"Self-Signed-CA                                               CT,, ": {"Self-Signed-CA", "CT,,"},
		"Server-Cert                                                  u,u,u": {"Server-Cert", "u,u,u"},
		"my ca with spaces                                            ,,":    {"my ca with spaces", ",,"},
	} {
		m := listLine.FindStringSubmatch(line)
		if m == nil || m[1] != want[0] || m[2] != want[1] {
			t.Errorf("%q: %v", line, m)
		}
	}
	if m := keyLine.FindStringSubmatch("< 0> rsa      c9c1defb57a5   NSS Certificate DB:Server-Cert"); m == nil || m[1] != "Server-Cert" {
		t.Errorf("key line: %v", m)
	}
}
