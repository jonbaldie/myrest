package mysqltest

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	defaultImage    = "mysql:8.0"
	defaultRootPass = "myrest"
	defaultUser     = "myrest"
	defaultPass     = "myrest"
	readyTimeout    = 60 * time.Second
)

// Harness is a disposable MySQL 8.0+ container for tests.
type Harness struct {
	containerID string
	hostPort    string
	rootPass    string
	user        string
	pass        string
}

// Start runs MySQL 8.0+ in Docker and waits until it accepts connections.
func Start() (*Harness, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, fmt.Errorf("docker is required for the MySQL test harness: %w", err)
	}

	hostPort, err := freeTCPPort()
	if err != nil {
		return nil, err
	}

	containerName := fmt.Sprintf("myrest-mysql-%d", time.Now().UnixNano())
	run := exec.Command(
		"docker", "run", "-d",
		"--name", containerName,
		"-e", "MYSQL_ROOT_PASSWORD="+defaultRootPass,
		"-e", "MYSQL_USER="+defaultUser,
		"-e", "MYSQL_PASSWORD="+defaultPass,
		"-p", hostPort+":3306",
		defaultImage,
		"--innodb_use_native_aio=0",
	)
	output, err := run.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker run mysql: %w: %s", err, strings.TrimSpace(string(output)))
	}
	containerID := strings.TrimSpace(string(output))

	harness := &Harness{
		containerID: containerID,
		hostPort:    hostPort,
		rootPass:    defaultRootPass,
		user:        defaultUser,
		pass:        defaultPass,
	}
	if err := harness.waitReady(readyTimeout); err != nil {
		_ = harness.Stop()
		return nil, err
	}
	return harness, nil
}

// DSN returns a MySQL DSN for the harness user with multiStatements enabled.
func (h *Harness) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(127.0.0.1:%s)/?parseTime=true&multiStatements=true", h.user, h.pass, h.hostPort)
}

// RootDSN returns a root DSN for loading fixtures that create databases.
func (h *Harness) RootDSN() string {
	return fmt.Sprintf("root:%s@tcp(127.0.0.1:%s)/?parseTime=true&multiStatements=true", h.rootPass, h.hostPort)
}

// LoadSQL reads fixture SQL files and executes them as root.
func (h *Harness) LoadSQL(paths ...string) error {
	if len(paths) == 0 {
		return errors.New("LoadSQL requires at least one SQL file path")
	}

	db, err := sql.Open("mysql", h.RootDSN())
	if err != nil {
		return err
	}
	defer db.Close()

	for _, path := range paths {
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read fixture %s: %w", path, err)
		}
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("exec fixture %s: %w", filepath.Base(path), err)
		}
	}

	// Grant the test user rights on databases created by fixtures.
	grant := fmt.Sprintf("GRANT ALL PRIVILEGES ON *.* TO '%s'@'%%'", h.user)
	if _, err := db.Exec(grant); err != nil {
		return fmt.Errorf("grant fixture privileges: %w", err)
	}
	return nil
}

// Stop removes the MySQL container.
func (h *Harness) Stop() error {
	if h == nil || h.containerID == "" {
		return nil
	}
	cmd := exec.Command("docker", "rm", "-f", h.containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker rm: %w: %s", err, strings.TrimSpace(string(output)))
	}
	h.reset()
	return nil
}

func (h *Harness) reset() {
	h.containerID = ""
	h.hostPort = ""
	h.rootPass = ""
	h.user = ""
	h.pass = ""
}

func (h *Harness) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		db, err := sql.Open("mysql", h.RootDSN())
		if err != nil {
			last = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		err = db.Ping()
		_ = db.Close()
		if err == nil {
			return nil
		}
		last = err
		time.Sleep(500 * time.Millisecond)
	}
	if last == nil {
		last = errors.New("timed out waiting for MySQL")
	}
	return fmt.Errorf("MySQL not ready: %w", last)
}

func freeTCPPort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	return strconv.Itoa(port), nil
}
