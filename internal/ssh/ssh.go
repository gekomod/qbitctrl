package ssh

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/qbitctrl/internal/models"
)

// Run executes a command on the remote host via SSH
func Run(s *models.QBitServer, command string) (bool, string) {
	args := []string{
		"-p", fmt.Sprintf("%d", s.SSHPort),
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes",
	}
	if s.SSHKeyPath != "" {
		args = append(args, "-i", s.SSHKeyPath)
	}
	args = append(args, fmt.Sprintf("%s@%s", s.SSHUser, s.Host))
	args = append(args, command)

	cmd := exec.Command("ssh", args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if len(output) > 200 {
		output = output[:200]
	}
	if err != nil {
		return false, fmt.Sprintf("SSH błąd: %v — %s", err, output)
	}
	if output == "" {
		output = "(brak outputu)"
	}
	return true, output
}

// Test verifies SSH connectivity
func Test(s *models.QBitServer) (bool, string) {
	return Run(s, "echo OK && hostname")
}

// DoRestart performs restart via SSH (docker or systemd)
func DoRestart(s *models.QBitServer) string {
	switch s.RestartType {
	case "docker":
		container := s.RestartUnit
		if container == "" {
			container = "qbittorrent"
		}
		ok, out := Run(s, fmt.Sprintf("docker restart %s", container))
		if ok {
			return fmt.Sprintf("docker restart %s: OK — %s", container, out)
		}
		return fmt.Sprintf("docker restart %s: BŁĄD — %s", container, out)

	case "systemd":
		unit := s.RestartUnit
		if unit == "" {
			unit = "qbittorrent-nox"
		}
		ok, out := Run(s, fmt.Sprintf("systemctl restart %s", unit))
		if ok {
			return fmt.Sprintf("systemctl restart %s: OK — %s", unit, out)
		}
		return fmt.Sprintf("systemctl restart %s: BŁĄD — %s", unit, out)

	default:
		return "brak metody restartu (none)"
	}
}

// RestartWithShutdown sends qBit shutdown then SSH restart
func RestartWithShutdown(s *models.QBitServer, shutdownFn func() error) string {
	// 1. Graceful shutdown
	if shutdownFn != nil {
		_ = shutdownFn()
		time.Sleep(2 * time.Second)
	}
	// 2. SSH restart
	return DoRestart(s)
}
