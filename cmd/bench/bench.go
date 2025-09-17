package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Matches ectorus JSON (strings for big ints for simplicity)
type pt struct {
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
	Inf bool   `json:"inf"`
}

type ectorusOut struct {
	P          string `json:"p"`
	A          string `json:"A"`
	B          string `json:"B"`
	PointCount string `json:"pointCount,omitempty"`
	Complete   bool   `json:"complete"`
	Found      []pt   `json:"found"`
	Lines      int    `json:"linesProcessed"`
}

type scenario struct {
	Name    string
	A, B, P string
	Args    []string // extra flags, e.g. {"-grid","-count_first"}
	Timeout time.Duration
}

func runScenario(path string, sc scenario, reps int) (time.Duration, ectorusOut, error) {
	var best time.Duration
	var last ectorusOut
	for i := range reps {
		ctx, cancel := context.WithTimeout(context.Background(), sc.Timeout)
		defer cancel()
		args := []string{"-A", sc.A, "-B", sc.B, "-p", sc.P, "-json"}
		args = append(args, sc.Args...)
		cmd := exec.CommandContext(ctx, path, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		t0 := time.Now()
		err := cmd.Run()
		dur := time.Since(t0)
		if ctx.Err() == context.DeadlineExceeded {
			return dur, last, fmt.Errorf("timeout: %s", sc.Name)
		}
		if err != nil {
			return dur, last, fmt.Errorf("%s failed: %v\n%s", sc.Name, err, stderr.String())
		}
		var out ectorusOut
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			return dur, last, fmt.Errorf("%s parse json: %v\nraw=%s", sc.Name, err, stdout.String())
		}
		last = out
		if i == 0 || dur < best {
			best = dur
		}
	}
	return best, last, nil
}

func main() {
	var ectorusPath string
	var reps int
	var timeout time.Duration
	flag.StringVar(&ectorusPath, "ectorus", "./ectorus", "path to ectorus binary")
	flag.IntVar(&reps, "reps", 1, "repetitions per scenario (report best)")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "per-run timeout")
	flag.Parse()

	if _, err := os.Stat(ectorusPath); err != nil {
		fmt.Fprintf(os.Stderr, "ectorus not found at %s (build it first)\n", ectorusPath)
		os.Exit(2)
	}

	scenarios := []scenario{
		{Name: "supersingular p=101 y^2=x^3+1 (grid)", A: "0", B: "1", P: "101", Args: []string{"-grid", "-count_first"}, Timeout: timeout},
		{Name: "ordinary-ish p=101 A=1,B=1 (grid)", A: "1", B: "1", P: "101", Args: []string{"-grid", "-count_first"}, Timeout: timeout},
		{Name: "implicit p=1009 A=0,B=7 (count_first)", A: "0", B: "7", P: "1009", Args: []string{"-count_first"}, Timeout: timeout},
		{Name: "implicit p=10007 A=2,B=3 (count_first)", A: "2", B: "3", P: "10007", Args: []string{"-count_first"}, Timeout: timeout},
	}

	fmt.Println("asteroids bench — running scenarios")
	for _, sc := range scenarios {
		dur, out, err := runScenario(ectorusPath, sc, reps)
		if err != nil {
			fmt.Printf("%-40s : ERROR: %v\n", sc.Name, err)
			continue
		}
		pts := len(out.Found)
		pc := out.PointCount
		if pc == "" {
			pc = "?"
		}
		fmt.Printf("%-40s : %8s  points=%-6d  lines=%-6d  complete=%v\n",
			sc.Name, dur.Truncate(time.Microsecond), pts, out.Lines, out.Complete)
		fmt.Printf("    curve: p=%s  A=%s  B=%s  count=%s\n", out.P, out.A, out.B, pc)
	}

}
