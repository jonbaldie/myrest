// Command cgptcampaign runs a coverage-guided property campaign against the
// myrest HTTP API, the query parser, and JWT verification.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	duration := flag.Duration("duration", time.Hour, "campaign budget")
	outPath := flag.String("out", "/tmp/myrest-cgpt-findings.log", "finding log path")
	flag.Parse()

	out, err := os.OpenFile(*outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open log: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()

	campaign, err := newCampaign(out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start service: %v\n", err)
		os.Exit(1)
	}
	defer campaign.close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("CGPT campaign start. duration=%s log=%s url=%s\n", *duration, *outPath, campaign.url)
	deadline := time.Now().Add(*duration)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		campaign.run(deadline)
		close(done)
	}()

	for {
		select {
		case <-done:
			campaign.report("final")
			return
		case <-ticker.C:
			campaign.report("live")
		case <-stop:
			campaign.report("stopped")
			return
		}
	}
}

func writeFinding(out *os.File, finding finding) {
	payload, err := json.Marshal(finding)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode finding: %v\n", err)
		return
	}
	line := string(payload) + "\n"
	_, _ = out.WriteString(line)
	_ = out.Sync()
	fmt.Print("FINDING " + line)
}
