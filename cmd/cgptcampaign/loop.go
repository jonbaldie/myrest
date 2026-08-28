package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"sync"
	"time"

	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/jwt"
)

type seed struct {
	Input  input
	Energy int
	Cover  int
}

type campaign struct {
	url      string
	service  *httpapi.Service
	verifier *jwt.Verifier
	out      *os.File
	rng      *rand.Rand

	mu       sync.Mutex
	success  []seed
	discards []seed
	seen        map[string]bool
	seenFinding map[string]bool
	runs        int
	oks      int
	skips    int
	fails    int
	findings []finding
	started  time.Time
}

func newCampaign(out *os.File) (*campaign, error) {
	service, err := startService()
	if err != nil {
		return nil, err
	}
	verifier, err := newVerifier()
	if err != nil {
		_ = service.Close()
		return nil, err
	}
	return &campaign{
		url:      service.URL(),
		service:  service,
		verifier: verifier,
		out:      out,
		rng:      rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x9e3779b97f4a7c15)),
		seen:        map[string]bool{},
		seenFinding: map[string]bool{},
		started:     time.Now(),
	}, nil
}

func (c *campaign) close() {
	if c.service != nil {
		_ = c.service.Close()
	}
}

func (c *campaign) run(deadline time.Time) {
	for _, seedInput := range seedCorpus() {
		c.step(seedInput, false)
	}
	for time.Now().Before(deadline) {
		in, fromSeed := c.nextInput()
		c.step(in, fromSeed)
	}
}

func (c *campaign) step(in input, fromSeed bool) {
	result := evaluate(c, in)
	c.observe(in, result, fromSeed)
	if result.Verdict != verdictFail {
		return
	}
	shrunk := c.shrinkFailure(in, result)
	item := finding{
		Property: result.Property,
		Detail:   result.Detail,
		Input:    formatInput(in),
		Shrunk:   formatInput(shrunk),
	}
	key := result.Property + "|" + result.Detail
	c.mu.Lock()
	if c.seenFinding[key] {
		c.mu.Unlock()
		return
	}
	c.seenFinding[key] = true
	c.fails++
	c.findings = append(c.findings, item)
	c.mu.Unlock()
	writeFinding(c.out, item)
}

func (c *campaign) nextInput() (input, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.success) == 0 && len(c.discards) == 0 {
		return generate(c.rng), false
	}
	// Escape a local minimum often enough to keep random search alive.
	if c.rng.IntN(6) == 0 {
		return generate(c.rng), false
	}
	pool := c.success
	if len(pool) == 0 || (len(c.discards) > 0 && c.rng.IntN(5) == 0) {
		pool = c.discards
	}
	if len(pool) == 0 {
		return generate(c.rng), false
	}
	chosen := &pool[c.rng.IntN(len(pool))]
	if chosen.Energy <= 0 {
		return generate(c.rng), false
	}
	chosen.Energy--
	return mutate(c.rng, chosen.Input), true
}

func (c *campaign) observe(in input, result checkResult, fromSeed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs++
	newEdges := 0
	for _, edge := range result.Cover {
		if !c.seen[edge] {
			c.seen[edge] = true
			newEdges++
		}
	}
	switch result.Verdict {
	case verdictOK:
		c.oks++
		if newEdges > 0 {
			c.success = append(c.success, seed{
				Input:  in,
				Energy: energy(in.size(), newEdges, true),
				Cover:  newEdges,
			})
		} else if !fromSeed && c.rng.IntN(30) == 0 {
			c.success = append(c.success, seed{Input: in, Energy: 2})
		}
	case verdictDiscard:
		c.skips++
		if newEdges > 0 {
			c.discards = append(c.discards, seed{
				Input:  in,
				Energy: energy(in.size(), newEdges, false),
				Cover:  newEdges,
			})
		}
	}
	const capSeeds = 400
	if len(c.success) > capSeeds {
		c.success = c.success[len(c.success)-capSeeds:]
	}
	if len(c.discards) > capSeeds/2 {
		c.discards = c.discards[len(c.discards)-capSeeds/2:]
	}
}

func energy(size, newEdges int, success bool) int {
	base := 16
	if !success {
		base = 6
	}
	value := base * newEdges
	if size > 0 {
		value = value * 32 / (32 + size/16)
	}
	if value < 1 {
		value = 1
	}
	if value > 64 {
		value = 64
	}
	return value
}

func (c *campaign) shrinkFailure(in input, original checkResult) input {
	best := in
	for round := 0; round < 24; round++ {
		progress := false
		for _, candidate := range shrink(best) {
			result := evaluate(c, candidate)
			if result.Verdict == verdictFail && result.Property == original.Property && candidate.size() < best.size() {
				best = candidate
				progress = true
			}
		}
		if !progress {
			break
		}
	}
	return best
}

func (c *campaign) report(label string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Printf(
		"STAT %s elapsed=%s runs=%d ok=%d discard=%d fail=%d edges=%d successSeeds=%d discardSeeds=%d findings=%d\n",
		label,
		time.Since(c.started).Round(time.Second),
		c.runs, c.oks, c.skips, c.fails,
		len(c.seen), len(c.success), len(c.discards), len(c.findings),
	)
	shown := map[string]bool{}
	for i, item := range c.findings {
		if shown[item.Property] && i < len(c.findings)-3 {
			continue
		}
		shown[item.Property] = true
		fmt.Printf("  finding[%d] %s: %s | %s\n", i+1, item.Property, item.Detail, item.Shrunk)
	}
}
