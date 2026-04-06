package qbit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qbitctrl/internal/models"
)

// Login authenticates with qBittorrent WebAPI
func Login(s *models.QBitServer) bool {
	form := url.Values{
		"username": {s.Username},
		"password": {s.Password},
	}
	req, err := http.NewRequest("POST", s.BaseURL()+"/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		s.Online = false
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", s.Schema()+"://"+s.Host+fmt.Sprintf(":%d", s.Port))

	resp, err := s.GetClient().Do(req)
	if err != nil {
		s.Online = false
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if strings.TrimSpace(string(body)) != "Ok." {
		s.Online = false
		return false
	}

	// Extract SID cookie
	for _, c := range resp.Cookies() {
		if c.Name == "SID" {
			s.Mu.Lock()
			s.Cookie = c.Value
			s.Mu.Unlock()
			break
		}
	}
	s.Online = true
	s.LastCheck = time.Now()
	return true
}

// Get makes authenticated GET request
func Get(s *models.QBitServer, endpoint string, params url.Values) ([]byte, error) {
	u := s.BaseURL() + "/" + endpoint
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	s.Mu.RLock()
	cookie := s.Cookie
	s.Mu.RUnlock()
	req.AddCookie(&http.Cookie{Name: "SID", Value: cookie})

	resp, err := s.GetClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		// Re-login
		if Login(s) {
			return Get(s, endpoint, params)
		}
		return nil, fmt.Errorf("auth failed")
	}
	return io.ReadAll(resp.Body)
}

// Post makes authenticated POST request
func Post(s *models.QBitServer, endpoint string, form url.Values) error {
	req, err := http.NewRequest("POST", s.BaseURL()+"/"+endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.Mu.RLock()
	cookie := s.Cookie
	s.Mu.RUnlock()
	req.AddCookie(&http.Cookie{Name: "SID", Value: cookie})

	resp, err := s.GetClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		if Login(s) {
			return Post(s, endpoint, form)
		}
		return fmt.Errorf("auth failed")
	}
	return nil
}

// Version returns qBittorrent version string
func Version(s *models.QBitServer) (string, error) {
	b, err := Get(s, "app/version", nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// TransferInfo returns download/upload speeds
type TransferInfo struct {
	DLSpeed    int64  `json:"dl_info_speed"`
	ULSpeed    int64  `json:"up_info_speed"`
	DLSpeedFmt string `json:"dl_speed_fmt"`
	ULSpeedFmt string `json:"up_speed_fmt"`
}

func Transfer(s *models.QBitServer) (*TransferInfo, error) {
	b, err := Get(s, "transfer/info", nil)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	dl := int64(toFloat(raw["dl_info_speed"]))
	ul := int64(toFloat(raw["up_info_speed"]))
	return &TransferInfo{
		DLSpeed:    dl,
		ULSpeed:    ul,
		DLSpeedFmt: fmtSpeed(dl),
		ULSpeedFmt: fmtSpeed(ul),
	}, nil
}

// Torrent represents a qBittorrent torrent entry
type Torrent struct {
	Hash     string  `json:"hash"`
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
    DLSpeed  int64   `json:"dlspeed"`
    ULSpeed  int64   `json:"upspeed"`
	State    string  `json:"state"`
	ETA      int64   `json:"eta"`
	Ratio    float64 `json:"ratio"`
	Category string  `json:"category"`
	NumSeeds int     `json:"num_seeds"`
}

func Torrents(s *models.QBitServer) ([]Torrent, error) {
	b, err := Get(s, "torrents/info", nil)
	if err != nil {
		return nil, err
	}
	var torrents []Torrent
	if err := json.Unmarshal(b, &torrents); err != nil {
		return nil, err
	}
	// Convert progress 0-1 to 0-100
	for i := range torrents {
		torrents[i].Progress = torrents[i].Progress * 100
	}
	return torrents, nil
}

// TorrentFile represents a file inside a torrent
type TorrentFile struct {
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Priority int     `json:"priority"`
}

func TorrentFiles(s *models.QBitServer, hash string) ([]TorrentFile, error) {
	b, err := Get(s, "torrents/files", url.Values{"hash": {hash}})
	if err != nil {
		return nil, err
	}
	var files []TorrentFile
	return files, json.Unmarshal(b, &files)
}

func PauseTorrent(s *models.QBitServer, hash string) error {
	return Post(s, "torrents/pause", url.Values{"hashes": {hash}})
}

func ResumeTorrent(s *models.QBitServer, hash string) error {
	return Post(s, "torrents/resume", url.Values{"hashes": {hash}})
}

func DeleteTorrent(s *models.QBitServer, hash string, deleteFiles bool) error {
	df := "false"
	if deleteFiles {
		df = "true"
	}
	return Post(s, "torrents/delete", url.Values{"hashes": {hash}, "deleteFiles": {df}})
}

func SetCategory(s *models.QBitServer, hash, category string) error {
	return Post(s, "torrents/setCategory", url.Values{"hashes": {hash}, "category": {category}})
}

func SetDownloadLimit(s *models.QBitServer, hash string, limit int64) error {
	return Post(s, "torrents/setDownloadLimit", url.Values{
		"hashes": {hash}, "limit": {fmt.Sprintf("%d", limit)},
	})
}

func SetUploadLimit(s *models.QBitServer, hash string, limit int64) error {
	return Post(s, "torrents/setUploadLimit", url.Values{
		"hashes": {hash}, "limit": {fmt.Sprintf("%d", limit)},
	})
}

func AddMagnet(s *models.QBitServer, magnet, category, savePath string, paused bool) error {
	form := url.Values{"urls": {magnet}}
	if category != "" {
		form.Set("category", category)
	}
	if savePath != "" {
		form.Set("savepath", savePath)
	}
	if paused {
		form.Set("paused", "true")
	}
	return Post(s, "torrents/add", form)
}

// AddTorrentFile adds a torrent from a .torrent file
func AddTorrentFile(s *models.QBitServer, data []byte, filename, category, savePath string, paused bool) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	// Add torrent file
	part, err := writer.CreateFormFile("torrents", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	
	// Add optional parameters
	if category != "" {
		writer.WriteField("category", category)
	}
	if savePath != "" {
		writer.WriteField("savepath", savePath)
	}
	if paused {
		writer.WriteField("paused", "true")
	}
	
	if err := writer.Close(); err != nil {
		return err
	}
	
	url := s.BaseURL() + "/torrents/add"
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", writer.FormDataContentType())
	s.Mu.RLock()
	cookie := s.Cookie
	s.Mu.RUnlock()
	req.AddCookie(&http.Cookie{Name: "SID", Value: cookie})
	
	resp, err := s.GetClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 403 {
		if Login(s) {
			return AddTorrentFile(s, data, filename, category, savePath, paused)
		}
		return fmt.Errorf("auth failed")
	}
	
	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to add torrent: %s", string(respBody))
	}
	
	return nil
}

func Shutdown(s *models.QBitServer) error {
	return Post(s, "app/shutdown", nil)
}

func PauseAll(s *models.QBitServer) error {
	return Post(s, "torrents/pause", url.Values{"hashes": {"all"}})
}

func ResumeAll(s *models.QBitServer) error {
	return Post(s, "torrents/resume", url.Values{"hashes": {"all"}})
}

func BulkResume(s *models.QBitServer, hashes []string) error {
	return Post(s, "torrents/resume", url.Values{"hashes": {strings.Join(hashes, "|")}})
}

func BulkPause(s *models.QBitServer, hashes []string) error {
	return Post(s, "torrents/pause", url.Values{"hashes": {strings.Join(hashes, "|")}})
}

// Helpers
func fmtSpeed(bps int64) string {
	switch {
	case bps >= 1048576:
		return fmt.Sprintf("%.1f MB/s", float64(bps)/1048576)
	case bps >= 1024:
		return fmt.Sprintf("%.0f KB/s", float64(bps)/1024)
	default:
		return fmt.Sprintf("%d B/s", bps)
	}
}

func toFloat(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	}
	return 0
}
