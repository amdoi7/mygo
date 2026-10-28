//go:build windows && (amd64 || arm64)

package windows

import (
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// HTTP authentication, server certificates and client certificates of
// pages: ICoreWebView2_10's BasicAuthenticationRequested,
// ICoreWebView2_14's ServerCertificateErrorDetected and ICoreWebView2_5's
// ClientCertificateRequested. Vtable indices from WebView2.h, as in
// webview2.go: ICoreWebView2_4 ends with remove_DownloadStarting (76),
// ICoreWebView2_9 with put_DefaultDownloadDialogMargin (96),
// ICoreWebView2_13 with get_Profile (105).
const (
	wv5AddClientCertificateRequested      = 77
	wv10AddBasicAuthenticationRequested   = 97
	wv14AddServerCertificateErrorDetected = 106

	// ICoreWebView2BasicAuthenticationRequestedEventArgs and its response.
	authGetURI          = 3
	authGetChallenge    = 4
	authGetResponse     = 5
	authPutCancel       = 7
	authRespPutUserName = 4
	authRespPutPassword = 6

	// ICoreWebView2ServerCertificateErrorDetectedEventArgs, and
	// ICoreWebView2Certificate's ToPemEncoding.
	certErrGetErrorStatus       = 3
	certErrGetRequestURI        = 4
	certErrGetServerCertificate = 5
	certErrPutAction            = 7
	certToPemEncoding           = 9

	// ICoreWebView2ClientCertificateRequestedEventArgs, and the
	// collections it holds: ICoreWebView2StringCollection and
	// ICoreWebView2ClientCertificateCollection.
	clientCertGetHost                = 3
	clientCertGetPort                = 4
	clientCertGetAllowedAuthorities  = 6
	clientCertGetMutuallyTrusted     = 7
	clientCertPutSelectedCertificate = 9
	clientCertPutHandled             = 13
	collectionGetCount               = 3
	collectionGetValueAtIndex        = 4
)

var (
	iidICoreWebView2_5  = guid("bedb11b8-d63c-11eb-b8bc-0242ac130003")
	iidICoreWebView2_10 = guid("b1690564-6f5a-4983-8e48-31d1143fecdb")
	iidICoreWebView2_14 = guid("6daa4f10-4a90-4753-8898-77c5df534165")
)

// certError words COREWEBVIEW2_WEB_ERROR_STATUS's certificate errors.
func certError(status int32) string {
	switch status {
	case 1:
		return "the certificate does not match the host"
	case 2:
		return "the certificate expired"
	case 3:
		return "the client certificate contains errors"
	case 4:
		return "the certificate was revoked"
	case 5:
		return "the certificate is invalid"
	}
	return "the certificate does not verify"
}

// pemCertificate decodes the certificate that ToPemEncoding wrote, as
// encoding/pem would, which apps that never ask need not link.
func pemCertificate(s string) []byte {
	_, s, ok := strings.Cut(s, "-----BEGIN CERTIFICATE-----")
	if s, _, ok2 := strings.Cut(s, "-----END"); ok && ok2 {
		s = strings.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, s)
		b := make([]byte, base64.StdEncoding.DecodedLen(len(s)))
		if n, err := base64.StdEncoding.Decode(b, []byte(s)); err == nil {
			return b[:n]
		}
	}
	return nil
}

// addAuthHandlers handles the web view's sign-ins and certificate errors,
// on runtimes that have them.
func (w *window) addAuthHandlers() {
	var tok int64
	if wv5 := queryInterface(w.webview, &iidICoreWebView2_5); wv5 != 0 {
		withHandler(w.clientCertificateRequested, func(h uintptr) uintptr {
			return comCall(wv5, wv5AddClientCertificateRequested, h, uintptr(unsafe.Pointer(&tok)))
		})
		release(wv5)
	}
	if wv10 := queryInterface(w.webview, &iidICoreWebView2_10); wv10 != 0 {
		withHandler(w.basicAuthenticationRequested, func(h uintptr) uintptr {
			return comCall(wv10, wv10AddBasicAuthenticationRequested, h, uintptr(unsafe.Pointer(&tok)))
		})
		release(wv10)
	}
	if wv14 := queryInterface(w.webview, &iidICoreWebView2_14); wv14 != 0 {
		withHandler(w.serverCertificateErrorDetected, func(h uintptr) uintptr {
			return comCall(wv14, wv14AddServerCertificateErrorDetected, h, uintptr(unsafe.Pointer(&tok)))
		})
		release(wv14)
	}
}

// basicAuthenticationRequested answers a sign-in. Doing nothing leaves it
// to WebView2: its sign-in dialog.
func (w *window) basicAuthenticationRequested(_, args uintptr) {
	var p uintptr
	comCall(args, authGetURI, uintptr(unsafe.Pointer(&p)))
	u, err := url.Parse(takeWstr(p))
	if err != nil {
		return
	}
	comCall(args, authGetChallenge, uintptr(unsafe.Pointer(&p)))
	req := platform.AuthRequest{Host: u.Hostname()}
	req.Scheme, req.Realm = platform.ParseAuthChallenge(takeWstr(p))
	if req.Port, err = strconv.Atoi(u.Port()); err != nil {
		req.Port = 80
		if u.Scheme == "https" {
			req.Port = 443
		}
	}
	resp := w.h.AuthRequested(req)
	switch {
	case resp.Default:
	case resp.Username == "" && resp.Password == "":
		comCall(args, authPutCancel, 1) // the server's refusal
	default:
		var r uintptr
		if failed(comCall(args, authGetResponse, uintptr(unsafe.Pointer(&r)))) || r == 0 {
			return
		}
		comCall(r, authRespPutUserName, uintptr(unsafe.Pointer(u16(resp.Username))))
		comCall(r, authRespPutPassword, uintptr(unsafe.Pointer(u16(resp.Password))))
		release(r)
	}
}

// serverCertificateErrorDetected asks the app about a certificate that
// does not verify. Leaving the action as it is (DEFAULT) fails the
// request, with WebView2's error page for a page.
func (w *window) serverCertificateErrorDetected(_, args uintptr) {
	if !w.h.HandlesCertificateErrors() {
		return
	}
	var status int32
	comCall(args, certErrGetErrorStatus, uintptr(unsafe.Pointer(&status)))
	var p uintptr
	comCall(args, certErrGetRequestURI, uintptr(unsafe.Pointer(&p)))
	raw := takeWstr(p)
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	e := platform.CertificateError{URL: raw, Host: u.Hostname(), Error: certError(status)}
	var cert uintptr
	if !failed(comCall(args, certErrGetServerCertificate, uintptr(unsafe.Pointer(&cert)))) && cert != 0 {
		if !failed(comCall(cert, certToPemEncoding, uintptr(unsafe.Pointer(&p)))) {
			e.Certificate = pemCertificate(takeWstr(p))
		}
		release(cert)
	}
	if w.h.CertificateError(e) {
		const alwaysAllow = 0 // COREWEBVIEW2_SERVER_CERTIFICATE_ERROR_ACTION_ALWAYS_ALLOW
		comCall(args, certErrPutAction, alwaysAllow)
	}
}

// clientCertificateRequested asks the app which of the certificates
// WebView2 found in the user's store to present to a server asking for
// one. Leaving Handled false leaves it to WebView2's own handling.
func (w *window) clientCertificateRequested(_, args uintptr) {
	if !w.h.HandlesClientCertificates() {
		return
	}
	var p uintptr
	comCall(args, clientCertGetHost, uintptr(unsafe.Pointer(&p)))
	req := platform.ClientCertificateRequest{Host: takeWstr(p)}
	var port int32
	comCall(args, clientCertGetPort, uintptr(unsafe.Pointer(&port)))
	req.Port = int(port)
	// The issuers the server accepts: Base64 of DER-encoded names.
	var names uintptr
	if !failed(comCall(args, clientCertGetAllowedAuthorities, uintptr(unsafe.Pointer(&names)))) && names != 0 {
		for i := range collectionCount(names) {
			if failed(comCall(names, collectionGetValueAtIndex, uintptr(i), uintptr(unsafe.Pointer(&p)))) {
				continue
			}
			if der := base64DER(takeWstr(p)); der != nil {
				req.Issuers = append(req.Issuers, der)
			}
		}
		release(names)
	}
	// The store's certificates WebView2 offers for the server, kept
	// until the answer, by the index the app answers with.
	var certs []uintptr
	defer func() {
		for _, c := range certs {
			release(c)
		}
	}()
	var list uintptr
	if !failed(comCall(args, clientCertGetMutuallyTrusted, uintptr(unsafe.Pointer(&list)))) && list != 0 {
		for i := range collectionCount(list) {
			var c uintptr
			if failed(comCall(list, collectionGetValueAtIndex, uintptr(i), uintptr(unsafe.Pointer(&c)))) || c == 0 {
				continue
			}
			var der []byte
			if !failed(comCall(c, certToPemEncoding, uintptr(unsafe.Pointer(&p)))) {
				der = pemCertificate(takeWstr(p))
			}
			if der == nil {
				release(c)
				continue
			}
			certs = append(certs, c)
			req.Certificates = append(req.Certificates, der)
		}
		release(list)
	}
	resp := w.h.ClientCertificateRequested(req)
	if resp.Default {
		return
	}
	if resp.Index >= 0 && resp.Index < len(certs) {
		comCall(args, clientCertPutSelectedCertificate, certs[resp.Index])
	}
	// Handled without a certificate goes on without one.
	comCall(args, clientCertPutHandled, 1)
}

// collectionCount returns the Count of a WebView2 collection.
func collectionCount(c uintptr) uint32 {
	var n uint32
	if failed(comCall(c, collectionGetCount, uintptr(unsafe.Pointer(&n)))) {
		return 0
	}
	return n
}

// base64DER decodes the Base64 of a DER encoding, padded or not, nil
// for anything else.
func base64DER(s string) []byte {
	s = strings.TrimSpace(s)
	der, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		der, err = base64.RawStdEncoding.DecodeString(s)
	}
	if err != nil || len(der) == 0 {
		return nil
	}
	return der
}
