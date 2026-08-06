package main_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// processDeadline is how long a test waits for a myrest process to answer.
const processDeadline = 20 * time.Second

// binary is the myrest command this test package builds once and then runs as
// a process, so that the config surface is observed where an operator sees it.
var binary string

func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "myrest-process")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp directory: %v\n", err)
		os.Exit(1)
	}
	binary = filepath.Join(directory, "myrest")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "build myrest: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(directory)
	os.Exit(code)
}

// cfg-001: a half-configured process must stay off the wire and must say
// which knob the minimum run set still needs.
func TestProcessDoesNotServeWhenTheMinimumRunSetIsIncomplete(t *testing.T) {
	t.Parallel()

	messages, err := runUntilStop(t, map[string]string{"MYREST_DB_ANON_ROLE": "myrest_anon"})
	if err == nil {
		t.Fatalf("myrest stopped without failure; messages: %s", messages)
	}

	for _, knob := range []string{"db-uri", "db-schemas"} {
		if !strings.Contains(messages, knob) {
			t.Errorf("messages %q do not name the missing knob %q", messages, knob)
		}
	}
	if strings.Contains(messages, "listening") {
		t.Errorf("messages %q say the process served the API", messages)
	}
}

// cfg-002: MYREST_* variables alone must behave as a config file alone.
func TestProcessBehavesTheSameFromEnvironmentVariablesAsFromAConfigFile(t *testing.T) {
	t.Parallel()

	fromEnvironment := start(t, map[string]string{
		"MYREST_DB_URI":       "mysql://authenticator@127.0.0.1:3306/",
		"MYREST_DB_SCHEMAS":   "shop",
		"MYREST_DB_ANON_ROLE": "myrest_anon",
	})
	fromFile := start(t, nil, configFile(t, `db-uri = "mysql://authenticator@127.0.0.1:3306/"
db-schemas = "shop"
db-anon-role = "myrest_anon"
`))

	environmentURL, environmentExposed := fromEnvironment.waitForServe(t)
	fileURL, fileExposed := fromFile.waitForServe(t)

	if environmentExposed != fileExposed {
		t.Errorf("environment exposes %q, the file exposes %q", environmentExposed, fileExposed)
	}
	if environmentBody, fileBody := get(t, environmentURL), get(t, fileURL); environmentBody != fileBody {
		t.Errorf("environment body %q differs from file body %q", environmentBody, fileBody)
	}
}

// cfg-003: no knob on the drop list is needed to start the process.
func TestProcessStartsWithTheMinimumRunSetAlone(t *testing.T) {
	t.Parallel()

	process := start(t, nil, configFile(t, `db-uri = "mysql://authenticator@127.0.0.1:3306/"
db-schemas = "shop"
jwt-secret = "reallyreallyreallyreallyverysafe"
`))

	base, _ := process.waitForServe(t)
	if body := get(t, base); !strings.Contains(body, `"service":"myrest"`) {
		t.Fatalf("body = %q, does not come from myrest", body)
	}
}

func TestRestartAppliesAChangedConfigurationValue(t *testing.T) {
	t.Parallel()

	path := configFile(t, `db-uri = "mysql://authenticator@127.0.0.1:3306/"
db-schemas = "shop"
db-anon-role = "myrest_anon"
`)

	before := start(t, nil, path)
	if line := before.waitForLine(t, "listening"); !strings.Contains(line, "databases=shop") {
		t.Fatalf("start line %q does not expose database shop", line)
	}
	before.stop()

	rewrite(t, path, `db-uri = "mysql://authenticator@127.0.0.1:3306/"
db-schemas = "warehouse"
db-anon-role = "myrest_anon"
`)

	after := start(t, nil, path)
	if line := after.waitForLine(t, "listening"); !strings.Contains(line, "databases=warehouse") {
		t.Fatalf("line after restart %q does not expose database warehouse", line)
	}
}

func TestProcessRefusesMoreThanOneConfigFileArgument(t *testing.T) {
	t.Parallel()

	messages, err := runUntilStop(t, nil, "first.conf", "second.conf")
	if err == nil {
		t.Fatalf("myrest stopped without failure; messages: %s", messages)
	}
	if !strings.Contains(messages, "usage") {
		t.Fatalf("messages %q do not show the usage of myrest", messages)
	}
}

// process is a running myrest command and the messages it writes.
type process struct {
	command  *exec.Cmd
	messages chan string
	scanned  chan struct{}
	seen     []string
}

// runUntilStop runs myrest until it stops by itself, and gives back the
// messages it wrote and how it stopped.
func runUntilStop(t *testing.T, env map[string]string, args ...string) (string, error) {
	t.Helper()

	context, cancel := context.WithTimeout(t.Context(), processDeadline)
	defer cancel()

	command := exec.CommandContext(context, binary, args...)
	command.Env = entries(env)
	messages, err := command.CombinedOutput()
	return string(messages), err
}

// start runs the myrest binary with the given environment and arguments.
func start(t *testing.T, env map[string]string, args ...string) *process {
	t.Helper()

	command := exec.Command(binary, args...)
	command.Env = entries(env)
	pipe, err := command.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start myrest: %v", err)
	}

	running := &process{
		command:  command,
		messages: make(chan string, 64),
		scanned:  make(chan struct{}),
	}
	go func() {
		defer close(running.scanned)
		defer close(running.messages)
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			running.messages <- scanner.Text()
		}
	}()
	t.Cleanup(running.stop)
	return running
}

// stop ends the process. A stopped process stays stopped.
func (p *process) stop() {
	_ = p.command.Process.Kill()
	<-p.scanned
	_ = p.command.Wait()
}

// waitForLine gives back the first message that holds want.
func (p *process) waitForLine(t *testing.T, want string) string {
	t.Helper()

	deadline := time.After(processDeadline)
	for {
		select {
		case message, open := <-p.messages:
			if !open {
				t.Fatalf("myrest stopped before it wrote %q; messages: %v", want, p.seen)
			}
			p.seen = append(p.seen, message)
			if strings.Contains(message, want) {
				return message
			}
		case <-deadline:
			t.Fatalf("myrest did not write %q in time; messages: %v", want, p.seen)
		}
	}
}

// waitForServe gives back the base URL the process serves, and what it says it
// exposes there.
func (p *process) waitForServe(t *testing.T) (string, string) {
	t.Helper()

	line := p.waitForLine(t, "listening on ")
	_, address, _ := strings.Cut(line, "listening on ")
	base, exposed, _ := strings.Cut(address, " ")
	return base, exposed
}

// entries builds a process environment that holds only what the test sets.
func entries(env map[string]string) []string {
	entries := []string{"PATH=" + os.Getenv("PATH"), "MYREST_LISTEN=127.0.0.1:0"}
	for name, value := range env {
		entries = append(entries, name+"="+value)
	}
	return entries
}

// configFile writes config file text and gives back its path.
func configFile(t *testing.T, text string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "myrest.conf")
	rewrite(t, path, text)
	return path
}

// rewrite replaces the text of a config file.
func rewrite(t *testing.T, path string, text string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// get reads the body of GET / from a running myrest process.
func get(t *testing.T, base string) string {
	t.Helper()

	response, err := http.Get(base + "/")
	if err != nil {
		t.Fatalf("GET %s/: %v", base, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}
