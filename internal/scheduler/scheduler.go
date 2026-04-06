package scheduler

import (
	"log"
	"time"

	"github.com/qbitctrl/internal/models"
	"github.com/qbitctrl/internal/qbit"
	"github.com/qbitctrl/internal/ssh"
	"github.com/qbitctrl/internal/stats"
	"github.com/qbitctrl/internal/store"
	"github.com/qbitctrl/internal/websocket"
)

type Scheduler struct {
	store  *store.Store
	hub    *websocket.Hub
	stats  *stats.Collector
	stop   chan struct{}
	ticker *time.Ticker
}

func New(st *store.Store, hub *websocket.Hub, sc *stats.Collector) *Scheduler {
	return &Scheduler{
		store: st,
		hub:   hub,
		stats: sc,
		stop:  make(chan struct{}),
	}
}

func (sc *Scheduler) Start() {
	sc.ticker = time.NewTicker(30 * time.Second)
	go sc.run()
	log.Println("[Scheduler] Uruchomiony (co 30s)")
}

func (sc *Scheduler) Stop() {
	close(sc.stop)
	if sc.ticker != nil {
		sc.ticker.Stop()
	}
}

func (sc *Scheduler) run() {
	sc.check()
	for {
		select {
		case <-sc.ticker.C:
			sc.check()
		case <-sc.stop:
			return
		}
	}
}

func (sc *Scheduler) check() {
	servers := sc.store.GetAll()
	now := time.Now()
	for _, s := range servers {
		sc.checkServer(s, now)
	}
	// Broadcast updated server list to all SSE clients
	if sc.hub != nil && sc.hub.ClientCount() > 0 {
		dtos := make([]interface{}, 0, len(servers))
		for _, s := range servers {
			dtos = append(dtos, s.ToDTO())
		}
		sc.hub.Broadcast("servers", dtos)
	}
}

func (sc *Scheduler) checkServer(s *models.QBitServer, now time.Time) {
	wasOnline := s.Online

	_, err := qbit.Version(s)
	if err != nil {
		if !qbit.Login(s) {
			s.Online = false
		}
	} else {
		s.Online = true
	}
	s.LastCheck = now

	if s.Online && !wasOnline {
		log.Printf("[%s] Wróciło online", s.Name)
		s.OfflineSince = time.Time{}
		sc.hub.Broadcast("server_online", map[string]string{"id": s.ID, "name": s.Name})
	}
	if !s.Online && s.OfflineSince.IsZero() {
		s.OfflineSince = now
		log.Printf("[%s] Offline od %s", s.Name, now.Format("15:04:05"))
		sc.hub.Broadcast("server_offline", map[string]string{"id": s.ID, "name": s.Name})
	}

	// Collect speed stats
	if s.Online {
		if tf, err := qbit.Transfer(s); err == nil {
			sc.stats.Push(s.ID, tf.DLSpeed, tf.ULSpeed)
			// Broadcast speed update to SSE clients
			if sc.hub.ClientCount() > 0 {
				sc.hub.Broadcast("speed", map[string]interface{}{
					"id": s.ID,
					"dl": tf.DLSpeed,
					"ul": tf.ULSpeed,
					"dl_fmt": tf.DLSpeedFmt,
					"ul_fmt": tf.ULSpeedFmt,
				})
			}
		}
	}

	sc.checkScheduledRestart(s, now)
}

func (sc *Scheduler) checkScheduledRestart(s *models.QBitServer, now time.Time) {
	if !s.AutoRestart || s.RestartType == "none" || s.AutoRestartInterval <= 0 {
		return
	}
	interval := time.Duration(s.AutoRestartInterval) * time.Minute
	if s.LastAutoRestart.IsZero() {
		s.LastAutoRestart = now
		return
	}
	if now.Sub(s.LastAutoRestart) < interval {
		return
	}
	s.AutoRestartCount++
	s.LastAutoRestart = now
	log.Printf("[%s] SCHEDULED RESTART #%d (co %dmin)", s.Name, s.AutoRestartCount, s.AutoRestartInterval)
	sc.hub.Broadcast("auto_restart", map[string]interface{}{
		"id":    s.ID,
		"name":  s.Name,
		"count": s.AutoRestartCount,
	})

	result := ssh.RestartWithShutdown(s, func() error {
		return qbit.Shutdown(s)
	})
	log.Printf("[%s] Restart: %s", s.Name, result)

	go func(srv *models.QBitServer) {
		for i := 0; i < 15; i++ {
			time.Sleep(5 * time.Second)
			if qbit.Login(srv) {
				log.Printf("[%s] Reconnect OK po %ds", srv.Name, (i+1)*5)
				sc.hub.Broadcast("server_online", map[string]string{"id": srv.ID, "name": srv.Name})
				return
			}
		}
		log.Printf("[%s] Reconnect timeout po 75s", srv.Name)
	}(s)
}

func (sc *Scheduler) ForceCheck() {
	go sc.check()
}

func NextRestartIn(s *models.QBitServer) int64 {
	if !s.AutoRestart || s.LastAutoRestart.IsZero() {
		return -1
	}
	interval := time.Duration(s.AutoRestartInterval) * time.Minute
	remaining := interval - time.Since(s.LastAutoRestart)
	if remaining < 0 {
		return 0
	}
	return int64(remaining.Seconds())
}
