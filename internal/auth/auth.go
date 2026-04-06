package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	sessionDuration = 24 * time.Hour
	tokenLen        = 32
	cookieName      = "qbitctrl_session"
	configFile      = "qbitctrl_auth.json"
)

type authConfig struct {
	PasswordHash string `json:"password_hash"`
	Token        string `json:"token"` // The generated access token (shown once)
}

type session struct {
	createdAt time.Time
}

type Manager struct {
	mu           sync.RWMutex
	passwordHash string
	sessions     map[string]*session
	configPath   string
}

// New creates or loads an auth manager.
// If no config exists, generates a new random password and prints it.
func New(configPath string) *Manager {
	m := &Manager{
		sessions:   make(map[string]*session),
		configPath: configPath,
	}
	m.load()
	return m
}

func (m *Manager) load() {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		// First run — generate password
		m.generatePassword()
		return
	}
	var cfg authConfig
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.PasswordHash == "" {
		m.generatePassword()
		return
	}
	m.passwordHash = cfg.PasswordHash
	log.Printf("[Auth] Załadowano konfigurację z %s", m.configPath)
}

func (m *Manager) generatePassword() {
	// Generate cryptographically random password
	raw := make([]byte, 16)
	rand.Read(raw)
	password := hex.EncodeToString(raw) // 32 chars hex
	hash := hashPassword(password)

	m.passwordHash = hash
	m.save(password)

	// Print prominently to console
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║           qBitCtrl — PIERWSZE URUCHOMIENIE           ║")
	fmt.Println("╠══════════════════════════════════════════════════════╣")
	fmt.Printf( "║  Hasło dostępu: %-36s ║\n", password)
	fmt.Println("║                                                      ║")
	fmt.Println("║  Zapisane w: qbitctrl_auth.json                     ║")
	fmt.Println("║  Zmień hasło: edytuj plik lub usuń go i uruchom      ║")
	fmt.Println("║  ponownie żeby wygenerować nowe.                     ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()
}

func (m *Manager) save(plainPassword string) {
	cfg := authConfig{
		PasswordHash: m.passwordHash,
		Token:        plainPassword,
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(m.configPath, data, 0600); err != nil {
		log.Printf("WARN: nie można zapisać auth config: %v", err)
	}
}

func hashPassword(password string) string {
	h := sha256.Sum256([]byte(password))
	return hex.EncodeToString(h[:])
}

// Login validates password and creates a session, returns session token
func (m *Manager) Login(password string) (string, bool) {
	if hashPassword(password) != m.passwordHash {
		return "", false
	}
	token := make([]byte, tokenLen)
	rand.Read(token)
	sessionToken := hex.EncodeToString(token)

	m.mu.Lock()
	m.sessions[sessionToken] = &session{createdAt: time.Now()}
	m.mu.Unlock()

	return sessionToken, true
}

// Logout removes a session
func (m *Manager) Logout(token string) {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

// IsValid checks if a session token is valid
func (m *Manager) IsValid(token string) bool {
	m.mu.RLock()
	s, ok := m.sessions[token]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Since(s.createdAt) > sessionDuration {
		m.mu.Lock()
		delete(m.sessions, token)
		m.mu.Unlock()
		return false
	}
	return true
}

// GetToken extracts session token from cookie or Authorization header
func GetToken(r *http.Request) string {
	if c, err := r.Cookie(cookieName); err == nil {
		return c.Value
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// Middleware protects routes — redirects to login page if not authenticated
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always allow login endpoint and static assets
		if r.URL.Path == "/login" || r.URL.Path == "/api/login" || r.URL.Path == "/api/logout" {
			next.ServeHTTP(w, r)
			return
		}
		token := GetToken(r)
		if !m.IsValid(token) {
			// API request → 401 JSON
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			// Browser request → redirect to login
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// HandleLogin handles POST /api/login
func (m *Manager) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Serve login page
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(loginHTML))
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Password == "" {
		// Try form
		r.ParseForm()
		body.Password = r.FormValue("password")
	}
	token, ok := m.Login(body.Password)
	if !ok {
		time.Sleep(500 * time.Millisecond) // brute force delay
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"Nieprawidłowe hasło"}`))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// HandleLogout handles POST /api/logout
func (m *Manager) HandleLogout(w http.ResponseWriter, r *http.Request) {
	token := GetToken(r)
	m.Logout(token)
	http.SetCookie(w, &http.Cookie{
		Name:   cookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// ChangePassword updates the password
func (m *Manager) ChangePassword(oldPassword, newPassword string) bool {
	if hashPassword(oldPassword) != m.passwordHash {
		return false
	}
	m.mu.Lock()
	m.passwordHash = hashPassword(newPassword)
	// Invalidate all sessions
	m.sessions = make(map[string]*session)
	m.mu.Unlock()
	m.save(newPassword)
	return true
}

var loginHTML = `<!DOCTYPE html>
<html lang="pl">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>qBitCtrl — Logowanie</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0a0c10;color:#cdd9e5;font-family:'Segoe UI',sans-serif;
  min-height:100vh;display:flex;align-items:center;justify-content:center}
.card{background:#0f1218;border:1px solid #1e2530;border-radius:8px;
  padding:40px;width:360px;max-width:95vw}
.logo{font-size:24px;font-weight:700;letter-spacing:3px;color:#00e5a0;
  text-align:center;margin-bottom:8px}
.sub{text-align:center;color:#4a5568;font-size:12px;margin-bottom:32px;
  font-family:monospace}
label{display:block;font-size:11px;font-weight:700;letter-spacing:1.5px;
  color:#7a8fa3;text-transform:uppercase;margin-bottom:6px}
input[type=password]{width:100%;background:#0a0c10;border:1px solid #2a3340;
  color:#cdd9e5;font-family:monospace;font-size:14px;padding:10px 14px;
  border-radius:4px;outline:none;transition:border-color .15s}
input[type=password]:focus{border-color:#00e5a0}
button{width:100%;margin-top:20px;padding:12px;background:#00e5a0;color:#000;
  font-weight:700;font-size:14px;letter-spacing:1px;border:none;border-radius:4px;
  cursor:pointer;transition:background .15s}
button:hover{background:#00ffc0}
.err{color:#ff4455;font-size:12px;margin-top:10px;text-align:center;
  font-family:monospace;min-height:16px}
</style>
</head>
<body>
<div class="card">
  <div class="logo">QBIT<span style="color:#cdd9e5">CTRL</span></div>
  <div class="sub">Panel zarządzania qBittorrent</div>
  <label>Hasło dostępu</label>
  <input type="password" id="pw" placeholder="••••••••••••••••"
    onkeydown="if(event.key==='Enter')login()">
  <button onclick="login()">ZALOGUJ</button>
  <div class="err" id="err"></div>
</div>
<script>
async function login() {
  const pw = document.getElementById('pw').value;
  const err = document.getElementById('err');
  err.textContent = '';
  try {
    const r = await fetch('/api/login', {
      method: 'POST',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify({password: pw})
    });
    const d = await r.json();
    if (d.ok) {
      window.location.href = '/';
    } else {
      err.textContent = d.error || 'Nieprawidłowe hasło';
      document.getElementById('pw').value = '';
    }
  } catch(e) {
    err.textContent = 'Błąd połączenia';
  }
}
document.getElementById('pw').focus();
</script>
</body>
</html>`
