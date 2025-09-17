// cmd/benchscan/benchscan.go
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

type runResult struct {
	points   int64
	duration time.Duration
	err      error
}

// detectInfinitySentinel returns true if the line is the "point at infinity" marker.
func detectInfinitySentinel(line string) bool {
	s := strings.TrimSpace(line)
	// big.Int path: "-1 -1"
	if s == "-1 -1" {
		return true
	}
	// uint64 path: MaxUint64 MaxUint64
	if s == "18446744073709551615 18446744073709551615" {
		return true
	}
	return false
}

func runOnce(ecscan string, args []string, timeout time.Duration, quiet bool) runResult {
	ctx := context.Background()
	var cancel func()
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, ecscan, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return runResult{err: fmt.Errorf("stdout pipe: %w", err)}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return runResult{err: fmt.Errorf("stderr pipe: %w", err)}
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return runResult{err: fmt.Errorf("start: %w", err)}
	}

	// Stream-count stdout
	var points int64
	var lastLine string
	sc := bufio.NewScanner(stdout)
	// lines are tiny ("x y"), default buffer is fine; set larger if needed:
	// buf := make([]byte, 0, 64*1024); sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		lastLine = sc.Text()
		points++
	}
	if err := sc.Err(); err != nil {
		// keep reading stderr for context
		slurp, _ := bufio.NewReader(stderr).ReadString(0)
		return runResult{err: fmt.Errorf("scan stdout: %w (stderr: %q)", err, slurp)}
	}

	// Drain stderr (log lines from ecscan) without failing the run.
	if !quiet {
		se := bufio.NewScanner(stderr)
		for se.Scan() {
			log.Printf("[ecscan] %s", se.Text())
		}
	}

	// Wait for exit
	if err := cmd.Wait(); err != nil {
		return runResult{err: fmt.Errorf("wait: %w", err)}
	}
	dur := time.Since(start)

	// Adjust for infinity sentinel if present
	if points > 0 && detectInfinitySentinel(lastLine) {
		points--
	}

	return runResult{points: points, duration: dur, err: nil}
}

func main() {
	var (
		// path to ecscan binary
		bin = flag.String("ecscan", "./bin/ecscan", "path to ecscan binary")

		// ecscan flags
		p       = flag.String("p", "", "prime modulus p (decimal string, required)")
		A       = flag.String("A", "0", "curve parameter A (decimal)")
		B       = flag.String("B", "0", "curve parameter B (decimal)")
		mode    = flag.String("mode", "auto", "ecscan mode: auto|table|onthefly")
		maxMem  = flag.String("max-mem", "48GB", "memory cap for table-mode decision")
		workers = flag.Int("workers", 0, "worker override (0 => GOMAXPROCS*4)")

		// bench controls
		runs    = flag.Int("runs", 3, "number of timed runs")
		warmup  = flag.Int("warmup", 1, "number of warmup runs (not timed in summary)")
		timeout = flag.Duration("timeout", 0, "per-run timeout (e.g. 10m, 0 = none)")
		label   = flag.String("label", "", "optional label for this scenario")
		quiet   = flag.Bool("quiet", false, "suppress ecscan stderr logs")
	)
	flag.Parse()

	if strings.TrimSpace(*p) == "" {
		log.Fatal("benchscan: missing required -p")
	}

	// Build ecscan args – output to stdout so we can count lines.
	args := []string{
		"--p=" + *p,
		"--A=" + *A,
		"--B=" + *B,
		"--mode=" + *mode,
		"--max-mem=" + *maxMem,
		"--out=-",
	}
	if *workers > 0 {
		args = append(args, fmt.Sprintf("--workers=%d", *workers))
	}

	title := "ecscan bench"
	if *label != "" {
		title += " - " + *label
	}
	log.Printf("%s", title)
	log.Printf("cmd: %s %s", *bin, strings.Join(args, " "))

	// Warmups
	for i := 0; i < *warmup; i++ {
		if !*quiet {
			log.Printf("warmup %d/%d ...", i+1, *warmup)
		}
		_ = runOnce(*bin, args, *timeout, *quiet) // ignore results
	}

	// Timed runs
	var total time.Duration
	var min, max time.Duration
	var lastPoints int64 = -1
	for i := 0; i < *runs; i++ {
		res := runOnce(*bin, args, *timeout, *quiet)
		if res.err != nil {
			log.Fatalf("run %d/%d failed: %v", i+1, *runs, res.err)
		}
		if lastPoints >= 0 && res.points != lastPoints {
			log.Printf("warning: point count changed between runs (%d -> %d)", lastPoints, res.points)
		}
		lastPoints = res.points

		if !*quiet {
			log.Printf("run %d/%d: %v, points=%d", i+1, *runs, res.duration, res.points)
		}
		if i == 0 || res.duration < min {
			min = res.duration
		}
		if res.duration > max {
			max = res.duration
		}
		total += res.duration
	}

	avg := time.Duration(0)
	if *runs > 0 {
		avg = time.Duration(int64(total) / int64(*runs))
	}

	fmt.Println("---- summary ----")
	fmt.Printf("label:    %s\n", title)
	fmt.Printf("p:        %s\n", *p)
	fmt.Printf("A, B:     %s, %s\n", *A, *B)
	fmt.Printf("mode:     %s\n", *mode)
	fmt.Printf("max-mem:  %s\n", *maxMem)
	if *workers > 0 {
		fmt.Printf("workers:  %d\n", *workers)
	}
	fmt.Printf("runs:     %d (warmup=%d)\n", *runs, *warmup)
	fmt.Printf("points:   %d (affine; infinity sentinel excluded if present)\n", lastPoints)
	fmt.Printf("time:     avg=%v  min=%v  max=%v\n", avg, min, max)
}
