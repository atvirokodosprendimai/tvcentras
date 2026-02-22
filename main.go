package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", homePage)
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, page)
}

const page = `<!DOCTYPE html>
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
      display: flex;
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
  </style>
</head>
<body>
  <div class="mug">🍺</div>
</body>
</html>`
