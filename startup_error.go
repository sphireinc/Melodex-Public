package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func newAssetHandler(assets fs.FS) http.Handler {
	assetHandler := application.AssetFileServerFS(assets)
	startupErrorHandler := newStartupErrorHandler(assets)
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		recorder := httptest.NewRecorder()
		assetHandler.ServeHTTP(recorder, req)
		if recorder.Code == http.StatusNotFound && shouldServeStartupErrorPage(req) {
			startupErrorHandler.ServeHTTP(rw, req)
			return
		}
		for key, values := range recorder.Header() {
			for _, value := range values {
				rw.Header().Add(key, value)
			}
		}
		rw.WriteHeader(recorder.Code)
		_, _ = rw.Write(recorder.Body.Bytes())
	})
}

func newStartupErrorHandler(assets fs.FS) http.Handler {
	logoDataURI := loadEmbeddedDataURI(assets, "logo_cropped.png")
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if !shouldServeStartupErrorPage(req) {
			http.NotFound(rw, req)
			return
		}
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(rw, renderStartupErrorHTML(logoDataURI))
	})
}

func shouldServeStartupErrorPage(req *http.Request) bool {
	if req == nil || req.Method != http.MethodGet {
		return false
	}
	pathname := strings.TrimSpace(req.URL.Path)
	if pathname == "" || pathname == "/" || strings.EqualFold(path.Base(pathname), "index.html") {
		return true
	}
	accept := strings.ToLower(req.Header.Get("Accept"))
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "application/xhtml+xml")
}

func loadEmbeddedDataURI(assets fs.FS, filename string) string {
	if assets == nil {
		return ""
	}
	data, err := fs.ReadFile(assets, filename)
	if err != nil {
		return ""
	}
	contentType := mime.TypeByExtension(path.Ext(filename))
	if contentType == "" {
		contentType = "image/png"
	}
	var buf bytes.Buffer
	buf.WriteString("data:")
	buf.WriteString(contentType)
	buf.WriteString(";base64,")
	buf.WriteString(base64.StdEncoding.EncodeToString(data))
	return buf.String()
}

func renderStartupErrorHTML(logoDataURI string) string {
	logoMarkup := ""
	if strings.TrimSpace(logoDataURI) != "" {
		logoMarkup = fmt.Sprintf(`<img class="melodex-error-logo" src="%s" alt="Melodex">`, logoDataURI)
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Melodex could not load</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #090e19;
      --panel: rgba(17, 22, 36, 0.92);
      --panel-border: rgba(255, 255, 255, 0.09);
      --text: #f4f7fb;
      --muted: #94a3b8;
      --accent: #e24d4d;
      --accent-2: #f26e6e;
      --shadow: 0 32px 80px rgba(0, 0, 0, 0.46);
    }
    * { box-sizing: border-box; }
    html, body {
      margin: 0;
      min-height: 100%%;
      background:
        radial-gradient(circle at top, rgba(226, 77, 77, 0.16), transparent 30%%),
        linear-gradient(180deg, #0b1020 0%%, #090e19 100%%);
      color: var(--text);
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    }
    body {
      min-height: 100vh;
      display: grid;
      place-items: center;
      padding: 28px;
    }
    .shell {
      width: min(760px, 100%%);
      border: 1px solid var(--panel-border);
      background: var(--panel);
      border-radius: 28px;
      box-shadow: var(--shadow);
      overflow: hidden;
      backdrop-filter: blur(18px);
    }
    .hero {
      display: grid;
      grid-template-columns: auto 1fr;
      gap: 18px;
      padding: 32px;
      align-items: center;
      border-bottom: 1px solid rgba(255, 255, 255, 0.06);
    }
    .logo-wrap {
      width: 88px;
      height: 88px;
      border-radius: 22px;
      display: grid;
      place-items: center;
      background: rgba(255, 255, 255, 0.04);
      border: 1px solid rgba(255, 255, 255, 0.08);
      overflow: hidden;
    }
    .logo-wrap img {
      width: 100%%;
      height: 100%%;
      object-fit: cover;
      display: block;
    }
    .eyebrow {
      text-transform: uppercase;
      letter-spacing: 0.18em;
      color: var(--muted);
      font-size: 0.72rem;
      margin: 0 0 8px;
    }
    h1 {
      margin: 0;
      font-size: clamp(2rem, 4vw, 3.1rem);
      line-height: 1.05;
    }
    .lede {
      margin: 10px 0 0;
      color: var(--muted);
      font-size: 1rem;
      line-height: 1.6;
      max-width: 62ch;
    }
    .body {
      padding: 24px 32px 32px;
      display: grid;
      gap: 18px;
    }
    .panel {
      border-radius: 22px;
      border: 1px solid rgba(255, 255, 255, 0.08);
      background: rgba(255, 255, 255, 0.03);
      padding: 18px 20px;
    }
    .panel-title {
      display: flex;
      gap: 10px;
      align-items: center;
      margin: 0 0 10px;
      font-size: 0.85rem;
      text-transform: uppercase;
      letter-spacing: 0.14em;
      color: var(--muted);
    }
    .panel-copy {
      margin: 0;
      color: var(--text);
      line-height: 1.6;
    }
    .actions {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      margin-top: 6px;
    }
    .button {
      appearance: none;
      border: 0;
      border-radius: 999px;
      padding: 13px 18px;
      font-size: 0.98rem;
      font-weight: 600;
      cursor: pointer;
      transition: transform 140ms ease, box-shadow 140ms ease, background 140ms ease;
      text-decoration: none;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
    }
    .button:hover {
      transform: translateY(-1px);
    }
    .button-primary {
      color: white;
      background: linear-gradient(135deg, var(--accent), var(--accent-2));
      box-shadow: 0 14px 30px rgba(226, 77, 77, 0.26);
    }
    .button-secondary {
      color: var(--text);
      background: rgba(255, 255, 255, 0.06);
      border: 1px solid rgba(255, 255, 255, 0.1);
    }
    .hint {
      color: var(--muted);
      font-size: 0.92rem;
      line-height: 1.55;
      margin: 0;
    }
    code {
      padding: 2px 8px;
      border-radius: 999px;
      background: rgba(255, 255, 255, 0.08);
      color: #fff;
    }
    @media (max-width: 640px) {
      .hero,
      .body {
        padding-left: 20px;
        padding-right: 20px;
      }
      .hero {
        grid-template-columns: 1fr;
      }
      .logo-wrap {
        width: 72px;
        height: 72px;
      }
    }
  </style>
</head>
<body>
  <main class="shell">
    <section class="hero">
      <div class="logo-wrap">%s</div>
      <div>
        <p class="eyebrow">Melodex startup error</p>
        <h1>The app UI could not load</h1>
        <p class="lede">Melodex could not find the frontend entry page it needs to render the library UI. This is usually a build, packaging, or asset-path problem. The app can usually recover once the frontend is rebuilt or the bundle is present again.</p>
      </div>
    </section>
    <section class="body">
      <div class="panel">
        <div class="panel-title">What to try</div>
        <p class="panel-copy">If this is a development launch, run <code>npm run build</code> in the frontend and restart Melodex. If this is a packaged build, reinstall or repackage the app so the embedded frontend assets are present.</p>
      </div>
      <div class="panel">
        <div class="panel-title">Recovery</div>
        <p class="hint">Try reloading the app first. If the page is still missing, the frontend bundle is probably not available to Wails yet.</p>
        <div class="actions">
          <button class="button button-primary" onclick="window.location.reload()">Reload app</button>
          <button class="button button-secondary" onclick="window.location.href='/'">Go home</button>
        </div>
      </div>
    </section>
  </main>
</body>
</html>`, logoMarkup)
}
