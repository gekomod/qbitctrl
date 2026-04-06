package store

import (
	"encoding/json"
	"log"
	"os"
	"sync"

	"github.com/qbitctrl/internal/models"
)

type Store struct {
	mu      sync.RWMutex
	servers map[string]*models.QBitServer
	order   []string // preserve insertion order
	arr     models.ARRStore
	dbPath  string
	arrPath string
}

func New(dbPath, arrPath string) *Store {
	return &Store{
		servers: make(map[string]*models.QBitServer),
		dbPath:  dbPath,
		arrPath: arrPath,
	}
}

// ── Servers ──────────────────────────────────────────────────────────────────

func (st *Store) GetAll() []*models.QBitServer {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*models.QBitServer, 0, len(st.order))
	for _, id := range st.order {
		if s, ok := st.servers[id]; ok {
			out = append(out, s)
		}
	}
	return out
}

func (st *Store) Get(id string) (*models.QBitServer, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	s, ok := st.servers[id]
	return s, ok
}

func (st *Store) Add(s *models.QBitServer) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, exists := st.servers[s.ID]; !exists {
		st.order = append(st.order, s.ID)
	}
	st.servers[s.ID] = s
}

func (st *Store) Remove(id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.servers[id]; !ok {
		return false
	}
	delete(st.servers, id)
	for i, v := range st.order {
		if v == id {
			st.order = append(st.order[:i], st.order[i+1:]...)
			break
		}
	}
	return true
}

// ── ARR ──────────────────────────────────────────────────────────────────────

func (st *Store) GetARR() models.ARRStore {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.arr
}

func (st *Store) SetARR(a models.ARRStore) {
	st.mu.Lock()
	st.arr = a
	st.mu.Unlock()
	st.saveARR()
}

// ── Persistence ───────────────────────────────────────────────────────────────

type serverRecord struct {
	ID                  string `json:"id"`
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

func (st *Store) Load() error {
	// Load servers
	if data, err := os.ReadFile(st.dbPath); err == nil {
		var records []serverRecord
		if err := json.Unmarshal(data, &records); err != nil {
			log.Printf("WARN: parse %s: %v", st.dbPath, err)
		} else {
			for _, r := range records {
				s := models.NewServer(r.ID, r.Name, r.Host, r.Port)
				s.Username = r.Username
				s.Password = r.Password
				s.HTTPS = r.HTTPS
				s.RestartType = r.RestartType
				s.RestartUnit = r.RestartUnit
				s.SSHUser = r.SSHUser
				s.SSHPort = r.SSHPort
				s.SSHKeyPath = r.SSHKeyPath
				s.AutoRestart = r.AutoRestart
				s.AutoRestartInterval = r.AutoRestartInterval
				if s.SSHPort == 0 {
					s.SSHPort = 22
				}
				if s.AutoRestartInterval == 0 {
					s.AutoRestartInterval = 60
				}
				st.Add(s)
				log.Printf("Załadowano: %s (%s:%d)", s.Name, s.Host, s.Port)
			}
		}
	}
	// Load ARR
	if data, err := os.ReadFile(st.arrPath); err == nil {
		json.Unmarshal(data, &st.arr)
	}
	return nil
}

func (st *Store) Save() error {
	st.mu.RLock()
	defer st.mu.RUnlock()

	records := make([]serverRecord, 0, len(st.order))
	for _, id := range st.order {
		s, ok := st.servers[id]
		if !ok {
			continue
		}
		records = append(records, serverRecord{
			ID:                  s.ID,
			Name:                s.Name,
			Host:                s.Host,
			Port:                s.Port,
			Username:            s.Username,
			Password:            s.Password,
			HTTPS:               s.HTTPS,
			RestartType:         s.RestartType,
			RestartUnit:         s.RestartUnit,
			SSHUser:             s.SSHUser,
			SSHPort:             s.SSHPort,
			SSHKeyPath:          s.SSHKeyPath,
			AutoRestart:         s.AutoRestart,
			AutoRestartInterval: s.AutoRestartInterval,
		})
	}
	data, _ := json.MarshalIndent(records, "", "  ")
	return writeAtomic(st.dbPath, data)
}

func (st *Store) saveARR() {
	st.mu.RLock()
	defer st.mu.RUnlock()
	data, _ := json.MarshalIndent(st.arr, "", "  ")
	writeAtomic(st.arrPath, data)
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
