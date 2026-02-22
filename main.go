package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", requireAuth(homePage))
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/logout", handleLogout)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	log.Println("TV dashboard starting on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func homePage(w http.ResponseWriter, r *http.Request) {
	fp := currentFingerprint(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, renderHomePage(fp))
}

func renderHomePage(fingerprint string) string {
	return `<!DOCTYPE html>
<html lang="lt">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>TV Centras · BeerPub</title>
  <script type="module" src="https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.0-RC.7/bundles/datastar.js"></script>
  <style>
    *, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }
    html, body {
      width: 100vw;
      height: 100vh;
      background: #0d0d0d;
      color: #e0e0e0;
      font-family: monospace;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      overflow: hidden;
    }
    .mug {
      font-size: 35vmin;
      line-height: 1;
      filter: drop-shadow(0 0 6vmin rgba(255, 180, 0, 0.4));
      animation: breathe 5s ease-in-out infinite;
      user-select: none;
    }
    @keyframes breathe {
      0%, 100% { transform: scale(1) translateY(0); }
      50%       { transform: scale(1.03) translateY(-0.5%); }
    }
    .identity {
      margin-top: 1.5rem;
      font-size: .75rem;
      color: #666;
      text-align: center;
    }
    .identity span { color: #ffb400; }
    .identity a {
      color: #555;
      text-decoration: none;
      margin-left: .75rem;
    }
    .identity a:hover { color: #ffb400; }
  </style>
</head>
<body>
  <div class="mug">🍺</div>
  <div class="identity">
    <span>` + fingerprint + `</span>
    <a href="/logout">logout</a>
  </div>
</body>
</html>`
}

func renderLoginPage(nonce, errMsg string) string {
	errHTML := ""
	if errMsg != "" {
		errHTML = `<div class="error">⚠ ` + errMsg + `</div>`
	}
	return `<!DOCTYPE html>
<html lang="lt">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>TV Centras · Login</title>
  <style>
    *, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }
    html, body {
      width: 100vw; min-height: 100vh;
      background: #0d0d0d; color: #e0e0e0;
      font-family: monospace;
      display: flex; align-items: center; justify-content: center;
    }
    .card {
      background: #1a1a1a; border: 1px solid #333; border-radius: 8px;
      padding: 2rem; max-width: 640px; width: 90%;
    }
    h1 { color: #ffb400; margin-bottom: 1.5rem; font-size: 1.2rem; }
    label { display: block; color: #aaa; font-size: .85rem; margin-top: 1.25rem; margin-bottom: .3rem; }
    .nonce {
      display: block; padding: .5rem;
      background: #111; border: 1px solid #333; border-radius: 4px;
      color: #ffb400; font-size: .85rem; word-break: break-all;
    }
    pre {
      padding: .75rem; margin-top: .3rem;
      background: #111; border: 1px solid #333; border-radius: 4px;
      white-space: pre-wrap; word-break: break-all;
      font-size: .8rem; line-height: 1.6; color: #ccc;
    }
    textarea {
      width: 100%; height: 140px; margin-top: .3rem;
      background: #111; color: #e0e0e0; border: 1px solid #333;
      border-radius: 4px; padding: .5rem;
      font-family: monospace; font-size: .8rem; resize: vertical;
    }
    button {
      margin-top: 1.5rem; padding: .6rem 1.5rem;
      background: #ffb400; color: #000; font-weight: bold;
      border: none; border-radius: 4px; cursor: pointer; font-size: 1rem;
    }
    button:hover { background: #ffc933; }
    .error { margin-top: 1rem; padding: .5rem; background: #2a1111; border: 1px solid #662222; border-radius: 4px; color: #ff6b6b; font-size: .85rem; }
  </style>
</head>
<body>
  <div class="card">
    <h1>🍺 TV Centras — EdProof Login</h1>
    ` + errHTML + `
    <label>Step 1 — Copy your nonce:</label>
    <code class="nonce">` + nonce + `</code>

    <label>Step 2 — Sign it with your SSH Ed25519 key:</label>
    <pre>printf '%s' '` + nonce + `' > /tmp/edproof_msg
ssh-keygen -Y sign -f ~/.ssh/id_ed25519 -n edproof /tmp/edproof_msg
cat /tmp/edproof_msg.sig</pre>

    <form method="POST" action="/login">
      <input type="hidden" name="nonce" value="` + nonce + `">
      <label>Step 3 — Paste the signature (-----BEGIN SSH SIGNATURE----- … -----END SSH SIGNATURE-----):</label>
      <textarea name="signature" placeholder="-----BEGIN SSH SIGNATURE-----&#10;...&#10;-----END SSH SIGNATURE-----" required></textarea>
      <button type="submit">Login</button>
    </form>
  </div>
</body>
</html>`
}
