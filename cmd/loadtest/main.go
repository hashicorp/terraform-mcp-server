// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

// loadtest stress-tests a streamable MCP server with concurrent clients.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	duration = flag.Duration("duration", 1*time.Minute, "duration of the load test")
	tool     = flag.String("tool", "search_providers", "tool to call")
	jsonArgs = flag.String("args", `{"query": "aws"}`, "JSON arguments to pass")
	workers  = flag.Int("workers", 10, "number of concurrent workers")
	timeout  = flag.Duration("timeout", 10*time.Second, "request timeout")
	qps      = flag.Int("qps", 10, "tool calls per second, per worker")
	verbose  = flag.Bool("v", false, "verbose logging")
	race     = flag.Bool("race", false, "race detection mode (workers=100, qps=50, duration=30s)")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: loadtest [flags] <URL>\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  loadtest http://localhost:8080/mcp\n")
		fmt.Fprintf(os.Stderr, "  loadtest -tool=search_modules -args='{\"query\": \"vpc\"}' http://localhost:8080/mcp\n")
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

	if *race {
		*workers = 100
		*qps = 50
		*duration = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	var (
		start   = time.Now()
		success atomic.Int64
		failure atomic.Int64
	)

	log.Printf("loadtest: %d workers, %d qps/worker, %s duration, tool=%s", *workers, *qps, *duration, *tool)

	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			client := mcp.NewClient(&mcp.Implementation{Name: "loadtest", Version: "v1.0.0"}, nil)
			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
			if err != nil {
				log.Printf("connect error: %v", err)
				return
			}
			defer session.Close()

			ticker := time.NewTicker(time.Second / time.Duration(*qps))
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					reqCtx, reqCancel := context.WithTimeout(ctx, *timeout)
					res, err := session.CallTool(reqCtx, &mcp.CallToolParams{
						Name:      *tool,
						Arguments: json.RawMessage(*jsonArgs),
					})
					reqCancel()

					if err != nil {
						if ctx.Err() != nil {
							return
						}
						failure.Add(1)
						if *verbose {
							log.Printf("error: %v", err)
						}
					} else {
						success.Add(1)
						if *verbose {
							data, _ := json.Marshal(res)
							log.Printf("ok: %s", data)
						}
					}
				}
			}
		}()
	}

	wg.Wait()

	dur := time.Since(start)
	succ := success.Load()
	fail := failure.Load()
	total := succ + fail

	fmt.Printf("\nResults (%s):\n", dur.Round(time.Millisecond))
	fmt.Printf("  success: %d (%.1f/s)\n", succ, float64(succ)/dur.Seconds())
	fmt.Printf("  failure: %d (%.1f/s)\n", fail, float64(fail)/dur.Seconds())
	fmt.Printf("  total:   %d (%.1f/s)\n", total, float64(total)/dur.Seconds())

	if fail > 0 {
		os.Exit(1)
	}
}
