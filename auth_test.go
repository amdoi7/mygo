//go:build !js

package mygo

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestAuthHandler(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	ask := func(req platform.AuthRequest) platform.AuthResponse {
		return onMainValue(func() platform.AuthResponse { return fw.H.AuthRequested(req) })
	}
	challenge := platform.AuthRequest{Host: "router.lan", Port: 80, Realm: "admin", Scheme: "basic"}
	// Without a handler the web view decides, as before.
	if got := ask(challenge); got != (platform.AuthResponse{Default: true}) {
		t.Errorf("default answer = %+v", got)
	}
	var got AuthRequest
	w.Page().SetAuthHandler(func(req AuthRequest) AuthResponse {
		got = req
		if req.Host == "router.lan" && !req.IsProxy {
			return AuthResponse{Username: "admin", Password: "hunter2"}
		}
		return AuthResponse{}
	})
	if r := ask(challenge); r != (platform.AuthResponse{Username: "admin", Password: "hunter2"}) {
		t.Errorf("answer = %+v", r)
	}
	if got != (AuthRequest{Host: "router.lan", Port: 80, Realm: "admin", Scheme: "basic"}) {
		t.Errorf("request = %+v", got)
	}
	// An empty response cancels: no credentials, and not the default.
	proxy := platform.AuthRequest{Host: "proxy.lan", Port: 3128, Scheme: "digest", IsProxy: true}
	if r := ask(proxy); r != (platform.AuthResponse{}) {
		t.Errorf("canceled answer = %+v", r)
	}
	if !got.IsProxy || got.Scheme != "digest" || got.Port != 3128 {
		t.Errorf("proxy request = %+v", got)
	}
	w.Page().SetAuthHandler(nil)
	if r := ask(challenge); !r.Default {
		t.Error("SetAuthHandler(nil) did not restore the default")
	}
}

func TestCertificateErrorHandler(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	handles := func() bool { return onMainValue(fw.H.HandlesCertificateErrors) }
	ask := func(e platform.CertificateError) bool {
		return onMainValue(func() bool { return fw.H.CertificateError(e) })
	}
	der := []byte{0x30, 0x82, 0x01, 0x0a}
	e := platform.CertificateError{URL: "https://dev.example:8443/app", Host: "dev.example", Error: "the certificate's issuer is not known", Certificate: der}
	// By default certificate errors fail, and backends need not check.
	if handles() || ask(e) {
		t.Error("a certificate error is allowed without a handler")
	}
	var got CertificateError
	w.Page().SetCertificateErrorHandler(func(e CertificateError) bool {
		got = e
		return e.Host == "dev.example" && bytes.Equal(e.Certificate, der)
	})
	if !handles() || !ask(e) {
		t.Error("the handler's yes was not applied")
	}
	if got.URL != e.URL || got.Host != e.Host || got.Error != e.Error || !bytes.Equal(got.Certificate, der) {
		t.Errorf("error = %+v", got)
	}
	other := e
	other.Certificate = []byte{0x30, 0x00}
	if ask(other) {
		t.Error("the handler's no was not applied")
	}
	w.Page().SetCertificateErrorHandler(nil)
	if handles() || ask(e) {
		t.Error("SetCertificateErrorHandler(nil) did not restore the default")
	}
}

// The page of a web view in native UI has handlers of its own.
func TestWebViewAuthHandlers(t *testing.T) {
	show := true
	_, v, _, fv := webViewWindow(t, &show)
	v.Page().SetAuthHandler(func(AuthRequest) AuthResponse { return AuthResponse{Username: "u", Password: "p"} })
	v.Page().SetCertificateErrorHandler(func(CertificateError) bool { return true })
	if r := onMainValue(func() platform.AuthResponse { return fv.H.AuthRequested(platform.AuthRequest{Host: "h"}) }); r.Username != "u" {
		t.Errorf("web view answer = %+v", r)
	}
	if !onMainValue(func() bool { return fv.H.CertificateError(platform.CertificateError{Host: "h"}) }) {
		t.Error("the web view's certificate error handler was not asked")
	}
	v.Page().SetClientCertificateHandler(func(ClientCertificateRequest) ClientCertificateResponse { return ClientCertificateResponse{} })
	if !onMainValue(fv.H.HandlesClientCertificates) {
		t.Error("the web view's client certificate handler is not set")
	}
	if r := onMainValue(func() platform.ClientCertificateResponse {
		return fv.H.ClientCertificateRequested(platform.ClientCertificateRequest{Host: "h"})
	}); r != (platform.ClientCertificateResponse{Index: -1}) {
		t.Errorf("web view client certificate answer = %+v", r)
	}
}

// testCertificate returns a DER certificate of subject, issued by issuer
// (self-signed when it is nil), and its key.
func testCertificate(t *testing.T, subject pkix.Name, issuer *x509.Certificate, issuerKey *ecdsa.PrivateKey, notBefore, notAfter time.Time) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  issuer == nil,
	}
	if issuer == nil {
		issuer, issuerKey = tmpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, issuer, &key.PublicKey, issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func TestClientCertificateHandler(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	handles := func() bool { return onMainValue(fw.H.HandlesClientCertificates) }
	ask := func(req platform.ClientCertificateRequest) platform.ClientCertificateResponse {
		return onMainValue(func() platform.ClientCertificateResponse { return fw.H.ClientCertificateRequested(req) })
	}
	from := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	until := from.AddDate(1, 0, 0)
	ca, caKey := testCertificate(t, pkix.Name{CommonName: "Example CA", Organization: []string{"Example"}}, nil, nil, from, until)
	alice, _ := testCertificate(t, pkix.Name{CommonName: "alice", Organization: []string{"Example"}}, ca, caKey, from, until)
	bob, _ := testCertificate(t, pkix.Name{CommonName: "bob, jr.", OrganizationalUnit: []string{"Ops"}}, ca, caKey, from, until.AddDate(1, 0, 0))
	other, _ := testCertificate(t, pkix.Name{CommonName: "mallory"}, nil, nil, from, until)
	garbage := []byte{0x30, 0x03, 0x02, 0x01, 0x01}
	req := platform.ClientCertificateRequest{
		Host:         "intranet.example",
		Port:         8443,
		Issuers:      [][]byte{ca.RawSubject, {0xff}},
		Certificates: [][]byte{alice.Raw, garbage, bob.Raw},
	}

	// Without a handler the web view decides, and backends need not list
	// certificates.
	if handles() {
		t.Error("HandlesClientCertificates without a handler")
	}
	if got := ask(req); got != (platform.ClientCertificateResponse{Default: true}) {
		t.Errorf("default answer = %+v", got)
	}

	var got ClientCertificateRequest
	answer := func(r ClientCertificateRequest) ClientCertificateResponse { return ClientCertificateResponse{} }
	w.Page().SetClientCertificateHandler(func(r ClientCertificateRequest) ClientCertificateResponse {
		got = r
		return answer(r)
	})
	if !handles() {
		t.Error("no HandlesClientCertificates with a handler")
	}
	// The zero answer presents none.
	if r := ask(req); r != (platform.ClientCertificateResponse{Index: -1}) {
		t.Errorf("answer = %+v, want none", r)
	}
	if got.Host != "intranet.example" || got.Port != 8443 {
		t.Errorf("request = %+v", got)
	}
	// A name that does not parse keeps its place, empty.
	if want := []string{"CN=Example CA,O=Example", ""}; len(got.Issuers) != 2 || got.Issuers[0] != want[0] || got.Issuers[1] != want[1] {
		t.Errorf("issuers = %q, want %q", got.Issuers, want)
	}
	if len(got.Certificates) != 3 {
		t.Fatalf("certificates = %+v", got.Certificates)
	}
	want := ClientCertificate{Subject: "CN=alice,O=Example", Issuer: "CN=Example CA,O=Example", NotBefore: from, NotAfter: until, Certificate: alice.Raw}
	if c := got.Certificates[0]; c.Subject != want.Subject || c.Issuer != want.Issuer || !c.NotBefore.Equal(from) || !c.NotAfter.Equal(until) || !bytes.Equal(c.Certificate, alice.Raw) {
		t.Errorf("certificate 0 = %+v, want %+v", c, want)
	}
	if c := got.Certificates[2]; c.Subject != `CN=bob\, jr.,OU=Ops` || !c.NotAfter.Equal(until.AddDate(1, 0, 0)) {
		t.Errorf("certificate 2 = %+v", c)
	}
	// One that does not parse keeps its place, with only its DER.
	if c := got.Certificates[1]; c.Subject != "" || c.Issuer != "" || !c.NotAfter.IsZero() || !bytes.Equal(c.Certificate, garbage) {
		t.Errorf("certificate 1 = %+v", c)
	}

	for _, c := range []struct {
		name   string
		answer func(r ClientCertificateRequest) ClientCertificateResponse
		want   platform.ClientCertificateResponse
	}{
		{"by index", func(r ClientCertificateRequest) ClientCertificateResponse {
			return ClientCertificateResponse{Certificate: &r.Certificates[2]}
		}, platform.ClientCertificateResponse{Index: 2}},
		{"a copy", func(r ClientCertificateRequest) ClientCertificateResponse {
			c := r.Certificates[0]
			return ClientCertificateResponse{Certificate: &c}
		}, platform.ClientCertificateResponse{Index: 0}},
		{"one the system did not offer", func(r ClientCertificateRequest) ClientCertificateResponse {
			return ClientCertificateResponse{Certificate: &ClientCertificate{Certificate: other.Raw}}
		}, platform.ClientCertificateResponse{Index: -1}},
		{"one without DER", func(r ClientCertificateRequest) ClientCertificateResponse {
			return ClientCertificateResponse{Certificate: &ClientCertificate{Subject: "CN=alice,O=Example"}}
		}, platform.ClientCertificateResponse{Index: -1}},
		{"the default", func(r ClientCertificateRequest) ClientCertificateResponse {
			return ClientCertificateResponse{Default: true, Certificate: &r.Certificates[0]}
		}, platform.ClientCertificateResponse{Default: true}},
	} {
		answer = c.answer
		if r := ask(req); r != c.want {
			t.Errorf("%s: answer = %+v, want %+v", c.name, r, c.want)
		}
	}

	// A request without issuers or certificates, as on Linux.
	answer = func(r ClientCertificateRequest) ClientCertificateResponse { return ClientCertificateResponse{} }
	if r := ask(platform.ClientCertificateRequest{Host: "h", Port: 443}); r.Index != -1 || r.Default {
		t.Errorf("answer without certificates = %+v", r)
	}
	if got.Issuers != nil || len(got.Certificates) != 0 {
		t.Errorf("request without certificates = %+v", got)
	}

	w.Page().SetClientCertificateHandler(nil)
	if handles() || !ask(req).Default {
		t.Error("SetClientCertificateHandler(nil) did not restore the default")
	}
}
