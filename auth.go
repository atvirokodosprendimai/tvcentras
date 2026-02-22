package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	nonceTTL   = 5 * time.Minute
	sessionTTL = 24 * time.Hour
	cookieName = "edproof_session"
)

var (
	nonceMu  sync.Mutex
	nonceDB  = make(map[string]time.Time) // nonce → expiry
	sessMu   sync.Mutex
	sessionDB = make(map[string]sessEntry) // token → entry
)

type sessEntry struct {
	fingerprint string
	expires     time.Time
}

// generateNonce creates a fresh single-use nonce, stores it, and returns it.
func generateNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nonce := base64.RawURLEncoding.EncodeToString(b)
	nonceMu.Lock()
	defer nonceMu.Unlock()
	now := time.Now()
	for k, exp := range nonceDB {
		if now.After(exp) {
			delete(nonceDB, k)
		}
	}
	nonceDB[nonce] = now.Add(nonceTTL)
	return nonce, nil
}

// consumeNonce removes the nonce and reports whether it was valid and unexpired.
func consumeNonce(nonce string) bool {
	nonceMu.Lock()
	defer nonceMu.Unlock()
	exp, ok := nonceDB[nonce]
	if !ok {
		return false
	}
	delete(nonceDB, nonce)
	return time.Now().Before(exp)
}

// createSession stores a new session for fingerprint and returns the token.
func createSession(fingerprint string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	sessMu.Lock()
	defer sessMu.Unlock()
	sessionDB[token] = sessEntry{fingerprint: fingerprint, expires: time.Now().Add(sessionTTL)}
	return token, nil
}

// lookupSession returns the fingerprint for token, or "" if invalid/expired.
func lookupSession(token string) string {
	sessMu.Lock()
	defer sessMu.Unlock()
	e, ok := sessionDB[token]
	if !ok || time.Now().After(e.expires) {
		delete(sessionDB, token)
		return ""
	}
	return e.fingerprint
}

// deleteSession removes a session token.
func deleteSession(token string) {
	sessMu.Lock()
	defer sessMu.Unlock()
	delete(sessionDB, token)
}

// currentFingerprint returns the fingerprint from the session cookie, or "".
func currentFingerprint(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return lookupSession(c.Value)
}

// requireAuth redirects unauthenticated requests to /login.
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if currentFingerprint(r) == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// handleLogin dispatches GET and POST /login.
func handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		loginGet(w, r)
	case http.MethodPost:
		loginPost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func loginGet(w http.ResponseWriter, r *http.Request) {
	nonce, err := generateNonce()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Replay-Nonce", nonce)
	w.Header().Set("WWW-Authenticate", `EdProof realm="edproof"`)
	fmt.Fprint(w, renderLoginPage(nonce, ""))
}

func loginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	nonce := r.FormValue("nonce")
	sig := r.FormValue("signature")
	if nonce == "" || sig == "" {
		http.Error(w, "missing fields", http.StatusBadRequest)
		return
	}
	if !consumeNonce(nonce) {
		newNonce, err := generateNonce()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Replay-Nonce", newNonce)
		fmt.Fprint(w, renderLoginPage(newNonce, "Nonce expired or already used — please try again."))
		return
	}
	fingerprint, err := verifySSHSig([]byte(nonce), sig, "edproof")
	if err != nil {
		newNonce, err2 := generateNonce()
		if err2 != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Replay-Nonce", newNonce)
		fmt.Fprint(w, renderLoginPage(newNonce, "Signature verification failed: "+err.Error()))
		return
	}
	token, err := createSession(fingerprint)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout clears the session and redirects to /login.
func handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		deleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// verifySSHSig parses an SSHSIG-armored signature, verifies it covers message
// with the given namespace, and returns the signer's key fingerprint.
//
// The on-disk SSHSIG format (PROTOCOL.sshsig) is:
//
//	byte[6]  magic "SSHSIG"      (raw, not length-prefixed)
//	uint32   version             (= 1)
//	string   public_key          (SSH wire format)
//	string   namespace
//	string   reserved            (empty)
//	string   hash_algorithm
//	string   signature           (SSH signature wire format)
//
// The data that was signed is:
//
//	string   "SSHSIG"            (length-prefixed)
//	string   namespace
//	string   ""                  (reserved)
//	string   hash_algorithm
//	string   H(message)          (hash bytes, not hex)
func verifySSHSig(message []byte, armored, namespace string) (string, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(armored)))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block")
	}
	if block.Type != "SSH SIGNATURE" {
		return "", fmt.Errorf("expected type 'SSH SIGNATURE', got %q", block.Type)
	}

	const magic = "SSHSIG"
	data := block.Bytes
	if len(data) < len(magic) || string(data[:len(magic)]) != magic {
		return "", fmt.Errorf("invalid SSHSIG magic")
	}
	data = data[len(magic):]

	var wrapper struct {
		Version       uint32
		PublicKey     []byte
		Namespace     string
		Reserved      string
		HashAlgorithm string
		Signature     []byte
	}
	if err := ssh.Unmarshal(data, &wrapper); err != nil {
		return "", fmt.Errorf("parsing SSHSIG wrapper: %w", err)
	}
	if wrapper.Version != 1 {
		return "", fmt.Errorf("unsupported SSHSIG version %d", wrapper.Version)
	}
	if wrapper.Namespace != namespace {
		return "", fmt.Errorf("namespace mismatch: got %q, want %q", wrapper.Namespace, namespace)
	}

	pubKey, err := ssh.ParsePublicKey(wrapper.PublicKey)
	if err != nil {
		return "", fmt.Errorf("parsing public key: %w", err)
	}
	if pubKey.Type() != "ssh-ed25519" {
		return "", fmt.Errorf("only ssh-ed25519 keys are accepted, got %q", pubKey.Type())
	}

	var sigWire struct {
		Format string
		Blob   []byte
	}
	if err := ssh.Unmarshal(wrapper.Signature, &sigWire); err != nil {
		return "", fmt.Errorf("parsing signature wire format: %w", err)
	}

	var msgHash []byte
	switch wrapper.HashAlgorithm {
	case "sha256":
		h := sha256.Sum256(message)
		msgHash = h[:]
	case "sha512":
		h := sha512.Sum512(message)
		msgHash = h[:]
	default:
		return "", fmt.Errorf("unsupported hash algorithm %q", wrapper.HashAlgorithm)
	}

	// Build the SSHSIG signing body per PROTOCOL.sshsig / OpenSSH sshsig.c:
	//   byte[6]  "SSHSIG"  (raw, no length prefix – written with sshbuf_put, not sshbuf_put_cstring)
	//   string   namespace
	//   string   reserved  (empty)
	//   string   hash_algorithm
	//   string   H(message)
	appendSSHStr := func(b, s []byte) []byte {
		ln := make([]byte, 4)
		binary.BigEndian.PutUint32(ln, uint32(len(s)))
		return append(append(b, ln...), s...)
	}
	signedData := []byte(magic) // raw magic, no uint32 length prefix
	signedData = appendSSHStr(signedData, []byte(wrapper.Namespace))
	signedData = appendSSHStr(signedData, []byte{}) // reserved
	signedData = appendSSHStr(signedData, []byte(wrapper.HashAlgorithm))
	signedData = appendSSHStr(signedData, msgHash)

	if err := pubKey.Verify(signedData, &ssh.Signature{
		Format: sigWire.Format,
		Blob:   sigWire.Blob,
	}); err != nil {
		return "", fmt.Errorf("signature invalid: %w", err)
	}

	return ssh.FingerprintSHA256(pubKey), nil
}
