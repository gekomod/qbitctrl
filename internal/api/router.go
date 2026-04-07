package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qbitctrl/internal/auth"
	"github.com/qbitctrl/internal/config"
	"github.com/qbitctrl/internal/models"
	"github.com/qbitctrl/internal/qbit"
	"github.com/qbitctrl/internal/scheduler"
	sshpkg "github.com/qbitctrl/internal/ssh"
	"github.com/qbitctrl/internal/stats"
	"github.com/qbitctrl/internal/store"
	"github.com/qbitctrl/internal/websocket"
)

type Router struct {
	mux   *http.ServeMux
	store *store.Store
	sched *scheduler.Scheduler
	hub   *websocket.Hub
	stats *stats.Collector
	auth  *auth.Manager
	cfg   *config.Config
}

func NewRouter(st *store.Store, sched *scheduler.Scheduler, hub *websocket.Hub, sc *stats.Collector, am *auth.Manager, cfg *config.Config) http.Handler {
	r := &Router{
		mux:   http.NewServeMux(),
		store: st,
		sched: sched,
		hub:   hub,
		stats: sc,
		auth:  am,
		cfg:   cfg,
	}
	r.registerRoutes()
	return cors(r.mux)
}

func (r *Router) registerRoutes() {
	// Public — login page and API
	r.mux.HandleFunc("/login",       r.auth.HandleLogin)
	r.mux.HandleFunc("/api/login",   r.auth.HandleLogin)
	r.mux.HandleFunc("/api/logout",  r.auth.HandleLogout)
	r.mux.HandleFunc("/api/change_password", r.handleChangePassword)

	// Health check (public — for monitoring)
	r.mux.HandleFunc("/health", r.handleHealth)

	// Protected routes — wrapped with auth middleware
	protected := r.auth.Middleware(http.HandlerFunc(r.handleProtected))
	r.mux.Handle("/", protected)
}

// handleProtected routes all protected endpoints
func (r *Router) handleProtected(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	switch {
	case path == "/":
		r.handleIndex(w, req)
	case path == "/api/events":
		r.hub.ServeSSE(w, req)
	case path == "/api/overview":
		r.handleOverview(w, req)
	case path == "/api/ping":
		r.handlePing(w, req)
	case path == "/api/servers" || path == "/api/servers/":
		r.handleServers(w, req)
	case strings.HasPrefix(path, "/api/servers/"):
		r.handleServerDetail(w, req)
	case path == "/api/arr/config":
		r.handleARRConfig(w, req)
	case path == "/api/arr/config/full":
		r.handleARRConfigFull(w, req)
	case path == "/api/radarr/test":
		r.handleRadarrTest(w, req)
	case path == "/api/sonarr/test":
		r.handleSonarrTest(w, req)
	case path == "/api/radarr/queue":
		r.handleRadarrQueue(w, req)
	case path == "/api/sonarr/queue":
		r.handleSonarrQueue(w, req)
	case path == "/api/radarr/search":
		r.handleArrSearch(w, req)
	case path == "/api/sonarr/search":
		r.handleArrSearch(w, req)
	case path == "/api/radarr/add":
		r.handleArrAdd(w, req)
	case path == "/api/sonarr/add":
		r.handleArrAdd(w, req)
	case path == "/api/radarr/recent":
		r.handleRadarrRecent(w, req)
	case path == "/api/sonarr/recent":
		r.handleSonarrRecent(w, req)
	case path == "/api/radarr/library":
		r.handleRadarrLibrary(w, req)
	case path == "/api/sonarr/library":
		r.handleSonarrLibrary(w, req)
	case path == "/api/radarr/health":
		r.handleRadarrHealth(w, req)
	case path == "/api/sonarr/health":
		r.handleSonarrHealth(w, req)
	case path == "/api/radarr/lookup":
		r.handleRadarrLookup(w, req)
	case path == "/api/sonarr/lookup":
		r.handleSonarrLookup(w, req)
	default:
		http.NotFound(w, req)
	}
}

func (r *Router) handleChangePassword(w http.ResponseWriter, req *http.Request) {
	// Only POST
	if req.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	// Must be authenticated
	token := auth.GetToken(req)
	if !r.auth.IsValid(token) {
		errJSON(w, 401, "unauthorized")
		return
	}
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := readJSON(req, &body); err != nil {
		errJSON(w, 400, "invalid JSON")
		return
	}
	if len(body.NewPassword) < 8 {
		errJSON(w, 400, "nowe hasło musi mieć co najmniej 8 znaków")
		return
	}
	if !r.auth.ChangePassword(body.OldPassword, body.NewPassword) {
		errJSON(w, 401, "nieprawidłowe stare hasło")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── Index ─────────────────────────────────────────────────────────────────────

func (r *Router) handleIndex(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/" {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(indexHTML))
}

// ── Servers ───────────────────────────────────────────────────────────────────

func (r *Router) handleServers(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		servers := r.store.GetAll()
		dtos := make([]models.ServerDTO, len(servers))
		for i, s := range servers {
			dtos[i] = s.ToDTO()
		}
		writeJSON(w, 200, dtos)

	case http.MethodPost:
		var body struct {
			Name                string `json:"name"`
			Host                string `json:"host"`
			Port                int    `json:"port"`
			Username            string `json:"username"`
			Password            string `json:"password"`
			HTTPS               bool   `json:"https"`
			RestartType         string `json:"restart_type"`
			RestartUnit         string `json:"restart_unit"`
			SSHUser             string `json:"ssh_user"`
			SSHPort             int    `json:"ssh_port"`
			SSHKeyPath          string `json:"ssh_key_path"`
			AutoRestart         bool   `json:"auto_restart"`
			AutoRestartInterval int    `json:"auto_restart_interval"`
		}
		if err := readJSON(req, &body); err != nil {
			errJSON(w, 400, "invalid JSON")
			return
		}
		if body.Name == "" || body.Host == "" || body.Port == 0 {
			errJSON(w, 400, "name, host, port są wymagane")
			return
		}
		id := strings.ToLower(strings.ReplaceAll(body.Name, " ", "-"))
		s := models.NewServer(id, body.Name, body.Host, body.Port)
		s.Username = body.Username
		s.Password = body.Password
		s.HTTPS = body.HTTPS
		s.RestartType = body.RestartType
		s.RestartUnit = body.RestartUnit
		s.SSHUser = body.SSHUser
		if s.SSHUser == "" {
			s.SSHUser = "root"
		}
		s.SSHPort = body.SSHPort
		if s.SSHPort == 0 {
			s.SSHPort = 22
		}
		s.SSHKeyPath = body.SSHKeyPath
		s.AutoRestart = body.AutoRestart
		s.AutoRestartInterval = body.AutoRestartInterval
		if s.AutoRestartInterval == 0 {
			s.AutoRestartInterval = 60
		}
		ok := qbit.Login(s)
		if ok {
			ver, _ := qbit.Version(s)
			s.Version = ver
		}
		r.store.Add(s)
		r.store.Save()
		msg := "Serwer dodany, zalogowano"
		if !ok {
			msg = "Serwer dodany, ale logowanie nieudane"
		}
		writeJSON(w, 201, map[string]interface{}{
			"server":   s.ToDTO(),
			"login_ok": ok,
			"message":  msg,
		})
	default:
		w.WriteHeader(405)
	}
}

func (r *Router) handleServerDetail(w http.ResponseWriter, req *http.Request) {
	// Path: /api/servers/<id>[/<action>[/<sub>[/<subsub>]]]
	path  := strings.TrimPrefix(req.URL.Path, "/api/servers/")
	parts := strings.Split(path, "/") // unlimited split — no truncation
	if len(parts) == 0 || parts[0] == "" {
		errJSON(w, 400, "missing server id")
		return
	}
	id := parts[0]

	// Helper to safely get part
	get := func(i int) string {
		if i < len(parts) { return parts[i] }
		return ""
	}
	action := get(1)
	sub    := get(2)
	subsub := get(3)

	// ── Torrent sub-routes (need hash + action) ──────────────────────────────
	if action == "torrents" {
		switch sub {
		case "bulk":
			// POST /api/servers/<id>/torrents/bulk/<resume|pause>
			r.handleBulkTorrent(w, req, id, subsub)
			return
		case "add":
			// POST /api/servers/<id>/torrents/add
			r.handleAddTorrent(w, req, id)
			return
		case "":
			// GET /api/servers/<id>/torrents
			s, ok := r.store.Get(id)
			if !ok { errJSON(w, 404, "server not found"); return }
			r.handleListTorrents(w, s)
			return
		default:
			// /api/servers/<id>/torrents/<hash>/<action>
			hash   := sub
			tAction := subsub
			if tAction == "" {
				// DELETE /api/servers/<id>/torrents/<hash>
				tAction = "delete"
			}
			r.handleTorrentAction(w, req, id, hash, tAction)
			return
		}
	}

	// ── Server-level routes ──────────────────────────────────────────────────
	s, ok := r.store.Get(id)
	if !ok {
		errJSON(w, 404, "server not found")
		return
	}

	switch {
	case req.Method == http.MethodDelete && action == "":
		r.store.Remove(id)
		r.store.Save()
		writeJSON(w, 200, map[string]string{"message": "Usunięto: " + s.Name})

	case req.Method == http.MethodPatch && action == "":
		r.handleEditServer(w, req, s)

	case action == "status":
		r.handleServerStatus(w, s)

	case action == "stats":
		r.handleStats(w, id)

	case action == "categories":
		torrents, err := qbit.Torrents(s)
		if err != nil { errJSON(w, 503, err.Error()); return }
		seen := map[string]bool{}
		var cats []string
		for _, t := range torrents {
			if t.Category != "" && !seen[t.Category] {
				seen[t.Category] = true
				cats = append(cats, t.Category)
			}
		}
		writeJSON(w, 200, cats)

	case action == "resume_all":
		qbit.ResumeAll(s)
		writeJSON(w, 200, map[string]string{"message": "Wznowiono wszystkie"})

	case action == "pause_all":
		qbit.PauseAll(s)
		writeJSON(w, 200, map[string]string{"message": "Pauza wszystkich"})

	case action == "restart":
		r.handleRestart(w, s)

	case action == "shutdown":
		qbit.Shutdown(s)
		s.Online = false
		writeJSON(w, 200, map[string]string{"message": "Wyłączono"})

	case action == "test":
		ok2 := qbit.Login(s)
		writeJSON(w, 200, map[string]interface{}{"online": ok2, "timestamp": time.Now().Format(time.RFC3339)})

	case action == "test_ssh":
		ok2, out := sshpkg.Test(s)
		writeJSON(w, 200, map[string]interface{}{"ok": ok2, "output": out})

	case action == "speed_limits":
		var body struct {
			DLLimit int64 `json:"dl_limit"`
			ULLimit int64 `json:"up_limit"`
		}
		readJSON(req, &body)
		prefs := fmt.Sprintf(`{"dl_limit":%d,"up_limit":%d}`, body.DLLimit, body.ULLimit)
		qbit.Post(s, "app/setPreferences", url.Values{"json": {prefs}})
		writeJSON(w, 200, map[string]string{"message": "Limity zaktualizowane"})

	case action == "auto_restart" && sub == "reset":
		s.AutoRestartCount = 0
		s.LastAutoRestart  = time.Time{}
		s.OfflineSince     = time.Time{}
		writeJSON(w, 200, map[string]string{"message": "Licznik zresetowany"})

	case action == "auto_restart":
		nextIn := scheduler.NextRestartIn(s)
		offlineSec := int64(0)
		if !s.OfflineSince.IsZero() {
			offlineSec = int64(time.Since(s.OfflineSince).Seconds())
		}
		writeJSON(w, 200, map[string]interface{}{
			"auto_restart":          s.AutoRestart,
			"auto_restart_interval": s.AutoRestartInterval,
			"auto_restart_count":    s.AutoRestartCount,
			"online":                s.Online,
			"offline_seconds":       offlineSec,
			"next_restart_in_s":     nextIn,
			"last_auto_restart":     s.LastAutoRestart,
		})

	default:
		errJSON(w, 404, fmt.Sprintf("unknown endpoint: %s %s", req.Method, req.URL.Path))
	}
}

func (r *Router) handleEditServer(w http.ResponseWriter, req *http.Request, s *models.QBitServer) {
	var body map[string]interface{}
	if err := readJSON(req, &body); err != nil {
		errJSON(w, 400, "invalid JSON")
		return
	}
	s.Mu.Lock()
	if v, ok := body["name"].(string); ok { s.Name = v }
	if v, ok := body["host"].(string); ok { s.Host = v }
	if v, ok := body["port"].(float64); ok { s.Port = int(v) }
	if v, ok := body["username"].(string); ok { s.Username = v }
	if v, ok := body["password"].(string); ok && v != "" { s.Password = v }
	if v, ok := body["https"].(bool); ok { s.HTTPS = v }
	if v, ok := body["restart_type"].(string); ok { s.RestartType = v }
	if v, ok := body["restart_unit"].(string); ok { s.RestartUnit = v }
	if v, ok := body["ssh_user"].(string); ok { s.SSHUser = v }
	if v, ok := body["ssh_port"].(float64); ok { s.SSHPort = int(v) }
	if v, ok := body["ssh_key_path"].(string); ok { s.SSHKeyPath = v }
	if v, ok := body["auto_restart"].(bool); ok { s.AutoRestart = v }
	if v, ok := body["auto_restart_interval"].(float64); ok && int(v) > 0 { s.AutoRestartInterval = int(v) }
	s.Mu.Unlock()

	s.Cookie = "" // force re-login
	qbit.Login(s)
	r.store.Save()
	writeJSON(w, 200, map[string]interface{}{
		"server":   s.ToDTO(),
		"login_ok": s.Online,
	})
}

func (r *Router) handleServerStatus(w http.ResponseWriter, s *models.QBitServer) {
	ver, err := qbit.Version(s)
	if err != nil {
		errJSON(w, 503, err.Error())
		return
	}
	s.Version = ver
	tf, err := qbit.Transfer(s)
	if err != nil {
		errJSON(w, 503, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"server":   s.ToDTO(),
		"version":  ver,
		"transfer": tf,
	})
}

func (r *Router) handleListTorrents(w http.ResponseWriter, s *models.QBitServer) {
	torrents, err := qbit.Torrents(s)
	if err != nil {
		errJSON(w, 503, err.Error())
		return
	}
	// Enrich with formatted speed strings for frontend
	type torrentOut struct {
		qbit.Torrent
		DLSpeedFmt string `json:"dl_speed_fmt"`
		ULSpeedFmt string `json:"ul_speed_fmt"`
		ETAFmt     string `json:"eta_fmt"`
	}
	out := make([]torrentOut, len(torrents))
	for i, t := range torrents {
		out[i] = torrentOut{
			Torrent:    t,
			DLSpeedFmt: fmtSpeed(t.DLSpeed),
			ULSpeedFmt: fmtSpeed(t.ULSpeed),
			ETAFmt:     fmtETA(t.ETA),
		}
	}
	writeJSON(w, 200, out)
}

func (r *Router) handleTorrentAction(w http.ResponseWriter, req *http.Request, sid, hash, action string) {
	s, ok := r.store.Get(sid)
	if !ok {
		errJSON(w, 404, "server not found")
		return
	}
	switch action {
	case "pause":
		qbit.PauseTorrent(s, hash)
		writeJSON(w, 200, map[string]string{"message": "Zatrzymano"})
	case "resume":
		qbit.ResumeTorrent(s, hash)
		writeJSON(w, 200, map[string]string{"message": "Wznowiono"})
	case "recheck":
		qbit.Post(s, "torrents/recheck", url.Values{"hashes": {hash}})
		writeJSON(w, 200, map[string]string{"message": "Recheck zlecony"})
	case "add_tags":
		var body struct{ Tags string `json:"tags"` }
		readJSON(req, &body)
		qbit.Post(s, "torrents/addTags", url.Values{"hashes": {hash}, "tags": {body.Tags}})
		writeJSON(w, 200, map[string]string{"message": "Tagi dodane"})
	case "remove_tags":
		var body struct{ Tags string `json:"tags"` }
		readJSON(req, &body)
		qbit.Post(s, "torrents/removeTags", url.Values{"hashes": {hash}, "tags": {body.Tags}})
		writeJSON(w, 200, map[string]string{"message": "Tagi usunięte"})
	case "set_location":
		var body struct{ Location string `json:"location"` }
		readJSON(req, &body)
		qbit.Post(s, "torrents/setLocation", url.Values{"hashes": {hash}, "location": {body.Location}})
		writeJSON(w, 200, map[string]string{"message": "Lokalizacja zmieniona"})
	case "files":
		files, err := qbit.TorrentFiles(s, hash)
		if err != nil {
			errJSON(w, 503, err.Error())
			return
		}
		writeJSON(w, 200, files)
	case "category":
		var body struct{ Category string `json:"category"` }
		readJSON(req, &body)
		qbit.SetCategory(s, hash, body.Category)
		writeJSON(w, 200, map[string]string{"message": "OK"})
	case "speed_limits":
		var body struct {
			DLLimit int64 `json:"dl_limit"`
			ULLimit int64 `json:"up_limit"`
		}
		readJSON(req, &body)
		qbit.SetDownloadLimit(s, hash, body.DLLimit)
		qbit.SetUploadLimit(s, hash, body.ULLimit)
		writeJSON(w, 200, map[string]string{"message": "OK"})
	default:
		// DELETE = delete torrent
		if req.Method == "DELETE" {
			delFiles := req.URL.Query().Get("delete_files") == "true"
			qbit.DeleteTorrent(s, hash, delFiles)
			writeJSON(w, 200, map[string]string{"message": "Usunięto"})
			return
		}
		errJSON(w, 404, "unknown action")
	}
}

func (r *Router) handleBulkTorrent(w http.ResponseWriter, req *http.Request, sid, action string) {
	s, ok := r.store.Get(sid)
	if !ok {
		errJSON(w, 404, "server not found")
		return
	}
	var body struct{ Hashes []string `json:"hashes"` }
	readJSON(req, &body)
	switch action {
	case "resume":
		qbit.BulkResume(s, body.Hashes)
	case "pause":
		qbit.BulkPause(s, body.Hashes)
	default:
		errJSON(w, 404, "unknown bulk action: "+action)
		return
	}
	writeJSON(w, 200, map[string]string{"message": "OK"})
}

func (r *Router) handleAddTorrent(w http.ResponseWriter, req *http.Request, sid string) {
	s, ok := r.store.Get(sid)
	if !ok {
		errJSON(w, 404, "server not found")
		return
	}

	contentType := req.Header.Get("Content-Type")

	// ── Multipart: .torrent file upload ──────────────────────────────────────
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			errJSON(w, 400, "błąd parsowania multipart: "+err.Error())
			return
		}
		file, header, err := req.FormFile("torrents")
		if err != nil {
			errJSON(w, 400, "brak pliku torrents: "+err.Error())
			return
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			errJSON(w, 500, "błąd odczytu pliku")
			return
		}

		category := req.FormValue("category")
		savePath  := req.FormValue("savepath")
		paused    := req.FormValue("paused") == "true"

		if err := qbit.AddTorrentFile(s, data, header.Filename, category, savePath, paused); err != nil {
			errJSON(w, 503, err.Error())
			return
		}
		writeJSON(w, 201, map[string]string{"message": "Dodano plik .torrent"})
		return
	}

	// ── JSON: magnet/URL lub base64 ──────────────────────────────────────────
	var body struct {
		Magnet   string `json:"magnet"`
		File     string `json:"file"`     // base64 encoded .torrent
		Filename string `json:"filename"`
		Category string `json:"category"`
		SavePath string `json:"savepath"`
		Paused   bool   `json:"paused"`
	}
	if err := readJSON(req, &body); err != nil {
		errJSON(w, 400, "invalid JSON")
		return
	}

	if body.Magnet != "" {
		if err := qbit.AddMagnet(s, body.Magnet, body.Category, body.SavePath, body.Paused); err != nil {
			errJSON(w, 503, err.Error())
			return
		}
		writeJSON(w, 201, map[string]string{"message": "Dodano magnet"})
		return
	}

	if body.File != "" {
		data, err := base64.StdEncoding.DecodeString(body.File)
		if err != nil {
			errJSON(w, 400, "błąd dekodowania base64")
			return
		}
		fname := body.Filename
		if fname == "" {
			fname = "torrent.torrent"
		}
		if err := qbit.AddTorrentFile(s, data, fname, body.Category, body.SavePath, body.Paused); err != nil {
			errJSON(w, 503, err.Error())
			return
		}
		writeJSON(w, 201, map[string]string{"message": "Dodano plik .torrent"})
		return
	}

	errJSON(w, 400, "podaj magnet lub plik .torrent")
}

func (r *Router) handleRestart(w http.ResponseWriter, s *models.QBitServer) {
	if s.RestartType == "none" {
		writeJSON(w, 200, map[string]interface{}{
			"restart_method": "none",
			"detail":         "Edytuj serwer i skonfiguruj SSH + Docker/systemd",
			"success":        false,
		})
		return
	}
	result := sshpkg.RestartWithShutdown(s, func() error {
		return qbit.Shutdown(s)
	})
	success := strings.Contains(result, "OK")

	// Background reconnect
	go func() {
		for i := 0; i < 15; i++ {
			time.Sleep(5 * time.Second)
			if qbit.Login(s) {
				return
			}
		}
	}()

	writeJSON(w, 200, map[string]interface{}{
		"restart_method": s.RestartType,
		"detail":         result,
		"success":        success,
	})
}

// ── Health ────────────────────────────────────────────────────────────────────

func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	servers := r.store.GetAll()
	online := 0
	for _, s := range servers {
		if s.Online {
			online++
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"status":         "ok",
		"servers_total":  len(servers),
		"servers_online": online,
		"sse_clients":    r.hub.ClientCount(),
		"time":           time.Now().Format(time.RFC3339),
	})
}

// ── Overview ─────────────────────────────────────────────────────────────────

func (r *Router) handleOverview(w http.ResponseWriter, req *http.Request) {
	servers := r.store.GetAll()
	result := make([]map[string]interface{}, 0, len(servers))
	for _, s := range servers {
		entry := map[string]interface{}{
			"id":      s.ID,
			"name":    s.Name,
			"online":  s.Online,
			"version": s.Version,
		}
		if s.Online {
			if tf, err := qbit.Transfer(s); err == nil {
				entry["dl_speed"]     = tf.DLSpeed
				entry["ul_speed"]     = tf.ULSpeed
				entry["dl_speed_fmt"] = tf.DLSpeedFmt
				entry["ul_speed_fmt"] = tf.ULSpeedFmt
			}
			if torrents, err := qbit.Torrents(s); err == nil {
				downloading, seeding, errors := 0, 0, 0
				for _, t := range torrents {
					switch t.State {
					case "downloading", "forcedDL", "metaDL":
						downloading++
					case "uploading", "stalledUP", "forcedUP":
						seeding++
					case "error", "missingFiles":
						errors++
					}
				}
				entry["total"]       = len(torrents)
				entry["downloading"] = downloading
				entry["seeding"]     = seeding
				entry["errors"]      = errors
			}
			// Speed history from stats collector
			latest := r.stats.Latest(s.ID)
			if !latest.Time.IsZero() {
				entry["stats_dl"] = latest.DLSpeed
				entry["stats_ul"] = latest.ULSpeed
			}
		}
		result = append(result, entry)
	}
	writeJSON(w, 200, result)
}

// ── Ping ──────────────────────────────────────────────────────────────────────

func (r *Router) handlePing(w http.ResponseWriter, req *http.Request) {
	host := req.URL.Query().Get("host")
	port := req.URL.Query().Get("port")
	if port == "" {
		port = "8080"
	}
	schema := "http"
	if req.URL.Query().Get("https") == "true" {
		schema = "https"
	}
	u := fmt.Sprintf("%s://%s:%s/api/v2/app/version", schema, host, port)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	writeJSON(w, 200, map[string]interface{}{
		"ok":      true,
		"status":  resp.StatusCode,
		"version": strings.TrimSpace(string(body)),
	})
}

// ── Stats ─────────────────────────────────────────────────────────────────────

func (r *Router) handleStats(w http.ResponseWriter, sid string) {
	samples := r.stats.Get(sid)
	if samples == nil {
		writeJSON(w, 200, []interface{}{})
		return
	}
	writeJSON(w, 200, samples)
}

func (r *Router) handleARRConfig(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		arr := r.store.GetARR()
		masked := func(c models.ARRConfig) map[string]string {
			key := c.Key
			if len(key) > 6 {
				key = key[:6] + "…"
			}
			return map[string]string{"url": c.URL, "key": key}
		}
		writeJSON(w, 200, map[string]interface{}{
			"radarr": masked(arr.Radarr),
			"sonarr": masked(arr.Sonarr),
		})

	case http.MethodPost:
		// JS wysyła: { radarr: {url, key}, sonarr: {url, key} }
		var body struct {
			Radarr *models.ARRConfig `json:"radarr"`
			Sonarr *models.ARRConfig `json:"sonarr"`
		}
		if err := readJSON(req, &body); err != nil {
			errJSON(w, 400, "invalid JSON: "+err.Error())
			return
		}
		arr := r.store.GetARR()
		changed := false
		if body.Radarr != nil && body.Radarr.URL != "" && body.Radarr.Key != "" {
			arr.Radarr = *body.Radarr
			arr.Radarr.URL = strings.TrimRight(arr.Radarr.URL, "/")
			changed = true
		}
		if body.Sonarr != nil && body.Sonarr.URL != "" && body.Sonarr.Key != "" {
			arr.Sonarr = *body.Sonarr
			arr.Sonarr.URL = strings.TrimRight(arr.Sonarr.URL, "/")
			changed = true
		}
		if changed {
			r.store.SetARR(arr)
		}
		writeJSON(w, 200, map[string]interface{}{
			"ok":     true,
			"radarr": arr.Radarr.URL != "",
			"sonarr": arr.Sonarr.URL != "",
		})

	default:
		w.WriteHeader(405)
	}
}

func (r *Router) handleARRConfigFull(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		writeJSON(w, 200, r.store.GetARR())

	case http.MethodPost:
		var body struct {
			Radarr *models.ARRConfig `json:"radarr"`
			Sonarr *models.ARRConfig `json:"sonarr"`
		}
		if err := readJSON(req, &body); err != nil {
			errJSON(w, 400, "invalid JSON")
			return
		}
		arr := r.store.GetARR()
		if body.Radarr != nil && body.Radarr.URL != "" && body.Radarr.Key != "" {
			arr.Radarr = *body.Radarr
			arr.Radarr.URL = strings.TrimRight(arr.Radarr.URL, "/")
		}
		if body.Sonarr != nil && body.Sonarr.URL != "" && body.Sonarr.Key != "" {
			arr.Sonarr = *body.Sonarr
			arr.Sonarr.URL = strings.TrimRight(arr.Sonarr.URL, "/")
		}
		r.store.SetARR(arr)
		writeJSON(w, 200, map[string]bool{"ok": true})

	default:
		w.WriteHeader(405)
	}
}

// ARR proxy helpers — forward to Radarr/Sonarr API
func arrGet(apiURL, key, path string, params url.Values) ([]byte, error) {
	u := strings.TrimRight(apiURL, "/") + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("X-Api-Key", key)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (r *Router) handleRadarrTest(w http.ResponseWriter, req *http.Request) {
	u := req.URL.Query().Get("url")
	k := req.URL.Query().Get("key")
	if u == "" {
		u = r.store.GetARR().Radarr.URL
		k = r.store.GetARR().Radarr.Key
	}
	b, err := arrGet(u, k, "/api/v3/system/status", nil)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	var s map[string]interface{}
	json.Unmarshal(b, &s)
	writeJSON(w, 200, map[string]interface{}{"ok": true, "version": s["version"]})
}

func (r *Router) handleSonarrTest(w http.ResponseWriter, req *http.Request) {
	u := req.URL.Query().Get("url")
	k := req.URL.Query().Get("key")
	if u == "" {
		u = r.store.GetARR().Sonarr.URL
		k = r.store.GetARR().Sonarr.Key
	}
	b, err := arrGet(u, k, "/api/v3/system/status", nil)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	var s map[string]interface{}
	json.Unmarshal(b, &s)
	writeJSON(w, 200, map[string]interface{}{"ok": true, "version": s["version"]})
}

func (r *Router) handleRadarrLookup(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	u, k := q.Get("url"), q.Get("key")
	if u == "" {
		u = r.store.GetARR().Radarr.URL
		k = r.store.GetARR().Radarr.Key
	}
	title := q.Get("title")
	b, err := arrGet(u, k, "/api/v3/movie/lookup", url.Values{"term": {title}})
	if err != nil {
		writeJSON(w, 200, map[string]string{"error": err.Error()})
		return
	}
	var results []map[string]interface{}
	if err := json.Unmarshal(b, &results); err != nil || len(results) == 0 {
		writeJSON(w, 200, map[string]string{"error": "Nie znaleziono"})
		return
	}
	// Pobierz liste filmów z biblioteki żeby sprawdzić status
	var libMovies []map[string]interface{}
	if lb, err2 := arrGet(u, k, "/api/v3/movie", nil); err2 == nil {
		json.Unmarshal(lb, &libMovies)
	}
	libByTmdb := map[string]map[string]interface{}{}
	for _, m := range libMovies {
		if tid, ok := m["tmdbId"]; ok {
			libByTmdb[fmt.Sprintf("%v", tid)] = m
		}
	}
	// Wzbogać top 5 wyników
	out := []map[string]interface{}{}
	for i, mv := range results {
		if i >= 5 { break }
		tmdbID := fmt.Sprintf("%v", mv["tmdbId"])
		if lib, found := libByTmdb[tmdbID]; found {
			mv["in_radarr"] = true
			if hf, ok := lib["hasFile"].(bool); ok { mv["has_file"] = hf }
			if mid, ok := lib["id"]; ok { mv["id"] = mid }
		} else {
			mv["in_radarr"] = false
			mv["has_file"] = false
		}
		// Extract poster
		if imgs, ok := mv["images"].([]interface{}); ok {
			for _, img := range imgs {
				if im, ok := img.(map[string]interface{}); ok {
					if im["coverType"] == "poster" {
						mv["poster"] = im["remoteUrl"]
						break
					}
				}
			}
		}
		out = append(out, mv)
	}
	writeJSON(w, 200, out)
}

func (r *Router) handleSonarrLookup(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	u, k := q.Get("url"), q.Get("key")
	if u == "" {
		u = r.store.GetARR().Sonarr.URL
		k = r.store.GetARR().Sonarr.Key
	}
	title := q.Get("title")
	b, err := arrGet(u, k, "/api/v3/series/lookup", url.Values{"term": {title}})
	if err != nil {
		writeJSON(w, 200, map[string]string{"error": err.Error()})
		return
	}
	var results []map[string]interface{}
	if err := json.Unmarshal(b, &results); err != nil || len(results) == 0 {
		writeJSON(w, 200, map[string]string{"error": "Nie znaleziono"})
		return
	}
	// Pobierz całą bibliotekę Sonarr
	var libSeries []map[string]interface{}
	if lb, err2 := arrGet(u, k, "/api/v3/series", nil); err2 == nil {
		json.Unmarshal(lb, &libSeries)
	}
	libByTvdb := map[string]map[string]interface{}{}
	for _, s := range libSeries {
		if tid, ok := s["tvdbId"]; ok {
			libByTvdb[fmt.Sprintf("%v", tid)] = s
		}
	}
	out := []map[string]interface{}{}
	for i, sv := range results {
		if i >= 5 { break }
		tvdbID := fmt.Sprintf("%v", sv["tvdbId"])
		if lib, found := libByTvdb[tvdbID]; found {
			sv["in_sonarr"] = true
			if sid, ok := lib["id"]; ok { sv["id"] = sid }
			sv["has_file"] = false
			if stats, ok := lib["statistics"].(map[string]interface{}); ok {
				if pct, ok := stats["percentOfEpisodes"].(float64); ok {
					sv["has_file"] = pct > 0
				}
			}
		} else {
			sv["in_sonarr"] = false
			sv["has_file"] = false
		}
		if imgs, ok := sv["images"].([]interface{}); ok {
			for _, img := range imgs {
				if im, ok := img.(map[string]interface{}); ok {
					if im["coverType"] == "poster" {
						sv["poster"] = im["remoteUrl"]
						break
					}
				}
			}
		}
		out = append(out, sv)
	}
	writeJSON(w, 200, out)
}

func (r *Router) handleRadarrQueue(w http.ResponseWriter, req *http.Request) {
	arr := r.store.GetARR()
	u, k := req.URL.Query().Get("url"), req.URL.Query().Get("key")
	if u == "" { u = arr.Radarr.URL; k = arr.Radarr.Key }
	b, err := arrGet(u, k, "/api/v3/queue", url.Values{"pageSize": {"50"}})
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"records": []interface{}{}})
		return
	}
	var result interface{}
	json.Unmarshal(b, &result)
	writeJSON(w, 200, result)
}

func (r *Router) handleSonarrQueue(w http.ResponseWriter, req *http.Request) {
	arr := r.store.GetARR()
	u, k := req.URL.Query().Get("url"), req.URL.Query().Get("key")
	if u == "" { u = arr.Sonarr.URL; k = arr.Sonarr.Key }
	b, err := arrGet(u, k, "/api/v3/queue", url.Values{"pageSize": {"50"}})
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"records": []interface{}{}})
		return
	}
	var result interface{}
	json.Unmarshal(b, &result)
	writeJSON(w, 200, result)
}

func (r *Router) handleArrSearch(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		errJSON(w, 405, "POST wymagany")
		return
	}
	arrType := "radarr"
	if strings.Contains(req.URL.Path, "sonarr") {
		arrType = "sonarr"
	}

	var body struct {
		URL      string `json:"url"`
		Key      string `json:"key"`
		TmdbID   int    `json:"tmdb_id"`
		TvdbID   int    `json:"tvdb_id"`
		MovieID  int    `json:"movie_id"`
		SeriesID int    `json:"series_id"`
	}
	if err := readJSON(req, &body); err != nil {
		errJSON(w, 400, "invalid JSON")
		return
	}

	arr := r.store.GetARR()
	apiURL := body.URL
	apiKey := body.Key
	if apiURL == "" {
		if arrType == "radarr" {
			apiURL = arr.Radarr.URL
			apiKey = arr.Radarr.Key
		} else {
			apiURL = arr.Sonarr.URL
			apiKey = arr.Sonarr.Key
		}
	}
	if apiURL == "" || apiKey == "" {
		errJSON(w, 400, "brak konfiguracji "+arrType)
		return
	}

	// Build command payload
	var cmdPayload map[string]interface{}
	if arrType == "radarr" {
		movieID := body.MovieID
		// If only tmdb_id given, look up internal id first
		if movieID == 0 && body.TmdbID != 0 {
			b, err := arrGet(apiURL, apiKey, "/api/v3/movie", url.Values{
				"tmdbId": {fmt.Sprintf("%d", body.TmdbID)},
			})
			if err == nil {
				var movies []map[string]interface{}
				if json.Unmarshal(b, &movies) == nil && len(movies) > 0 {
					if id, ok := movies[0]["id"].(float64); ok {
						movieID = int(id)
					}
				}
			}
		}
		if movieID == 0 {
			errJSON(w, 400, "film nie jest w bibliotece Radarr — najpierw go dodaj")
			return
		}
		cmdPayload = map[string]interface{}{
			"name":     "MoviesSearch",
			"movieIds": []int{movieID},
		}
	} else {
		seriesID := body.SeriesID
		if seriesID == 0 && body.TvdbID != 0 {
			b, err := arrGet(apiURL, apiKey, "/api/v3/series", url.Values{
				"tvdbId": {fmt.Sprintf("%d", body.TvdbID)},
			})
			if err == nil {
				var series []map[string]interface{}
				if json.Unmarshal(b, &series) == nil && len(series) > 0 {
					if id, ok := series[0]["id"].(float64); ok {
						seriesID = int(id)
					}
				}
			}
		}
		if seriesID == 0 {
			errJSON(w, 400, "serial nie jest w bibliotece Sonarr — najpierw go dodaj")
			return
		}
		cmdPayload = map[string]interface{}{
			"name":     "SeriesSearch",
			"seriesId": seriesID,
		}
	}

	// POST command to ARR
	cmdData, _ := json.Marshal(cmdPayload)
	httpReq, _ := http.NewRequest("POST",
		strings.TrimRight(apiURL, "/")+"/api/v3/command",
		bytes.NewReader(cmdData),
	)
	httpReq.Header.Set("X-Api-Key", apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		errJSON(w, 503, err.Error())
		return
	}
	defer resp.Body.Close()
	respData, _ := io.ReadAll(resp.Body)

	var cmdResult interface{}
	json.Unmarshal(respData, &cmdResult)
	writeJSON(w, 200, map[string]interface{}{
		"ok":      resp.StatusCode < 300,
		"command": cmdResult,
	})
}

func (r *Router) handleArrAdd(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		errJSON(w, 405, "POST wymagany")
		return
	}
	arrType := "radarr"
	if strings.Contains(req.URL.Path, "sonarr") {
		arrType = "sonarr"
	}

	var body struct {
		URL    string `json:"url"`
		Key    string `json:"key"`
		TmdbID int    `json:"tmdb_id"`
		TvdbID int    `json:"tvdb_id"`
	}
	if err := readJSON(req, &body); err != nil {
		errJSON(w, 400, "invalid JSON")
		return
	}

	arr := r.store.GetARR()
	apiURL := body.URL
	apiKey := body.Key
	if apiURL == "" {
		if arrType == "radarr" {
			apiURL = arr.Radarr.URL
			apiKey = arr.Radarr.Key
		} else {
			apiURL = arr.Sonarr.URL
			apiKey = arr.Sonarr.Key
		}
	}
	if apiURL == "" || apiKey == "" {
		errJSON(w, 400, "brak konfiguracji "+arrType)
		return
	}

	// Get root folder and quality profile
	rootFolders, err := arrGet(apiURL, apiKey, "/api/v3/rootfolder", nil)
	if err != nil {
		errJSON(w, 503, "błąd pobierania root folders: "+err.Error())
		return
	}
	var folders []map[string]interface{}
	json.Unmarshal(rootFolders, &folders)
	if len(folders) == 0 {
		errJSON(w, 400, "brak root folders w "+arrType)
		return
	}
	rootPath, _ := folders[0]["path"].(string)

	profiles, err := arrGet(apiURL, apiKey, "/api/v3/qualityprofile", nil)
	if err != nil {
		errJSON(w, 503, "błąd pobierania profili: "+err.Error())
		return
	}
	var profileList []map[string]interface{}
	json.Unmarshal(profiles, &profileList)
	if len(profileList) == 0 {
		errJSON(w, 400, "brak profili jakości w "+arrType)
		return
	}
	profileID := int(profileList[0]["id"].(float64))

	var addPayload map[string]interface{}
	if arrType == "radarr" {
		if body.TmdbID == 0 {
			errJSON(w, 400, "tmdb_id wymagany")
			return
		}
		// Fetch movie details from lookup
		lb, _ := arrGet(apiURL, apiKey, "/api/v3/movie/lookup/tmdb",
			url.Values{"tmdbId": {fmt.Sprintf("%d", body.TmdbID)}})
		var mv map[string]interface{}
		json.Unmarshal(lb, &mv)
		if mv == nil {
			errJSON(w, 404, "film nie znaleziony")
			return
		}
		mv["rootFolderPath"] = rootPath
		mv["qualityProfileId"] = profileID
		mv["monitored"] = true
		mv["addOptions"] = map[string]interface{}{
			"searchForMovie": true,
		}
		addPayload = mv
	} else {
		if body.TvdbID == 0 {
			errJSON(w, 400, "tvdb_id wymagany")
			return
		}
		lb, _ := arrGet(apiURL, apiKey, "/api/v3/series/lookup",
			url.Values{"term": {fmt.Sprintf("tvdb:%d", body.TvdbID)}})
		var results []map[string]interface{}
		json.Unmarshal(lb, &results)
		if len(results) == 0 {
			errJSON(w, 404, "serial nie znaleziony")
			return
		}
		sv := results[0]
		sv["rootFolderPath"] = rootPath
		sv["qualityProfileId"] = profileID
		sv["monitored"] = true
		sv["languageProfileId"] = 1
		sv["addOptions"] = map[string]interface{}{
			"searchForMissingEpisodes": true,
		}
		addPayload = sv
	}

	// POST to /api/v3/movie or /api/v3/series
	endpoint := "/api/v3/movie"
	if arrType == "sonarr" {
		endpoint = "/api/v3/series"
	}
	addData, _ := json.Marshal(addPayload)
	httpReq, _ := http.NewRequest("POST",
		strings.TrimRight(apiURL, "/")+endpoint,
		bytes.NewReader(addData),
	)
	httpReq.Header.Set("X-Api-Key", apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		errJSON(w, 503, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body2, _ := io.ReadAll(resp.Body)
		errJSON(w, 400, fmt.Sprintf("błąd %s: %s", arrType, string(body2)[:min(200, len(body2))]))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// SSH test route is handled in handleServerDetail via action == "test_ssh"
// This is a standalone handler for /api/servers/<id>/test_ssh via GET


// handleRadarrRecent – ostatnio dodane filmy (max 10)
func (r *Router) handleRadarrRecent(w http.ResponseWriter, req *http.Request) {
	arr := r.store.GetARR()
	u, k := arr.Radarr.URL, arr.Radarr.Key
	if u == "" { writeJSON(w, 200, []interface{}{}); return }
	b, err := arrGet(u, k, "/api/v3/movie", url.Values{"sortKey": {"added"}, "sortDir": {"desc"}})
	if err != nil { writeJSON(w, 200, []interface{}{}); return }
	var movies []map[string]interface{}
	if err := json.Unmarshal(b, &movies); err != nil { writeJSON(w, 200, []interface{}{}); return }
	out := []map[string]interface{}{}
	for i, mv := range movies {
		if i >= 10 { break }
		item := map[string]interface{}{
			"title":    mv["title"],
			"year":     mv["year"],
			"hasFile":  mv["hasFile"],
			"status":   mv["status"],
			"added":    mv["added"],
			"monitored":mv["monitored"],
		}
		if mfData, ok := mv["movieFile"].(map[string]interface{}); ok {
			item["quality"] = mfData["quality"]
			item["size"] = mfData["size"]
		}
		if imgs, ok := mv["images"].([]interface{}); ok {
			for _, img := range imgs {
				if im, ok := img.(map[string]interface{}); ok && im["coverType"] == "poster" {
					item["poster"] = im["remoteUrl"]
					break
				}
			}
		}
		out = append(out, item)
	}
	writeJSON(w, 200, out)
}

// handleSonarrRecent – ostatnio dodane seriale (max 10)
func (r *Router) handleSonarrRecent(w http.ResponseWriter, req *http.Request) {
	arr := r.store.GetARR()
	u, k := arr.Sonarr.URL, arr.Sonarr.Key
	if u == "" { writeJSON(w, 200, []interface{}{}); return }
	b, err := arrGet(u, k, "/api/v3/series", url.Values{"sortKey": {"added"}, "sortDir": {"desc"}})
	if err != nil { writeJSON(w, 200, []interface{}{}); return }
	var series []map[string]interface{}
	if err := json.Unmarshal(b, &series); err != nil { writeJSON(w, 200, []interface{}{}); return }
	out := []map[string]interface{}{}
	for i, sv := range series {
		if i >= 10 { break }
		item := map[string]interface{}{
			"title":    sv["title"],
			"year":     sv["year"],
			"status":   sv["status"],
			"added":    sv["added"],
			"monitored":sv["monitored"],
		}
		if svStats, ok := sv["statistics"].(map[string]interface{}); ok {
			item["episodeCount"]   = svStats["totalEpisodeCount"]
			item["episodeHave"]    = svStats["episodeCount"]
			item["sizeOnDisk"]     = svStats["sizeOnDisk"]
			item["percentOfEpisodes"] = svStats["percentOfEpisodes"]
		}
		if imgs, ok := sv["images"].([]interface{}); ok {
			for _, img := range imgs {
				if im, ok := img.(map[string]interface{}); ok && im["coverType"] == "poster" {
					item["poster"] = im["remoteUrl"]
					break
				}
			}
		}
		out = append(out, item)
	}
	writeJSON(w, 200, out)
}

// handleRadarrLibrary – statystyki biblioteki Radarr
func (r *Router) handleRadarrLibrary(w http.ResponseWriter, req *http.Request) {
	arr := r.store.GetARR()
	u, k := arr.Radarr.URL, arr.Radarr.Key
	if u == "" { writeJSON(w, 200, map[string]interface{}{"error": "not configured"}); return }
	b, err := arrGet(u, k, "/api/v3/movie", nil)
	if err != nil { writeJSON(w, 200, map[string]interface{}{"error": err.Error()}); return }
	var movies []map[string]interface{}
	if err := json.Unmarshal(b, &movies); err != nil { writeJSON(w, 200, map[string]interface{}{"error": "parse error"}); return }
	total, withFile, monitored := 0, 0, 0
	var totalSize int64
	for _, m := range movies {
		total++
		if hf, ok := m["hasFile"].(bool); ok && hf { withFile++ }
		if mo, ok := m["monitored"].(bool); ok && mo { monitored++ }
		if mf, ok := m["movieFile"].(map[string]interface{}); ok {
			if sz, ok := mf["size"].(float64); ok { totalSize += int64(sz) }
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"total": total, "withFile": withFile,
		"monitored": monitored, "missing": monitored - withFile,
		"totalSize": totalSize,
	})
}

// handleSonarrLibrary – statystyki biblioteki Sonarr
func (r *Router) handleSonarrLibrary(w http.ResponseWriter, req *http.Request) {
	arr := r.store.GetARR()
	u, k := arr.Sonarr.URL, arr.Sonarr.Key
	if u == "" { writeJSON(w, 200, map[string]interface{}{"error": "not configured"}); return }
	b, err := arrGet(u, k, "/api/v3/series", nil)
	if err != nil { writeJSON(w, 200, map[string]interface{}{"error": err.Error()}); return }
	var series []map[string]interface{}
	if err := json.Unmarshal(b, &series); err != nil { writeJSON(w, 200, map[string]interface{}{"error": "parse error"}); return }
	total, monitored := 0, 0
	var totalEp, haveEp int64
	var totalSize int64
	for _, sv := range series {
		total++
		if mo, ok := sv["monitored"].(bool); ok && mo { monitored++ }
		if svStats, ok := sv["statistics"].(map[string]interface{}); ok {
			if n, ok := svStats["totalEpisodeCount"].(float64); ok { totalEp += int64(n) }
			if n, ok := svStats["episodeCount"].(float64); ok { haveEp += int64(n) }
			if n, ok := svStats["sizeOnDisk"].(float64); ok { totalSize += int64(n) }
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"total": total, "monitored": monitored,
		"totalEpisodes": totalEp, "haveEpisodes": haveEp,
		"totalSize": totalSize,
	})
}

// handleRadarrHealth – health messages z Radarr
func (r *Router) handleRadarrHealth(w http.ResponseWriter, req *http.Request) {
	arr := r.store.GetARR()
	u, k := arr.Radarr.URL, arr.Radarr.Key
	if u == "" { writeJSON(w, 200, []interface{}{}); return }
	b, err := arrGet(u, k, "/api/v3/health", nil)
	if err != nil { writeJSON(w, 200, map[string]interface{}{"error": err.Error()}); return }
	var result interface{}
	json.Unmarshal(b, &result)
	writeJSON(w, 200, result)
}

// handleSonarrHealth – health messages z Sonarr
func (r *Router) handleSonarrHealth(w http.ResponseWriter, req *http.Request) {
	arr := r.store.GetARR()
	u, k := arr.Sonarr.URL, arr.Sonarr.Key
	if u == "" { writeJSON(w, 200, []interface{}{}); return }
	b, err := arrGet(u, k, "/api/v3/health", nil)
	if err != nil { writeJSON(w, 200, map[string]interface{}{"error": err.Error()}); return }
	var result interface{}
	json.Unmarshal(b, &result)
	writeJSON(w, 200, result)
}

func fmtSpeed(bps int64) string {
	switch {
	case bps >= 1_048_576:
		return fmt.Sprintf("%.1f MB/s", float64(bps)/1_048_576)
	case bps >= 1024:
		return fmt.Sprintf("%.0f KB/s", float64(bps)/1024)
	case bps > 0:
		return fmt.Sprintf("%d B/s", bps)
	default:
		return "—"
	}
}

func fmtETA(s int64) string {
	if s <= 0 || s >= 8_640_000 {
		return "∞"
	}
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	if s < 3600 {
		return fmt.Sprintf("%dm", s/60)
	}
	return fmt.Sprintf("%dh %dm", s/3600, (s%3600)/60)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
