package api

import (
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/certs"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/entries"
	"github.com/fomk/389-ds-ui/internal/files"
	"github.com/fomk/389-ds-ui/internal/ldapx"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
	"github.com/fomk/389-ds-ui/internal/logs"
	"github.com/fomk/389-ds-ui/internal/pages"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// Problem is an RFC 9457 problem document extended with LDAP details.
type Problem struct {
	huma.ErrorModel
	LDAPCode      int    `json:"ldapCode,omitempty" doc:"LDAP result code, if the error came from the server"`
	LDAPMatchedDN string `json:"ldapMatchedDN,omitempty"`
}

func (p *Problem) Error() string  { return p.ErrorModel.Error() }
func (p *Problem) GetStatus() int { return p.Status }

// httpStatusForLDAP maps LDAP result codes to HTTP statuses (see docs/api.md).
func httpStatusForLDAP(code uint16) int {
	switch code {
	case ldap.LDAPResultNoSuchObject:
		return http.StatusNotFound
	case ldap.LDAPResultEntryAlreadyExists, ldap.LDAPResultUnwillingToPerform,
		ldap.LDAPResultNotAllowedOnNonLeaf:
		return http.StatusConflict
	case ldap.LDAPResultConstraintViolation, ldap.LDAPResultInvalidAttributeSyntax,
		ldap.LDAPResultObjectClassViolation, ldap.LDAPResultNamingViolation,
		ldap.LDAPResultInvalidDNSyntax, ldap.LDAPResultUndefinedAttributeType,
		ldap.LDAPResultAttributeOrValueExists, ldap.LDAPResultNoSuchAttribute,
		ldap.LDAPResultObjectClassModsProhibited, ldap.LDAPResultNotAllowedOnRDN:
		return http.StatusUnprocessableEntity
	case ldap.LDAPResultInsufficientAccessRights, ldap.LDAPResultInvalidCredentials,
		ldap.LDAPResultInappropriateAuthentication:
		return http.StatusForbidden
	case ldap.LDAPResultBusy, ldap.LDAPResultUnavailable, ldap.LDAPResultServerDown,
		ldap.ErrorNetwork:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// toHTTPError converts an error from the domain layer into an API error.
func toHTTPError(err error) error {
	if err == nil {
		return nil
	}
	var se huma.StatusError
	if errors.As(err, &se) {
		return err
	}
	switch {
	case errors.Is(err, entries.ErrInvalidDN), errors.Is(err, entries.ErrInvalidCursor),
		errors.Is(err, entries.ErrInvalidChange), errors.Is(err, entries.ErrInvalidFilter),
		errors.Is(err, entries.ErrInvalidLDIF):
		return problem(http.StatusBadRequest, err.Error())
	case errors.Is(err, logs.ErrUnknownLog):
		return problem(http.StatusNotFound, err.Error())
	case errors.Is(err, files.ErrInvalidName):
		return problem(http.StatusBadRequest, err.Error())
	case errors.Is(err, files.ErrNotFound):
		return problem(http.StatusNotFound, err.Error())
	case errors.Is(err, files.ErrExists):
		return problem(http.StatusConflict, err.Error())
	case errors.Is(err, files.ErrOutsideRoot):
		return problem(http.StatusNotImplemented, err.Error())
	case errors.Is(err, schema.ErrInvalidDefinition):
		return problem(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ds.ErrSchemaInUse), errors.Is(err, ds.ErrSchemaNotUser), errors.Is(err, ds.ErrSchemaExists),
		errors.Is(err, ds.ErrExists), errors.Is(err, ds.ErrHasSubSuffixes), errors.Is(err, ds.ErrEncryptedValues),
		errors.Is(err, ds.ErrEncryptionKey):
		return problem(http.StatusConflict, err.Error())
	case errors.Is(err, certs.ErrInvalid), errors.Is(err, certs.ErrTool):
		return problem(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, certs.ErrNotFound):
		return problem(http.StatusNotFound, err.Error())
	case errors.Is(err, certs.ErrInUse):
		return problem(http.StatusConflict, err.Error())
	case errors.Is(err, ds.ErrNotReplicated):
		return problem(http.StatusConflict, err.Error())
	case errors.Is(err, ds.ErrInvalidValue):
		return problem(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ds.ErrInvalidName):
		return problem(http.StatusBadRequest, err.Error())
	case errors.Is(err, ds.ErrNotFound), errors.Is(err, ds.ErrTaskNotFound):
		return problem(http.StatusNotFound, err.Error())
	case errors.Is(err, ds.ErrSystemIndex), errors.Is(err, ds.ErrProtectedPlugin):
		return problem(http.StatusConflict, err.Error())
	case errors.Is(err, pages.ErrNotFound):
		return problem(http.StatusNotFound, err.Error())
	case errors.Is(err, pages.ErrInvalidValue):
		return problem(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, entries.ErrPagingExpired):
		return problem(http.StatusConflict, err.Error())
	case errors.Is(err, entries.ErrPreconditionFailed):
		return problem(http.StatusPreconditionFailed, err.Error())
	case errors.Is(err, entries.ErrSubtreeTooLarge), errors.Is(err, entries.ErrPasswordGenerationUnavailable):
		return problem(http.StatusUnprocessableEntity, err.Error())
	}
	if errors.Is(err, providers.ErrNotSupported) {
		return problem(http.StatusNotImplemented, err.Error())
	}
	if ldapx.IsConnError(err) {
		return problem(http.StatusServiceUnavailable, "directory server is unreachable: "+err.Error())
	}
	var le *ldap.Error
	if errors.As(err, &le) {
		p := problem(httpStatusForLDAP(le.ResultCode), err.Error())
		p.LDAPCode = int(le.ResultCode)
		p.LDAPMatchedDN = le.MatchedDN
		return p
	}
	return problem(http.StatusInternalServerError, err.Error())
}

func problem(status int, detail string) *Problem {
	return &Problem{ErrorModel: huma.ErrorModel{
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	}}
}

// errorsAs is errors.As, for handlers outside huma.
func errorsAs(err error, target any) bool { return errors.As(err, target) }
