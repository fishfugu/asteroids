package ecscan

import (
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type Mode string

const (
	ModeAuto     Mode = "auto"
	ModeTable    Mode = "table"
	ModeOnTheFly Mode = "onthefly"
)

type Config struct {
	P       string // decimal strings for generality
	A       string
	B       string
	Mode    Mode
	MaxMem  string // e.g. "48GB"
	OutPath string // "-" for stdout
	Workers int    // 0 => default
}

func ParseFlags(args []string) (*Config, error) {
	fs := flag.NewFlagSet("ecscan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		pStr      = fs.String("p", "", "prime modulus p (decimal string, required)")
		AStr      = fs.String("A", "0", "curve parameter A (decimal)")
		BStr      = fs.String("B", "0", "curve parameter B (decimal)")
		modeStr   = fs.String("mode", "auto", "mode: auto|table|onthefly")
		maxMemStr = fs.String("max-mem", "48GB", "memory cap for auto/table (e.g. 48GB, 500MB)")
		outPath   = fs.String("out", "-", "output file path, or - for stdout")
		workers   = fs.Int("workers", 0, "number of workers (default GOMAXPROCS*4)")
	)

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(*pStr) == "" {
		return nil, errors.New("missing required --p")
	}

	mode, err := parseMode(*modeStr)
	if err != nil {
		return nil, err
	}
	// Validate parseability early (friendlier errors)
	if _, ok := new(big.Int).SetString(*pStr, 10); !ok {
		return nil, fmt.Errorf("invalid integer for --p: %q", *pStr)
	}
	if _, ok := new(big.Int).SetString(*AStr, 10); !ok {
		return nil, fmt.Errorf("invalid integer for --A: %q", *AStr)
	}
	if _, ok := new(big.Int).SetString(*BStr, 10); !ok {
		return nil, fmt.Errorf("invalid integer for --B: %q", *BStr)
	}
	if _, err := parseBytes(*maxMemStr); err != nil {
		return nil, fmt.Errorf("bad --max-mem: %v", err)
	}

	w := *workers
	if w <= 0 {
		w = runtime.GOMAXPROCS(0) * 4
	}

	return &Config{
		P:       *pStr,
		A:       *AStr,
		B:       *BStr,
		Mode:    mode,
		MaxMem:  *maxMemStr,
		OutPath: *outPath,
		Workers: w,
	}, nil
}

// --- local helpers (kept here to avoid import cycles) ---

func parseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return ModeAuto, nil
	case "table":
		return ModeTable, nil
	case "onthefly", "on-the-fly", "fly":
		return ModeOnTheFly, nil
	default:
		return ModeAuto, fmt.Errorf("unknown mode %q", s)
	}
}

func parseBytes(s string) (uint64, error) {
	if s == "" {
		return 0, errors.New("empty size")
	}
	orig := s
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := uint64(1)
	switch {
	case strings.HasSuffix(s, "KB"):
		mult, s = 1<<10, strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "MB"):
		mult, s = 1<<20, strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "GB"):
		mult, s = 1<<30, strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "TB"):
		mult, s = 1<<40, strings.TrimSuffix(s, "TB")
	case strings.HasSuffix(s, "K"):
		mult, s = 1<<10, strings.TrimSuffix(s, "K")
	case strings.HasSuffix(s, "M"):
		mult, s = 1<<20, strings.TrimSuffix(s, "M")
	case strings.HasSuffix(s, "G"):
		mult, s = 1<<30, strings.TrimSuffix(s, "G")
	}
	s = strings.TrimSpace(s)
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parse size %q: %w", orig, err)
	}
	return uint64(val * float64(mult)), nil
}
