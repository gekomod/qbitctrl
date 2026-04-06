package models

import (
	"net/http"
	"sync"
	"time"
)

// QBitServer represents a qBittorrent instance
type QBitServer struct {
	Mu sync.RWMutex

	ID                  string `json:"id"`
	Name                string `json:"name"`
	Host                string `json:"host"`
	Port                int    `json:"port"`
	Username            string `json:"username"`
	Password            string `json:"password"`
	HTTPS               bool   `json:"https"`
	RestartType         string `json:"restart_type"`  // none | docker | systemd
	RestartUnit         string `json:"restart_unit"`  // unit name or container name
	SSHUser             string `json:"ssh_user"`
	SSHPort             int    `json:"ssh_port"`
	SSHKeyPath          string `json:"ssh_key_path"`
	AutoRestart         bool   `json:"auto_restart"`
	AutoRestartInterval int    `json:"auto_restart_interval"` // minutes

	// Runtime state (not persisted)
	Online            bool      `json:"online"`
	Version           string    `json:"version"`
	LastCheck         time.Time `json:"last_check"`
	OfflineSince      time.Time `json:"-"`
	LastAutoRestart   time.Time `json:"last_auto_restart"`
	AutoRestartCount  int       `json:"auto_restart_count"`
	Cookie            string    `json:"-"` // SID cookie
	client            *http.Client
}

func NewServer(id, name, host string, port int) *QBitServer {
	return &QBitServer{
		ID:                  id,
		Name:                name,
		Host:                host,
		Port:                port,
		Username:            "admin",
		SSHUser:             "root",
		SSHPort:             22,
		AutoRestartInterval: 60,
		RestartType:         "none",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *QBitServer) GetClient() *http.Client {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.client == nil {
		s.client = &http.Client{Timeout: 10 * time.Second}
	}
	return s.client
}

func (s *QBitServer) Schema() string {
	if s.HTTPS {
		return "https"
	}
	return "http"
}

func (s *QBitServer) BaseURL() string {
	return s.Schema() + "://" + s.Host + ":" + itoa(s.Port) + "/api/v2"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// ServerDTO is the API-safe representation (no password)
type ServerDTO struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Host                string    `json:"host"`
	Port                int       `json:"port"`
	HTTPS               bool      `json:"https"`
	Username            string    `json:"username"`
	RestartType         string    `json:"restart_type"`
	RestartUnit         string    `json:"restart_unit"`
	SSHUser             string    `json:"ssh_user"`
	SSHPort             int       `json:"ssh_port"`
	SSHKeyPath          string    `json:"ssh_key_path"`
	AutoRestart         bool      `json:"auto_restart"`
	AutoRestartInterval int       `json:"auto_restart_interval"`
	Online              bool      `json:"online"`
	Version             string    `json:"version"`
	LastCheck           time.Time `json:"last_check"`
	LastAutoRestart     time.Time `json:"last_auto_restart"`
	AutoRestartCount    int       `json:"auto_restart_count"`
}

func (s *QBitServer) ToDTO() ServerDTO {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return ServerDTO{
		ID:                  s.ID,
		Name:                s.Name,
		Host:                s.Host,
		Port:                s.Port,
		HTTPS:               s.HTTPS,
		Username:            s.Username,
		RestartType:         s.RestartType,
		RestartUnit:         s.RestartUnit,
		SSHUser:             s.SSHUser,
		SSHPort:             s.SSHPort,
		SSHKeyPath:          s.SSHKeyPath,
		AutoRestart:         s.AutoRestart,
		AutoRestartInterval: s.AutoRestartInterval,
		Online:              s.Online,
		Version:             s.Version,
		LastCheck:           s.LastCheck,
		LastAutoRestart:     s.LastAutoRestart,
		AutoRestartCount:    s.AutoRestartCount,
	}
}

// ARRConfig holds Radarr/Sonarr credentials
type ARRConfig struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

type ARRStore struct {
	Radarr ARRConfig `json:"radarr"`
	Sonarr ARRConfig `json:"sonarr"`
}
