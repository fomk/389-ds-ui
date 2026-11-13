package ds

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

var (
	// ErrEncryptedValues protects encrypted data: once the server stops
	// encrypting an attribute it returns the stored ciphertext as the value.
	ErrEncryptedValues = errors.New("the attribute has encrypted values")
	// ErrEncryptionKey protects attribute encryption: its key is wrapped with
	// the key of the server certificate, and the server does not start when
	// that key changes.
	ErrEncryptionKey = errors.New("attribute encryption needs the current server key")
)

// EncryptedAttr is an attribute the backend stores encrypted.
type EncryptedAttr struct {
	Attr      string `json:"attr"`
	Algorithm string `json:"algorithm" enum:"AES,3DES"`
}

func encryptedAttrsDN(backend string) string { return "cn=encrypted attributes," + backendDN(backend) }

// ListEncryptedAttrs returns the encrypted attributes of a backend.
func ListEncryptedAttrs(c *ldap.Conn, backend string) ([]EncryptedAttr, error) {
	if _, err := GetBackend(c, backend); err != nil {
		return nil, err
	}
	res, err := c.Search(ldap.NewSearchRequest(encryptedAttrsDN(backend), ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=nsAttributeEncryption)", []string{"cn", "nsEncryptionAlgorithm"}, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return []EncryptedAttr{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]EncryptedAttr, 0, len(res.Entries))
	for _, e := range res.Entries {
		out = append(out, EncryptedAttr{Attr: e.GetAttributeValue("cn"), Algorithm: e.GetAttributeValue("nsEncryptionAlgorithm")})
	}
	slices.SortFunc(out, func(a, b EncryptedAttr) int { return strings.Compare(strings.ToLower(a.Attr), strings.ToLower(b.Attr)) })
	return out, nil
}

// AddEncryptedAttr makes the backend encrypt attr with AES from now on;
// values stored earlier stay as they are until the backend is re-imported.
func AddEncryptedAttr(c *ldap.Conn, backend, attr string) error {
	if !attrRE.MatchString(attr) {
		return fmt.Errorf("%w: attribute %q", ErrInvalidName, attr)
	}
	if _, err := GetBackend(c, backend); err != nil {
		return err
	}
	// The container usually exists; older or hand-made backends may lack it.
	box := ldap.NewAddRequest(encryptedAttrsDN(backend), nil)
	box.Attribute("objectClass", []string{"top", "extensibleObject"})
	box.Attribute("cn", []string{"encrypted attributes"})
	if err := c.Add(box); err != nil && !ldap.IsErrorWithCode(err, ldap.LDAPResultEntryAlreadyExists) {
		return err
	}
	req := ldap.NewAddRequest("cn="+ldap.EscapeDN(attr)+","+encryptedAttrsDN(backend), nil)
	req.Attribute("objectClass", []string{"top", "nsAttributeEncryption"})
	req.Attribute("cn", []string{attr})
	req.Attribute("nsEncryptionAlgorithm", []string{"AES"})
	if err := c.Add(req); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultEntryAlreadyExists) {
			return fmt.Errorf("%w: %s is already encrypted", ErrExists, attr)
		}
		return err
	}
	return nil
}

// DeleteEncryptedAttr stops encrypting attr. Values already stored encrypted
// would read back as ciphertext after the next restart, so unless force is
// set it refuses while the suffix has values of attr (export the suffix with
// decryption, remove the setting, import the export).
func DeleteEncryptedAttr(c *ldap.Conn, backend, attr string, force bool) error {
	b, err := GetBackend(c, backend)
	if err != nil {
		return err
	}
	dn := "cn=" + ldap.EscapeDN(attr) + "," + encryptedAttrsDN(backend)
	if _, err := readEntry(c, dn, "1.1"); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			return fmt.Errorf("%w: %s is not encrypted", ErrNotFound, attr)
		}
		return err
	}
	if !force {
		res, err := c.Search(ldap.NewSearchRequest(b.Suffix, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
			"("+attr+"=*)", []string{"1.1"}, nil))
		if err != nil && !ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) && !ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			return err
		}
		if res != nil && len(res.Entries) > 0 {
			return fmt.Errorf("%w: entries under %s have %s; export the suffix with decryption, remove the setting, then import the export", ErrEncryptedValues, b.Suffix, attr)
		}
	}
	return c.Del(ldap.NewDelRequest(dn, nil))
}

// EncryptingBackends returns the backends that encrypt at least one
// attribute.
func EncryptingBackends(c *ldap.Conn) ([]string, error) {
	res, err := c.Search(ldap.NewSearchRequest(DNLDBM, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=nsAttributeEncryption)", []string{"1.1"}, nil))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range res.Entries {
		// cn=<attr>,cn=encrypted attributes,cn=<backend>,cn=ldbm database,...
		p, err := ldap.ParseDN(e.DN)
		if err != nil || len(p.RDNs) < 3 {
			continue
		}
		if be := p.RDNs[2].Attributes[0].Value; !slices.Contains(out, be) {
			out = append(out, be)
		}
	}
	return out, nil
}

// CheckServerKey refuses a new server certificate whose key differs from the
// current one while attribute encryption is configured: the encryption key
// is wrapped with the server key, and 389-ds does not start the backend when
// it cannot unwrap it. currentKey and newKey are public key fingerprints;
// an empty currentKey (no current certificate) allows any key.
func CheckServerKey(c *ldap.Conn, currentKey, newKey string) error {
	if currentKey == "" || strings.EqualFold(currentKey, newKey) {
		return nil
	}
	backends, err := EncryptingBackends(c)
	if err != nil {
		return err
	}
	if len(backends) > 0 {
		return fmt.Errorf("%w: %s encrypt attributes with a key wrapped by the current server key, and the server would not start with a certificate of another key; renew the certificate with the same key (certbot --reuse-key, cert-manager rotationPolicy: Never) or turn attribute encryption off first", ErrEncryptionKey, strings.Join(backends, ", "))
	}
	return nil
}
