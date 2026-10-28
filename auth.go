package mygo

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// AuthRequest is passed to the auth handler of a window (SetAuthHandler):
// a server, or a proxy, asks for a user name and a password before it
// serves the page, or something the page loads.
type AuthRequest struct {
	// Host and Port of the server asking, or of the proxy, e.g.
	// "intranet.example.com" and 443.
	Host string
	Port int
	// Realm is the protection space the server named, "" for none: one
	// server can ask for the credentials of several.
	Realm string
	// Scheme is the authentication scheme, lowercase: "basic", "digest",
	// "ntlm" or "negotiate".
	Scheme string
	// IsProxy is true when a proxy asks rather than the server. WebView2
	// does not tell: it is false on Windows, where Host is then the
	// proxy's.
	IsProxy bool
}

// AuthResponse answers an AuthRequest with the user name and the password
// to sign in with. One with neither cancels the sign-in, as a user
// dismissing a browser's prompt does: the page gets the server's refusal,
// its 401 (or 407) page.
type AuthResponse struct {
	Username string
	Password string
}

// SetAuthHandler answers the HTTP authentication challenges (Basic,
// Digest, NTLM, Negotiate) of the window's pages and of what they load,
// the server's or a proxy's: fn returns the credentials to sign in with,
// or an empty AuthResponse to cancel, and runs on the main thread while
// the load waits. A server that refuses the credentials asks again; the
// web view keeps those it accepts for the rest of the session, for that
// server and realm, as browsers do, so fn is not asked for every request.
// MyGo never logs them.
//
//	page.SetAuthHandler(func(req mygo.AuthRequest) mygo.AuthResponse {
//		if req.Host == "router.lan" && !req.IsProxy {
//			return mygo.AuthResponse{Username: "admin", Password: password}
//		}
//		return mygo.AuthResponse{}
//	})
//
// To ask the user, fn can cancel and show the app's own prompt, then
// Reload the page once it has the credentials to answer with.
//
// nil restores the default, the platform's:
//
//   - macOS: no prompt; the page gets the server's refusal. MyGo answers
//     WKNavigationDelegate's
//     webView:didReceiveAuthenticationChallenge:completionHandler:.
//   - Linux: WebKitGTK's sign-in dialog. MyGo answers
//     WebKitWebView::authenticate.
//   - Windows: WebView2's sign-in dialog. MyGo answers ICoreWebView2_10's
//     BasicAuthenticationRequested; on a runtime without it, fn is never
//     asked.
func (p *Page) SetAuthHandler(fn func(req AuthRequest) AuthResponse) {
	p.w.mu.Lock()
	p.w.authHandler = fn
	p.w.mu.Unlock()
}

func (h *windowHandler) AuthRequested(req platform.AuthRequest) platform.AuthResponse {
	h.w.mu.Lock()
	fn := h.w.authHandler
	h.w.mu.Unlock()
	if fn == nil {
		return platform.AuthResponse{Default: true}
	}
	r := fn(AuthRequest{Host: req.Host, Port: req.Port, Realm: req.Realm, Scheme: req.Scheme, IsProxy: req.IsProxy})
	return platform.AuthResponse{Username: r.Username, Password: r.Password}
}

// CertificateError is passed to the certificate error handler of a
// window (SetCertificateErrorHandler): a server's certificate that does
// not verify.
type CertificateError struct {
	// URL is what was loading: the page, or something it loads. On macOS
	// it is the server's origin, such as "https://dev.example:8443", as
	// WebKit asks about the connection rather than a request.
	URL string
	// Host is the server's host in URL, a name or an IP address.
	Host string
	// Error tells what is wrong with the certificate, in the platform's
	// words, for logs and messages.
	Error string
	// Certificate is the server's certificate, DER-encoded, which
	// x509.ParseCertificate reads. Compare it, or its hash, with the one
	// the app expects.
	Certificate []byte
}

// SetCertificateErrorHandler decides about the server certificates that
// do not verify, for the window's pages and what they load: fn returns
// true to load them anyway, as for a dev server with a self-signed
// certificate or an internal host whose certificate the app ships. It
// runs on the main thread while the load waits. By default, and whenever
// fn returns false, the load fails as in a browser (OnDidFailLoad). nil
// restores the default.
//
// Allow only the certificate expected, never a host whatever its
// certificate, which anyone between the app and the server can present:
//
//	page.SetCertificateErrorHandler(func(e mygo.CertificateError) bool {
//		sum := sha256.Sum256(e.Certificate)
//		return e.Host == "dev.example" && sum == devCertificateSHA256
//	})
//
// What is asked, and how long a yes lasts, depends on the platform:
//
//   - macOS: each connection to the server, of the page and of what it
//     loads; a yes lasts for that connection. MyGo checks the
//     certificate itself (SecTrustEvaluateAsyncWithError), only while a
//     handler is set, from WKNavigationDelegate's
//     webView:didReceiveAuthenticationChallenge:completionHandler:.
//   - Linux: the page only, as WebKitGTK tells no more
//     (WebKitWebView::load-failed-with-tls-errors): what it loads from
//     such a server fails without asking. A yes allows that certificate
//     for its host in every window until the app quits
//     (webkit_web_context_allow_tls_certificate_for_host), and the page
//     loads again.
//   - Windows: the page and what it loads (ICoreWebView2_14's
//     ServerCertificateErrorDetected; on a runtime without it, fn is
//     never asked).
//     WebView2 reports the page's load failed (OnDidFailLoad) before it
//     asks, and a yes allows that certificate for its host in every
//     window until the app quits.
func (p *Page) SetCertificateErrorHandler(fn func(e CertificateError) bool) {
	p.w.mu.Lock()
	p.w.certificateHandler = fn
	p.w.mu.Unlock()
}

func (h *windowHandler) CertificateError(e platform.CertificateError) bool {
	h.w.mu.Lock()
	fn := h.w.certificateHandler
	h.w.mu.Unlock()
	return fn != nil && fn(CertificateError{URL: e.URL, Host: e.Host, Error: e.Error, Certificate: e.Certificate})
}

func (h *windowHandler) HandlesCertificateErrors() bool {
	h.w.mu.Lock()
	defer h.w.mu.Unlock()
	return h.w.certificateHandler != nil
}

// ClientCertificateRequest is passed to the client certificate handler
// of a window (SetClientCertificateHandler): a server asks the page, or
// what it loads, for a TLS client certificate, to tell who connects.
type ClientCertificateRequest struct {
	// Host and Port of the server asking, e.g. "intranet.example.com"
	// and 443.
	Host string
	Port int
	// Issuers are the distinguished names of the certificate
	// authorities whose certificates the server accepts, as
	// pkix.RDNSequence's String writes them ("CN=Example CA,O=Example").
	// It is nil when the server names none, and on Linux, where
	// WebKitGTK does not tell.
	Issuers []string
	// Certificates are those the system offers for the server, each
	// with its private key, in the system's order: on macOS the
	// keychains' identities, on Windows the user's certificate store,
	// those whose chain has one of Issuers when the server names any.
	// It is empty on Linux, where WebKitGTK knows no certificates.
	Certificates []ClientCertificate
}

// ClientCertificate is a certificate the system can present to a
// server, with its private key, which stays in the system.
type ClientCertificate struct {
	// Subject and Issuer are the distinguished names of the certificate
	// and of its issuer, as pkix.RDNSequence's String writes them:
	// "CN=alice,O=Example".
	Subject, Issuer string
	// NotBefore and NotAfter bound when the certificate is valid.
	NotBefore, NotAfter time.Time
	// Certificate is the certificate, DER-encoded, which
	// x509.ParseCertificate reads for anything else it says.
	Certificate []byte
}

// ClientCertificateResponse answers a ClientCertificateRequest. The zero
// value presents no certificate, as a user cancelling a browser's
// picker does: the connection goes on without one, and the server
// decides, refusing it or serving what it serves to anyone.
type ClientCertificateResponse struct {
	// Default leaves the choice to the platform, as without a handler.
	Default bool
	// Certificate is the one to present, one of the request's
	// Certificates: &req.Certificates[i]. Any other presents none.
	Certificate *ClientCertificate
}

// SetClientCertificateHandler answers the servers that ask the window's
// pages, or what they load, for a TLS client certificate (mutual TLS):
// fn picks one of the certificates the system offers, presents none, or
// leaves the choice to the platform. It runs on the main thread while
// the connection waits. The web view may keep the answer for that
// server for the rest of the session, as browsers do, so fn is not
// asked for every connection.
//
//	page.SetClientCertificateHandler(func(req mygo.ClientCertificateRequest) mygo.ClientCertificateResponse {
//		if req.Host != "intranet.example.com" {
//			return mygo.ClientCertificateResponse{Default: true}
//		}
//		for i, c := range req.Certificates {
//			if c.Issuer == "CN=Example Corp CA,O=Example Corp" && time.Now().Before(c.NotAfter) {
//				return mygo.ClientCertificateResponse{Certificate: &req.Certificates[i]}
//			}
//		}
//		return mygo.ClientCertificateResponse{} // none
//	})
//
// The system may ask the user before the private key of the certificate
// is used (macOS can, for a key in the keychain; a smart card asks for
// its PIN). Certificates come from the system only: one the app holds,
// such as a PKCS#12 file, cannot be presented, as WebView2 presents
// only those of the certificate store, and macOS would have to import
// it into the keychain.
//
// nil restores the default, the platform's:
//
//   - macOS: no picker; WebKit goes on without a certificate. MyGo
//     answers the NSURLAuthenticationMethodClientCertificate challenges
//     of WKNavigationDelegate's
//     webView:didReceiveAuthenticationChallenge:completionHandler:, with
//     the identities SecItemCopyMatching finds (kSecMatchIssuers).
//   - Linux: WebKitGTK goes on without a certificate. MyGo answers
//     WebKitWebView::authenticate's
//     WEBKIT_AUTHENTICATION_SCHEME_CLIENT_CERTIFICATE_REQUESTED, which
//     WebKitGTK 2.34 and later ask with libsoup 3 (WebKitGTK 4.1; with
//     4.0, fn is never asked). WebKitGTK lists no certificates, so fn
//     can only present none or leave the default.
//   - Windows: WebView2's own handling, which went on without a
//     certificate where it was tried, though Microsoft documents a
//     picker it may show. MyGo answers ICoreWebView2_5's ClientCertificateRequested,
//     with its MutuallyTrustedCertificates; on a runtime without it, fn
//     is never asked.
func (p *Page) SetClientCertificateHandler(fn func(req ClientCertificateRequest) ClientCertificateResponse) {
	// The window keeps fn with what converts the backend's request and
	// answer, which only apps that set a handler link (crypto/x509's
	// parsing).
	var answer func(platform.ClientCertificateRequest) platform.ClientCertificateResponse
	if fn != nil {
		answer = func(req platform.ClientCertificateRequest) platform.ClientCertificateResponse {
			return clientCertificateAnswer(req, fn(clientCertificateRequest(req)))
		}
	}
	p.w.mu.Lock()
	p.w.clientCertificateHandler = answer
	p.w.mu.Unlock()
}

func (h *windowHandler) ClientCertificateRequested(req platform.ClientCertificateRequest) platform.ClientCertificateResponse {
	h.w.mu.Lock()
	answer := h.w.clientCertificateHandler
	h.w.mu.Unlock()
	if answer == nil {
		return platform.ClientCertificateResponse{Default: true}
	}
	return answer(req)
}

func (h *windowHandler) HandlesClientCertificates() bool {
	h.w.mu.Lock()
	defer h.w.mu.Unlock()
	return h.w.clientCertificateHandler != nil
}

// clientCertificateRequest describes what a backend asks the app.
func clientCertificateRequest(req platform.ClientCertificateRequest) ClientCertificateRequest {
	r := ClientCertificateRequest{Host: req.Host, Port: req.Port}
	for _, name := range req.Issuers {
		r.Issuers = append(r.Issuers, distinguishedName(name))
	}
	r.Certificates = make([]ClientCertificate, len(req.Certificates))
	for i, der := range req.Certificates {
		c := &r.Certificates[i]
		c.Certificate = der
		// One that does not parse keeps its place, the index the backend
		// presents it by, and only its DER.
		if cert, err := x509.ParseCertificate(der); err == nil {
			c.Subject = distinguishedName(cert.RawSubject)
			c.Issuer = distinguishedName(cert.RawIssuer)
			c.NotBefore, c.NotAfter = cert.NotBefore, cert.NotAfter
		}
	}
	return r
}

// clientCertificateAnswer finds the certificate resp presents among
// req's, by its DER encoding: the same certificate is the same key.
func clientCertificateAnswer(req platform.ClientCertificateRequest, resp ClientCertificateResponse) platform.ClientCertificateResponse {
	if resp.Default {
		return platform.ClientCertificateResponse{Default: true}
	}
	if resp.Certificate != nil && len(resp.Certificate.Certificate) > 0 {
		for i, der := range req.Certificates {
			if bytes.Equal(der, resp.Certificate.Certificate) {
				return platform.ClientCertificateResponse{Index: i}
			}
		}
	}
	return platform.ClientCertificateResponse{Index: -1}
}

// distinguishedName writes a DER-encoded X.501 name as RFC 2253 does,
// "" for one that does not parse.
func distinguishedName(der []byte) string {
	var rdn pkix.RDNSequence
	if rest, err := asn1.Unmarshal(der, &rdn); err != nil || len(rest) > 0 {
		return ""
	}
	return rdn.String()
}
