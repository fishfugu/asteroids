package ecscan

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"math/big"
	"math/bits"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Mode int

const (
	ModeAuto Mode = iota
	ModeTable
	ModeOnTheFly
)

type PointU64 struct{ X, Y uint64 }
type PointBig struct{ X, Y *big.Int }

// ------------------- helpers: parsing & memory -------------------

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
	bytes := uint64(val * float64(mult))
	return bytes, nil
}

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

func mustParseBig(s, name string) *big.Int {
	if s == "" {
		log.Fatalf("missing required %s", name)
	}
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

// ------------------- uint64 mod arithmetic (p < 2^63) -------------------

type mod64 struct{ p uint64 }

func (m mod64) add(a, b uint64) uint64 {
	c := a + b
	if c >= m.p || c < a {
		c -= m.p
	}
	return c
}
func (m mod64) sub(a, b uint64) uint64 {
	if a >= b {
		return a - b
	}
	return a + m.p - b
}
func (m mod64) mul(a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	// (hi,lo) mod p
	// Use 128/64 division to reduce: q = (hi:lo)/p; r = (hi:lo) - q*p
	_, r := bits.Div64(hi, lo, m.p)
	return r
}
func (m mod64) pow(a, e uint64) uint64 {
	res := uint64(1)
	base := a % m.p
	for e > 0 {
		if e&1 == 1 {
			res = m.mul(res, base)
		}
		base = m.mul(base, base)
		e >>= 1
	}
	return res
}

func legendre64(a, p uint64) int {
	if a == 0 {
		return 0
	}
	m := mod64{p}
	l := m.pow(a, (p-1)/2)
	if l == 1 {
		return 1
	}
	if l == p-1 {
		return -1
	}
	return 0
}

// Tonelli–Shanks for prime p (odd); returns y with y^2 ≡ n (mod p); panics if no root.
func tonelli64(n, p uint64) uint64 {
	if n == 0 {
		return 0
	}
	if p == 2 {
		return n
	}
	if legendre64(n, p) != 1 {
		panic("tonelli64: non-residue")
	}
	// Factor p-1 = q * 2^s with q odd
	q := p - 1
	s := 0
	for q&1 == 0 {
		q >>= 1
		s++
	}
	m := mod64{p}
	// z = quadratic non-residue
	var z uint64 = 2
	for legendre64(z, p) != -1 {
		z++
	}
	c := m.pow(z, q)
	x := m.pow(n, (q+1)/2)
	t := m.pow(n, q)
	si := s
	for t != 1 {
		// find i s.t. t^(2^i) = 1
		i := 1
		t2i := m.mul(t, t)
		for t2i != 1 {
			t2i = m.mul(t2i, t2i)
			i++
			if i == si {
				panic("tonelli64: loop i reached s")
			}
		}
		b := m.pow(c, 1<<uint(si-i-1))
		x = m.mul(x, b)
		b2 := m.mul(b, b)
		t = m.mul(t, b2)
		c = b2
		si = i
	}
	return x
}

// ------------------- big.Int mod arithmetic (fallback for huge p) -------------------

type modBig struct {
	p *big.Int
}

var (
	b0 = big.NewInt(0)
	b1 = big.NewInt(1)
	b2 = big.NewInt(2)
)

func (m modBig) norm(a *big.Int) *big.Int {
	var r big.Int
	r.Mod(a, m.p)
	if r.Sign() < 0 {
		r.Add(&r, m.p)
	}
	return &r
}
func (m modBig) add(a, b *big.Int) *big.Int {
	var r big.Int
	r.Add(a, b)
	r.Mod(&r, m.p)
	return &r
}
func (m modBig) sub(a, b *big.Int) *big.Int {
	var r big.Int
	r.Sub(a, b)
	r.Mod(&r, m.p)
	return &r
}
func (m modBig) mul(a, b *big.Int) *big.Int {
	var r big.Int
	r.Mul(a, b)
	r.Mod(&r, m.p)
	return &r
}
func (m modBig) pow(a, e *big.Int) *big.Int {
	var r big.Int
	r.Exp(a, e, m.p)
	return &r
}
func legendreBig(a, p *big.Int) int {
	if a.Sign() == 0 {
		return 0
	}
	var e big.Int
	e.Sub(p, b1)
	e.Rsh(&e, 1) // (p-1)/2
	var l big.Int
	l.Exp(a, &e, p)
	if l.Cmp(b1) == 0 {
		return 1
	}
	var pm1 big.Int
	pm1.Sub(p, b1)
	if l.Cmp(&pm1) == 0 {
		return -1
	}
	return 0
}
func tonelliBig(n, p *big.Int) *big.Int {
	if n.Sign() == 0 {
		return new(big.Int)
	}
	if p.Bit(0) == 0 {
		panic("tonelliBig: even modulus not supported")
	}
	if legendreBig(n, p) != 1 {
		panic("tonelliBig: non-residue")
	}
	// factor p-1 = q * 2^s
	var q big.Int
	q.Sub(p, b1)
	s := 0
	for q.Bit(0) == 0 {
		q.Rsh(&q, 1)
		s++
	}
	m := modBig{p}
	// find z non-residue
	z := big.NewInt(2)
	for legendreBig(z, p) != -1 {
		z.Add(z, b1)
	}
	c := m.pow(z, &q)
	// x = n^{(q+1)/2}
	var qp1 big.Int
	qp1.Add(&q, b1)
	qp1.Rsh(&qp1, 1)
	x := m.pow(n, &qp1)
	t := m.pow(n, &q)
	si := s
	one := big.NewInt(1)
	for t.Cmp(one) != 0 {
		// find i
		i := 1
		var t2i big.Int
		t2i.Mul(t, t)
		t2i.Mod(&t2i, p)
		for t2i.Cmp(one) != 0 {
			t2i.Mul(&t2i, &t2i)
			t2i.Mod(&t2i, p)
			i++
			if i == si {
				panic("tonelliBig: loop i reached s")
			}
		}
		// b = c^{2^{s-i-1}}
		exp := big.NewInt(1)
		exp.Lsh(exp, uint(si-i-1))
		b := m.pow(c, exp)
		x.Mul(x, b).Mod(x, p)
		b2 := new(big.Int).Mul(b, b)
		b2.Mod(b2, p)
		t.Mul(t, b2).Mod(t, p)
		c = b2
		si = i
	}
	return x
}

// ------------------- writer -------------------

type pointWriter interface {
	WriteU64(p PointU64) error
	WriteBig(p PointBig) error
	Close() error
}

type textWriter struct {
	bw *bufio.Writer
}

func newTextWriter(path string) (*textWriter, func(), error) {
	var f *os.File
	var err error
	if path == "-" {
		f = os.Stdout
	} else {
		f, err = os.Create(path)
		if err != nil {
			return nil, nil, err
		}
	}
	w := bufio.NewWriterSize(f, 4<<20) // 4 MB buffer
	closeFn := func() {
		w.Flush()
		if f != os.Stdout {
			f.Close()
		}
	}
	return &textWriter{bw: w}, closeFn, nil
}
func (w *textWriter) WriteU64(p PointU64) error {
	_, err := w.bw.WriteString(fmt.Sprintf("%d %d\n", p.X, p.Y))
	return err
}
func (w *textWriter) WriteBig(p PointBig) error {
	_, err := w.bw.WriteString(fmt.Sprintf("%s %s\n", p.X.String(), p.Y.String()))
	return err
}
func (w *textWriter) Close() error { return w.bw.Flush() }

// ------------------- sqrt table (uint64 fast path) -------------------

func buildSqrtTableU64(p uint64, workers int, store64 bool) (any, error) {
	// store64=false => []uint32 (p must fit in int and y<p<2^32)
	// store64=true  => []uint64
	plen := int(p)
	if int64(plen) < 0 || uint64(plen) != p {
		return nil, fmt.Errorf("p too large for slice length on this platform")
	}

	const u32sent = ^uint32(0)
	const u64sent = ^uint64(0)

	start := time.Now()
	log.Printf("building sqrt table with %d workers ...", workers)

	if !store64 {
		T := make([]uint32, plen)
		for i := range T {
			T[i] = u32sent
		}
		var wg sync.WaitGroup
		chunk := (p + uint64(workers) - 1) / uint64(workers)
		for w := 0; w < workers; w++ {
			s := uint64(w) * chunk
			e := s + chunk
			if e > p {
				e = p
			}
			if s >= e {
				continue
			}
			wg.Add(1)
			go func(a, b uint64) {
				defer wg.Done()
				for y := a; y < b; y++ {
					r := (y * y) % p
					// CAS first-wins
					for {
						old := atomic.LoadUint32(&T[r])
						if old != u32sent {
							break
						}
						if atomic.CompareAndSwapUint32(&T[r], u32sent, uint32(y)) {
							break
						}
					}
				}
			}(s, e)
		}
		wg.Wait()
		log.Printf("sqrt table ready in %v", time.Since(start))
		return T, nil
	}

	T := make([]uint64, plen)
	for i := range T {
		T[i] = u64sent
	}
	var wg sync.WaitGroup
	chunk := (p + uint64(workers) - 1) / uint64(workers)
	for w := 0; w < workers; w++ {
		s := uint64(w) * chunk
		e := s + chunk
		if e > p {
			e = p
		}
		if s >= e {
			continue
		}
		wg.Add(1)
		go func(a, b uint64) {
			defer wg.Done()
			for y := a; y < b; y++ {
				r := (y * y) % p
				for {
					old := atomic.LoadUint64(&T[r])
					if old != u64sent {
						break
					}
					if atomic.CompareAndSwapUint64(&T[r], u64sent, y) {
						break
					}
				}
			}
		}(s, e)
	}
	wg.Wait()
	log.Printf("sqrt table ready in %v", time.Since(start))
	return T, nil
}

// ------------------- enumeration: uint64 fast path -------------------

func enumerateU64(p, A, B uint64, mode Mode, maxMem uint64, outPath string, workers int) error {
	// Decide table layout
	store64 := p >= (1 << 32) // need 8B entries if y >= 2^32
	entryBytes := uint64(4)
	if store64 {
		entryBytes = 8
	}
	tableBytes := entryBytes * p
	autoMode := mode == ModeAuto

	if autoMode {
		if tableBytes <= maxMem*8/10 {
			mode = ModeTable
		} else {
			mode = ModeOnTheFly
		}
	}

	if mode == ModeTable && tableBytes > maxMem*8/10 {
		return fmt.Errorf("requested table mode needs ~%0.2f GB but max-mem allows ~%0.2f GB",
			float64(tableBytes)/(1<<30), float64(maxMem*8/10)/(1<<30))
	}

	w, closeFn, err := newTextWriter(outPath)
	if err != nil {
		return err
	}
	defer closeFn()

	log.Printf("p=%d A=%d B=%d mode=%v workers=%d", p, A, B, mode, workers)

	var Tany any
	if mode == ModeTable {
		Tany, err = buildSqrtTableU64(p, workers, store64)
		if err != nil {
			return err
		}
	}

	// work channel
	type job struct{ x0, x1 uint64 }
	jobs := make(chan job, workers*2)
	points := make(chan PointU64, 1<<16)

	// writer goroutine
	var wgW sync.WaitGroup
	wgW.Add(1)
	go func() {
		defer wgW.Done()
		for pt := range points {
			if err := w.WriteU64(pt); err != nil {
				log.Fatalf("write error: %v", err)
			}
		}
	}()

	// workers
	var wg sync.WaitGroup
	m := mod64{p}
	// unpack table
	var T32 []uint32
	var T64 []uint64
	const u32sent = ^uint32(0)
	const u64sent = ^uint64(0)
	if mode == ModeTable {
		if !store64 {
			T32 = Tany.([]uint32)
		} else {
			T64 = Tany.([]uint64)
		}
	}

	worker := func() {
		defer wg.Done()
		for jb := range jobs {
			x := jb.x0 % p
			x2 := m.mul(x, x)
			// f = x^3 + A*x + B
			f := m.add(m.add(m.mul(m.mul(x2, x), 1), m.mul(A, x)), B)

			for xx := jb.x0; xx < jb.x1; xx++ {
				if mode == ModeTable {
					if !store64 {
						y := T32[f]
						if y != u32sent {
							yy := uint64(y)
							points <- PointU64{X: x, Y: yy}
							if yy != 0 {
								points <- PointU64{X: x, Y: (p - yy) % p}
							}
						}
					} else {
						y := T64[f]
						if y != u64sent {
							points <- PointU64{X: x, Y: y}
							if y != 0 {
								points <- PointU64{X: x, Y: (p - y) % p}
							}
						}
					}
				} else { // on-the-fly
					leg := legendre64(f, p)
					if leg == 1 {
						y := tonelli64(f, p)
						points <- PointU64{X: x, Y: y}
						if y != 0 {
							points <- PointU64{X: x, Y: (p - y) % p}
						}
					} else if leg == 0 { // f==0
						points <- PointU64{X: x, Y: 0}
					}
				}
				// increment x, x2, f using finite-difference formula
				// delta = (3x^2 + 3x + 1 + A) mod p
				delta := m.add(m.add(m.add(m.mul(3, x2), m.mul(3, x)), 1), A)
				f = m.add(f, delta)
				x2 = m.add(x2, m.add(m.mul(2, x), 1))
				x = m.add(x, 1)
			}
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}

	// feed jobs
	const chunks = 1024
	chunk := (p + chunks - 1) / chunks
	for s := uint64(0); s < p; s += chunk {
		e := s + chunk
		if e > p {
			e = p
		}
		jobs <- job{x0: s, x1: e}
	}
	close(jobs)
	wg.Wait()
	close(points)
	wgW.Wait()

	// point at infinity marker:
	_ = w.WriteU64(PointU64{X: math.MaxUint64, Y: math.MaxUint64}) // prints -1 -1 if cast to signed; leave as big marker
	return nil
}

// ------------------- enumeration: big.Int fallback -------------------

func enumerateBig(p, A, B *big.Int, mode Mode, outPath string, workers int) error {
	// Only on-the-fly is viable (table would be absurd).
	if mode == ModeTable {
		return errors.New("table mode is not supported for big.Int p")
	}
	if mode == ModeAuto {
		mode = ModeOnTheFly
	}

	w, closeFn, err := newTextWriter(outPath)
	if err != nil {
		return err
	}
	defer closeFn()

	log.Printf("BIG mode p=%s A=%s B=%s workers=%d", p.String(), A.String(), B.String(), workers)

	type job struct {
		x0, x1 *big.Int // half-open
	}
	jobs := make(chan job, workers*2)
	points := make(chan PointBig, 1<<12)

	// writer
	var wgW sync.WaitGroup
	wgW.Add(1)
	go func() {
		defer wgW.Done()
		for pt := range points {
			if err := w.WriteBig(pt); err != nil {
				log.Fatalf("write error: %v", err)
			}
		}
	}()

	// worker
	var wg sync.WaitGroup
	mod := modBig{p: p}
	one := big.NewInt(1)
	two := big.NewInt(2)
	three := big.NewInt(3)

	worker := func() {
		defer wg.Done()
		for jb := range jobs {
			// x := x0
			x := new(big.Int).Set(jb.x0)
			// x2 := x*x mod p
			x2 := mod.mul(x, x)
			// f := x^3 + A*x + B
			f := mod.add(mod.add(mod.mul(mod.mul(x2, x), one), mod.mul(A, x)), B)
			for cmp := new(big.Int).Set(jb.x0); cmp.Cmp(jb.x1) < 0; cmp.Add(cmp, one) {
				leg := legendreBig(f, p)
				if leg == 1 {
					y := tonelliBig(f, p)
					points <- PointBig{X: new(big.Int).Set(x), Y: y}
					if y.Sign() != 0 {
						py := new(big.Int).Sub(p, y)
						points <- PointBig{X: new(big.Int).Set(x), Y: py}
					}
				} else if leg == 0 {
					points <- PointBig{X: new(big.Int).Set(x), Y: new(big.Int)}
				}
				// delta = (3x^2 + 3x + 1 + A) mod p
				d1 := mod.mul(three, x2)
				d2 := mod.mul(three, x)
				delta := mod.add(mod.add(mod.add(d1, d2), one), A)
				f = mod.add(f, delta)
				// x2 = x2 + (2x+1)
				t := mod.add(mod.mul(two, x), one)
				x2 = mod.add(x2, t)
				x = mod.add(x, one)
			}
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}

	// Partition [0, p) into ~1024 chunks
	total := new(big.Int).Set(p)
	chunks := big.NewInt(1024)
	chunk := new(big.Int).Add(new(big.Int).Quo(total, chunks), big.NewInt(1))
	for s := new(big.Int); s.Cmp(p) < 0; s.Add(s, chunk) {
		e := new(big.Int).Add(s, chunk)
		if e.Cmp(p) > 0 {
			e.Set(p)
		}
		jobs <- job{x0: new(big.Int).Set(s), x1: new(big.Int).Set(e)}
	}
	close(jobs)
	wg.Wait()
	close(points)
	wgW.Wait()

	// point at infinity marker:
	_ = w.WriteBig(PointBig{X: big.NewInt(-1), Y: big.NewInt(-1)})
	return nil
}

// ------------------- main -------------------

func main() {
	var (
		pStr       = flag.String("p", "", "prime modulus p (decimal string, required)")
		AStr       = flag.String("A", "0", "curve parameter A (decimal string)")
		BStr       = flag.String("B", "0", "curve parameter B (decimal string)")
		modeStr    = flag.String("mode", "auto", "mode: auto|table|onthefly")
		maxMemStr  = flag.String("max-mem", "48GB", "memory cap for auto/table decisions (e.g. 48GB, 500MB)")
		outPath    = flag.String("out", "-", "output file path, or - for stdout")
		workersInt = flag.Int("workers", 0, "number of workers (default GOMAXPROCS*4)")
	)
	flag.Parse()

	if *pStr == "" {
		log.Fatal("missing required --p")
	}
	p := mustParseBig(*pStr, "p")
	A := mustParseBig(*AStr, "A")
	B := mustParseBig(*BStr, "B")

	mode, err := parseMode(*modeStr)
	if err != nil {
		log.Fatal(err)
	}

	maxMemBytes, err := parseBytes(*maxMemStr)
	if err != nil {
		log.Fatalf("bad --max-mem: %v", err)
	}

	workers := *workersInt
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0) * 4
	}

	// Fast path if p fits in uint64 and p < 2^63 (we rely on 128/64 reductions anyway)
	if pu64, ok := fitsUint64(p); ok && pu64 < (1<<63) {
		Au64, okA := fitsUint64(A)
		Bu64, okB := fitsUint64(B)
		if !okA || !okB {
			log.Fatalf("A or B does not fit into uint64 while p does; supply values < 2^64")
		}

		// Estimate table memory (entry size chosen in enumerateU64)
		// Do a quick dry calculation to warn users in logs:
		entryBytes := uint64(4)
		if pu64 >= (1 << 32) {
			entryBytes = 8
		}
		tableBytes := entryBytes * pu64
		if mode == ModeAuto {
			log.Printf("auto-selecting mode (table bytes ≈ %.2f GB, cap=%.2f GB)",
				float64(tableBytes)/(1<<30), float64(maxMemBytes)/(1<<30))
		}
		if err := enumerateU64(pu64, Au64, Bu64, mode, maxMemBytes, *outPath, workers); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Fallback big.Int path (on-the-fly only)
	if mode == ModeTable {
		log.Fatal("mode=table is not supported when p does not fit in uint64")
	}
	if err := enumerateBig(p, A, B, mode, *outPath, workers); err != nil {
		log.Fatal(err)
	}
}
