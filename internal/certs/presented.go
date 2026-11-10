package certs

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// Presented is what a TLS handshake with the server returned.
type Presented struct {
	Address string        `json:"address"`
	Version string        `json:"version" doc:"Negotiated TLS version"`
	Cipher  string        `json:"cipher"`
	Chain   []Certificate `json:"chain" doc:"Server certificate first"`
}

// Handshake connects to an LDAPS address and reports the certificates the
// server presents. The chain is not verified: the point is to see it.
func Handshake(ctx context.Context, addr string) (*Presented, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	d := tls.Dialer{NetDialer: &net.Dialer{}, Config: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS10}} //nolint:gosec // inspection only
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("TLS handshake with %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	st := conn.(*tls.Conn).ConnectionState()
	p := &Presented{Address: addr, Version: tls.VersionName(st.Version), Cipher: tls.CipherSuiteName(st.CipherSuite), Chain: []Certificate{}}
	for _, c := range st.PeerCertificates {
		d, err := Describe(c.Raw)
		if err != nil {
			return nil, err
		}
		d.PEM = ""
		p.Chain = append(p.Chain, *d)
	}
	return p, nil
}
