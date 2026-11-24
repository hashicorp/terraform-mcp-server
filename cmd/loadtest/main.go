// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

// loadtest stress-tests a streamable MCP server with concurrent clients.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type authTransport struct {
	token        string
	roundtripper http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.roundtripper.RoundTrip(req)
}

var (
	duration = flag.Duration("duration", 1*time.Minute, "duration of the load test")
	tool     = flag.String("tool", "search_modules", "tool to call")
	jsonArgs = flag.String("args", `{"module_query": "vpc"}`, "JSON arguments to pass")
	workers  = flag.Int("workers", 10, "number of concurrent workers")
	timeout  = flag.Duration("timeout", 10*time.Second, "timeout for connecting and for each request")
	qps      = flag.Int("qps", 10, "tool calls per second, per worker")
	verbose  = flag.Bool("v", false, "verbose logging")
	race     = flag.Bool("race", false, "race detection mode (workers=100, qps=50, duration=30s)")
)

// loadConfig is everything a run needs, so tests can run it without flags.
type loadConfig struct {
	endpoint   string
	httpClient *http.Client
	workers    int
	qps        int
	timeout    time.Duration
	tool       string
	args       json.RawMessage
	verbose    bool
}

// loadResult is what a run produced.
type loadResult struct {
	success int64
	failure int64
	// Workers that fail to connect never make a call, so they're counted on
	// their own or a down server would look like a clean run.
	connectFailures  int64
	successLatencies []time.Duration
	failureLatencies []time.Duration
}

func (r *loadResult) total() int64 { return r.success + r.failure }

// failed reports whether the run should exit non-zero. A run that never
// completed a call didn't test anything, so it fails too.
func (r *loadResult) failed() bool {
	return r.failure > 0 || r.connectFailures > 0 || r.total() == 0
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: loadtest [flags] <URL>\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  loadtest http://localhost:8080/mcp\n")
		fmt.Fprintf(os.Stderr, "  loadtest -tool=get_latest_provider_version -args='{\"namespace\": \"hashicorp\", \"name\": \"aws\"}' http://localhost:8080/mcp\n")
		fmt.Fprintf(os.Stderr, "  loadtest -workers=50 -qps=20 -duration=5m http://localhost:8080/mcp\n")
		fmt.Fprintf(os.Stderr, "  loadtest -race http://localhost:8080/mcp\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	endpoint := flag.Arg(0)
	if err := validateEndpoint(endpoint); err != nil {
		fmt.Fprintf(os.Stderr, "loadtest: %v\n", err)
		os.Exit(2)
	}

	if *race {
		*workers = 100
		*qps = 50
		*duration = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	httpClient := http.DefaultClient
	if tfeToken := os.Getenv("TFE_TOKEN"); tfeToken != "" {
		httpClient = &http.Client{
			Transport: &authTransport{
				token:        tfeToken,
				roundtripper: http.DefaultTransport,
			},
		}
		log.Print("loadtest: using bearer authentication from TFE_TOKEN")
	} else {
		log.Print("loadtest: TFE_TOKEN not set; running without authentication")
	}

	log.Printf("loadtest: %d workers, %d qps/worker, %s duration, tool=%s", *workers, *qps, *duration, *tool)

	start := time.Now()
	result := runLoad(ctx, loadConfig{
		endpoint:   endpoint,
		httpClient: httpClient,
		workers:    *workers,
		qps:        *qps,
		timeout:    *timeout,
		tool:       *tool,
		args:       json.RawMessage(*jsonArgs),
		verbose:    *verbose,
	})
	printResult(result, time.Since(start), *workers)

	if result.failed() {
		os.Exit(1)
	}
}

// validateEndpoint catches typos like a missing scheme before starting
// workers. Whether a server is listening there shows up as connect errors.
func validateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", endpoint, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid URL %q: scheme must be http or https", endpoint)
	}
	if u.Host == "" {
		return fmt.Errorf("invalid URL %q: missing host", endpoint)
	}
	return nil
}

// runLoad runs the workers until ctx is done and returns what they saw.
func runLoad(ctx context.Context, cfg loadConfig) *loadResult {
	var (
		result          = &loadResult{}
		success         atomic.Int64
		failure         atomic.Int64
		connectFailures atomic.Int64

		// Each worker keeps its own durations and adds them once it's done, so
		// timing calls doesn't add lock contention.
		latencyMu sync.Mutex
		wg        sync.WaitGroup
	)

	for i := 0; i < cfg.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			session, err := connect(ctx, cfg)
			if err != nil {
				// Only skip it when the run itself ended (duration or ctrl-c)
				if ctx.Err() == nil {
					connectFailures.Add(1)
				}
				log.Printf("connect error: %v", err)
				return
			}
			defer session.Close()

			var okDurations, errDurations []time.Duration
			defer func() {
				latencyMu.Lock()
				result.successLatencies = append(result.successLatencies, okDurations...)
				result.failureLatencies = append(result.failureLatencies, errDurations...)
				latencyMu.Unlock()
			}()

			ticker := time.NewTicker(time.Second / time.Duration(cfg.qps))
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					reqCtx, reqCancel := context.WithTimeout(ctx, cfg.timeout)
					callStart := time.Now()
					res, err := session.CallTool(reqCtx, &mcp.CallToolParams{
						Name:      cfg.tool,
						Arguments: cfg.args,
					})
					elapsed := time.Since(callStart)
					reqCancel()

					if err != nil {
						if ctx.Err() != nil {
							return
						}
						failure.Add(1)
						errDurations = append(errDurations, elapsed)
						if cfg.verbose {
							log.Printf("error: %v", err)
						}
					} else if res.IsError {
						// The call went through but the tool failed (not found,
						// rate limited, etc), so count it as a failure
						failure.Add(1)
						errDurations = append(errDurations, elapsed)
						if cfg.verbose {
							data, _ := json.Marshal(res)
							log.Printf("tool error: %s", data)
						}
					} else {
						success.Add(1)
						okDurations = append(okDurations, elapsed)
						if cfg.verbose {
							data, _ := json.Marshal(res)
							log.Printf("ok: %s", data)
						}
					}
				}
			}
		}()
	}

	wg.Wait()
	result.success = success.Load()
	result.failure = failure.Load()
	result.connectFailures = connectFailures.Load()
	return result
}

type connectResult struct {
	session *mcp.ClientSession
	err     error
}

// connect has its own timeout so a server that takes the connection but never
// finishes the handshake shows up as a connect error instead of eating the
// whole run. go-sdk can take a few seconds to return after the deadline, so we
// don't wait on it, and if it connects late the session gets closed.
func connect(ctx context.Context, cfg loadConfig) (*mcp.ClientSession, error) {
	connectCtx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	results := make(chan connectResult)
	abandoned := make(chan struct{})
	go func() {
		client := mcp.NewClient(&mcp.Implementation{Name: "loadtest", Version: "v1.0.0"}, nil)
		session, err := client.Connect(connectCtx, &mcp.StreamableClientTransport{Endpoint: cfg.endpoint, HTTPClient: cfg.httpClient}, nil)
		select {
		case results <- connectResult{session, err}:
		case <-abandoned:
			if session != nil {
				_ = session.Close()
			}
		}
	}()

	select {
	case r := <-results:
		return r.session, r.err
	case <-connectCtx.Done():
		close(abandoned)
		return nil, fmt.Errorf("connect: %w", connectCtx.Err())
	}
}

func printResult(r *loadResult, dur time.Duration, workers int) {
	fmt.Printf("\nResults (%s):\n", dur.Round(time.Millisecond))
	fmt.Printf("  success: %d (%.1f/s)\n", r.success, float64(r.success)/dur.Seconds())
	fmt.Printf("  failure: %d (%.1f/s)\n", r.failure, float64(r.failure)/dur.Seconds())
	fmt.Printf("  total:   %d (%.1f/s)\n", r.total(), float64(r.total())/dur.Seconds())
	if r.connectFailures > 0 {
		fmt.Printf("  connect errors: %d of %d workers couldn't connect\n", r.connectFailures, workers)
	}
	if r.total() == 0 {
		fmt.Printf("  no tool calls completed\n")
	}
	if s := summarizeLatencies(r.successLatencies); s.count > 0 {
		fmt.Printf("  latency (success): %s\n", s)
	}
	if s := summarizeLatencies(r.failureLatencies); s.count > 0 {
		fmt.Printf("  latency (failure): %s\n", s)
	}
}
