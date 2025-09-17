package ecscan

import (
	"fmt"
	"log"
	"math/big"
)

// safety factor for table-mode RAM check (use up to 80% of cap)
const safety80 = 8.0 / 10.0

func Run(cfg *Config) error {
	// Parse numbers as big.Int first (keeps one codepath for validation)
	p := mustParseBig(cfg.P, "p")
	A := mustParseBig(cfg.A, "A")
	B := mustParseBig(cfg.B, "B")

	maxMemBytes, err := parseBytes(cfg.MaxMem)
	if err != nil {
		return fmt.Errorf("bad --max-mem: %v", err)
	}

	// Fast path if p fits in uint64 and p < 2^63
	if pu64, ok := fitsUint64(p); ok && pu64 < (1<<63) {
		Au64, okA := fitsUint64(A)
		Bu64, okB := fitsUint64(B)
		if !okA || !okB {
			return fmt.Errorf("A or B does not fit into uint64 while p does")
		}

		// Estimate table memory: 4B entry if p < 2^32, else 8B (store y)
		entryBytes := uint64(4)
		if pu64 >= (1 << 32) {
			entryBytes = 8
		}
		tableBytes := entryBytes * pu64

		mode := cfg.Mode
		if mode == ModeAuto {
			if float64(tableBytes) <= float64(maxMemBytes)*safety80 {
				mode = ModeTable
			} else {
				mode = ModeOnTheFly
			}
			log.Printf("auto mode => %s (table≈%.2fGB, cap=%.2fGB)",
				mode, float64(tableBytes)/(1<<30), float64(maxMemBytes)/(1<<30))
		}

		if mode == ModeTable && float64(tableBytes) > float64(maxMemBytes)*safety80 {
			return fmt.Errorf("mode=table needs ~%.2f GB; allowed ~%.2f GB (cap*safety)",
				float64(tableBytes)/(1<<30), float64(maxMemBytes)*safety80/(1<<30))
		}

		return internalscan.enumerateU64(pu64, Au64, Bu64, toEnumMode(mode), maxMemBytes, cfg.OutPath, cfg.Workers)
	}

	// Fallback big.Int path (only on-the-fly is sensible)
	if cfg.Mode == ModeTable {
		return fmt.Errorf("mode=table is not supported when p does not fit in uint64")
	}
	mode := cfg.Mode
	if mode == ModeAuto {
		mode = ModeOnTheFly
		log.Printf("auto mode => onthefly (big.Int path)")
	}
	return enumerateBig(p, A, B, toEnumMode(mode), cfg.OutPath, cfg.Workers)
}

// --- local helpers (mirror the ones used in the rest of the package) ---

func mustParseBig(s, name string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		log.Fatalf("invalid integer for %s: %q", name, s)
	}
	return n
}

func fitsUint64(z *big.Int) (uint64, bool) {
	if z.Sign() < 0 || z.BitLen() > 64 {
		return 0, false
	}
	return z.Uint64(), true
}

// If your enumerateU64/enumerateBig expect a custom enum type, adapt here:
type enumMode int

const (
	enumAuto enumMode = iota
	enumTable
	enumOnTheFly
)

func toEnumMode(m Mode) enumMode {
	switch m {
	case ModeTable:
		return enumTable
	case ModeOnTheFly:
		return enumOnTheFly
	default:
		return enumAuto
	}
}
