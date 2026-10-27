//go:build darwin

package darwin

import (
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/egoist/mygo/internal/platform"
)

// HTTP authentication, server certificates and client certificates of
// pages: the navigation delegate's
// webView:didReceiveAuthenticationChallenge:completionHandler:, whose
// completion handler takes an NSURLSessionAuthChallengeDisposition and a
// credential.

const (
	authUseCredential = 0
	authDefault       = 1
	authReject        = 3 // NSURLSessionAuthChallengeRejectProtectionSpace
)

// authScheme names the NSURLAuthenticationMethods of HTTP, "" the
// others, which get WebKit's default handling.
func authScheme(method string) string {
	switch method {
	case "NSURLAuthenticationMethodDefault", "NSURLAuthenticationMethodHTTPBasic":
		return "basic"
	case "NSURLAuthenticationMethodHTTPDigest":
		return "digest"
	case "NSURLAuthenticationMethodNTLM":
		return "ntlm"
	case "NSURLAuthenticationMethodNegotiate":
		return "negotiate"
	}
	return ""
}

// trustAPI is what checking server certificates takes of the Security
// framework, loaded with the first check.
var trustAPI = sync.OnceValue(func() *trustFuncs {
	lib, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		return nil
	}
	main, err := purego.Dlsym(purego.RTLD_DEFAULT, "_dispatch_main_q")
	if err != nil {
		return nil
	}
	return &trustFuncs{
		evaluate:  mustDlsym(lib, "SecTrustEvaluateAsyncWithError"),
		chain:     mustDlsym(lib, "SecTrustCopyCertificateChain"),
		data:      mustDlsym(lib, "SecCertificateCopyData"),
		mainQueue: main, // dispatch_get_main_queue()
	}
})

type trustFuncs struct{ evaluate, chain, data, mainQueue uintptr }

// authChallenge answers a challenge of the page or of what it loads.
func (w *window) authChallenge(challenge id, handler uintptr) {
	space := send(challenge, "protectionSpace")
	method := goString(send(space, "authenticationMethod"))
	switch method {
	case "NSURLAuthenticationMethodServerTrust":
		w.serverTrust(challenge, space, handler)
		return
	case "NSURLAuthenticationMethodClientCertificate":
		w.clientCertificate(space, handler)
		return
	}
	scheme := authScheme(method)
	if scheme == "" {
		callBlock(handler, authDefault, 0)
		return
	}
	resp := w.h.AuthRequested(platform.AuthRequest{
		Host:    goString(send(space, "host")),
		Port:    sendInt(space, "port"),
		Realm:   goString(send(space, "realm")),
		Scheme:  scheme,
		IsProxy: sendBool(space, "isProxy"),
	})
	switch {
	case resp.Default:
		callBlock(handler, authDefault, 0)
	case resp.Username == "" && resp.Password == "":
		// Without credentials: the server's refusal.
		callBlock(handler, authReject, 0)
	default:
		withPool(func() {
			const forSession = 1 // NSURLCredentialPersistenceForSession
			cred := send(class("NSURLCredential"), "credentialWithUser:password:persistence:",
				uintptr(nsString(resp.Username)), uintptr(nsString(resp.Password)), forSession)
			callBlock(handler, authUseCredential, uintptr(cred))
		})
	}
}

// serverTrust checks the server's certificate while the app handles
// certificate errors, and asks it about one that does not verify: WebKit
// checks it again otherwise (default handling), and fails the load.
func (w *window) serverTrust(challenge, space id, handler uintptr) {
	api := trustAPI()
	trust := send(space, "serverTrust")
	if api == nil || trust == 0 || !w.h.HandlesCertificateErrors() {
		callBlock(handler, authDefault, 0)
		return
	}
	host, port := goString(send(space, "host")), sendInt(space, "port")
	handler = blockCopy(handler)
	retain(challenge) // and its trust, until the check ends
	// SecTrustEvaluateAsyncWithError calls back on the queue it was
	// called from, the main one.
	block := newBlock(func(_ objc.Block, _ id, ok bool, nserr id) {
		defer release(challenge)
		defer blockRelease(handler)
		if ok || w.closed {
			callBlock(handler, authDefault, 0)
			return
		}
		origin := host
		if strings.Contains(host, ":") {
			origin = "[" + host + "]" // IPv6
		}
		if port != 443 && port != 0 {
			origin += ":" + strconv.Itoa(port)
		}
		e := platform.CertificateError{
			URL:         "https://" + origin,
			Host:        host,
			Error:       goString(send(nserr, "localizedDescription")), // a CFError is an NSError
			Certificate: leafCertificate(api, trust),
		}
		if !w.h.CertificateError(e) {
			callBlock(handler, authDefault, 0)
			return
		}
		withPool(func() {
			callBlock(handler, authUseCredential, uintptr(send(class("NSURLCredential"), "credentialForTrust:", uintptr(trust))))
		})
	})
	status, _, _ := purego.SyscallN(api.evaluate, uintptr(trust), api.mainQueue, uintptr(block))
	block.Release()
	if int32(status) != 0 { // errSecSuccess: the block never runs otherwise
		release(challenge)
		callBlock(handler, authDefault, 0)
		blockRelease(handler)
	}
}

// leafCertificate returns the DER encoding of the server's own
// certificate in trust.
func leafCertificate(api *trustFuncs, trust id) []byte {
	chain, _, _ := purego.SyscallN(api.chain, uintptr(trust))
	if chain == 0 {
		return nil
	}
	defer release(id(chain))
	if sendInt(id(chain), "count") == 0 {
		return nil
	}
	data, _, _ := purego.SyscallN(api.data, uintptr(send(id(chain), "objectAtIndex:", 0)))
	defer release(id(data))
	return goBytes(id(data))
}

// identityAPI is what listing the keychains' identities takes of the
// Security framework, loaded with the first client certificate request
// the app handles.
var identityAPI = sync.OnceValue(func() *identityFuncs {
	lib, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		return nil
	}
	return &identityFuncs{
		copyMatching:    mustDlsym(lib, "SecItemCopyMatching"),
		copyCertificate: mustDlsym(lib, "SecIdentityCopyCertificate"),
		data:            mustDlsym(lib, "SecCertificateCopyData"),
		class:           constString(lib, "kSecClass"),
		classIdentity:   constString(lib, "kSecClassIdentity"),
		returnRef:       constString(lib, "kSecReturnRef"),
		matchLimit:      constString(lib, "kSecMatchLimit"),
		matchLimitAll:   constString(lib, "kSecMatchLimitAll"),
		matchIssuers:    constString(lib, "kSecMatchIssuers"),
		cfTrue:          constString(libCF, "kCFBooleanTrue"),
	}
})

type identityFuncs struct {
	copyMatching, copyCertificate, data uintptr

	class, classIdentity, returnRef, matchLimit, matchLimitAll, matchIssuers, cfTrue id
}

// clientCertificate asks the app which identity of the keychains to
// present to a server that asks for a client certificate. WebKit's
// default handling goes on without one.
func (w *window) clientCertificate(space id, handler uintptr) {
	api := identityAPI()
	if api == nil || !w.h.HandlesClientCertificates() {
		callBlock(handler, authDefault, 0)
		return
	}
	withPool(func() {
		req := platform.ClientCertificateRequest{
			Host: goString(send(space, "host")),
			Port: sendInt(space, "port"),
		}
		// The issuers the server accepts, DER-encoded X.501 names.
		names := send(space, "distinguishedNames")
		for _, name := range arrayItems(names) {
			req.Issuers = append(req.Issuers, goBytes(name))
		}
		found, all := api.identities(names)
		if found != 0 {
			defer cfRelease(found)
		}
		// Those whose certificate reads, by the index the app answers with.
		var identities []id
		for _, identity := range all {
			if der := api.certificate(identity); der != nil {
				identities = append(identities, identity)
				req.Certificates = append(req.Certificates, der)
			}
		}
		resp := w.h.ClientCertificateRequested(req)
		switch {
		case resp.Default:
			callBlock(handler, authDefault, 0)
		case resp.Index < 0 || resp.Index >= len(identities):
			// No certificate: the connection goes on without one.
			callBlock(handler, authReject, 0)
		default:
			const forSession = 1 // NSURLCredentialPersistenceForSession
			cred := send(class("NSURLCredential"), "credentialWithIdentity:certificates:persistence:",
				uintptr(identities[resp.Index]), 0, forSession)
			callBlock(handler, authUseCredential, uintptr(cred))
		}
	})
}

// identities finds the identities of the keychains whose chain has one
// of issuers, an NSArray of DER-encoded names, or all of them when it
// is empty. It returns the CFArray it found them in, which the caller
// releases, and the identities, which that array keeps.
func (api *identityFuncs) identities(issuers id) (uintptr, []id) {
	q := send(class("NSMutableDictionary"), "dictionary")
	set := func(key, v id) { send(q, "setObject:forKey:", uintptr(v), uintptr(key)) }
	set(api.class, api.classIdentity)
	set(api.returnRef, api.cfTrue)
	set(api.matchLimit, api.matchLimitAll)
	if issuers != 0 && sendInt(issuers, "count") > 0 {
		set(api.matchIssuers, issuers)
	}
	var found uintptr
	status, _, _ := purego.SyscallN(api.copyMatching, uintptr(q), uintptr(unsafe.Pointer(&found)))
	if osStatus(status) != 0 || found == 0 { // errSecItemNotFound: none
		return 0, nil
	}
	return found, arrayItems(id(found))
}

// certificate returns the DER encoding of an identity's certificate.
func (api *identityFuncs) certificate(identity id) []byte {
	var cert uintptr
	status, _, _ := purego.SyscallN(api.copyCertificate, uintptr(identity), uintptr(unsafe.Pointer(&cert)))
	if osStatus(status) != 0 || cert == 0 {
		return nil
	}
	defer cfRelease(cert)
	data, _, _ := purego.SyscallN(api.data, cert)
	if data == 0 {
		return nil
	}
	defer cfRelease(data)
	return goBytes(id(data))
}
