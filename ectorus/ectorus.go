// ectorus: line-walk + torus-exclusion enumerator for E(F_p)
//
// Goal
//
//	Experimental stand‑alone tool that explores this idea:
//	Start from seed points on E: y^2 = x^3 + A x + B over F_p (p>3 prime),
//	generate tangents/secants to get new points, and for each processed line
//	exclude (mark as impossible) every other lattice point on that line in the
//	p×p torus except the (up to three) algebraic intersections with E.
//
// Build
//
//	go build -o bin/ectorus ./ectorus
//
// Run (examples)
//
//	./bin/ectorus -A 0 -B 1 -p 11 -grid   # tiny prime with explicit p×p grid
//	./bin/ectorus -A 2 -B 3 -p 101 -grid  # explicit grid up to ~p≈5000 is OK
//	./bin/ectorus -A 0 -B 7 -p 1009       # implicit (no full grid), still excludes by lines
//	./bin/ectorus -A 0 -B 1 -p 11 -json   # JSON output
//
// Flags
//
//	-A, -B, -p      : curve parameters (decimal or 0x-hex), p prime > 3
//	-grid           : enable explicit p×p bitsets for FOUND/EXCLUDED (memory ~ 2*p^2 bits)
//	-max_lines N    : safety cap on number of lines to process (default 0 = no cap)
//	-seed_x x       : optional x to try first when searching initial seed
//	-json           : emit JSON instead of human text
//	-count_first    : count #E(F_p) with Legendre scan to give a stopping target (O(p))
//
// Notes
//   - For large p, do NOT use -grid. The algorithm keeps an implicit list of processed
//     lines and their true intersections and can still avoid reconsidering many points.
//   - When p is modest (<= 4096-ish), -grid provides a vivid demonstration of the
//     exclusion idea — you can watch FOUND grow while EXCLUDED eats the plane.
//   - Complexity: each processed line touches O(p) lattice points if -grid is set.
//     This is an exploratory/experimental tool rather than an asymptotically faster
//     enumerator. It’s designed so you can measure how quickly exclusions shrink the
//     candidate space on real curves.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
)

func parseBig(s string) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		b, err := hex.DecodeString(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"))
		if err != nil {
			return nil, err
		}
		return new(big.Int).SetBytes(b), nil
	}
	z, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("cannot parse integer: %q", s)
	}
	return z, nil
}

func mod(a, p *big.Int) *big.Int {
	z := new(big.Int).Mod(a, p)
	if z.Sign() < 0 {
		z.Add(z, p)
	}
	return z
}

func addM(a, b, p *big.Int) *big.Int { return mod(new(big.Int).Add(a, b), p) }

func subM(a, b, p *big.Int) *big.Int { return mod(new(big.Int).Sub(a, b), p) }

func mulM(a, b, p *big.Int) *big.Int { return mod(new(big.Int).Mul(a, b), p) }

func negM(a, p *big.Int) *big.Int { return subM(new(big.Int), a, p) }

func invM(a, p *big.Int) (*big.Int, error) {
	if a.Sign() == 0 {
		return nil, errors.New("inverse of zero")
	}
	inv := new(big.Int).ModInverse(a, p)
	if inv == nil {
		return nil, errors.New("no inverse")
	}
	return inv, nil
}
func powM(a, e, p *big.Int) *big.Int { return new(big.Int).Exp(a, e, p) }

// Legendre symbol (a|p): -1,0,+1
func legendre(a, p *big.Int) int {
	A := mod(a, p)
	if A.Sign() == 0 {
		return 0
	}
	e := new(big.Int).Sub(p, big.NewInt(1))
	e.Rsh(e, 1)
	v := powM(A, e, p)
	if v.Cmp(big.NewInt(1)) == 0 {
		return 1
	}
	if v.Sign() == 0 {
		return 0
	}
	return -1 // v==p-1
}

// Tonelli–Shanks sqrt mod p (p odd prime)
func sqrtModP(a, p *big.Int) (*big.Int, error) {
	A := mod(a, p)
	if A.Sign() == 0 {
		return new(big.Int), nil
	}
	if legendre(A, p) != 1 {
		return nil, errors.New("non-residue")
	}
	// p ≡ 3 mod 4 shortcut
	if new(big.Int).And(new(big.Int).Sub(p, big.NewInt(3)), big.NewInt(3)).Cmp(big.NewInt(0)) == 0 {
		e := new(big.Int).Add(p, big.NewInt(1))
		e.Rsh(e, 2)
		return powM(A, e, p), nil
	}
	// factor p-1 = q*2^s
	q := new(big.Int).Sub(p, big.NewInt(1))
	s := 0
	for q.Bit(0) == 0 {
		q.Rsh(q, 1)
		s++
	}
	// find z non-residue
	z := new(big.Int).Set(big.NewInt(2))
	for legendre(z, p) != -1 {
		z.Add(z, big.NewInt(1))
	}
	c := powM(z, q, p)
	x := powM(A, new(big.Int).Rsh(new(big.Int).Add(q, big.NewInt(1)), 1), p)
	t := powM(A, q, p)
	m := s
	one := big.NewInt(1)
	for t.Cmp(one) != 0 {
		i := 1
		b := new(big.Int).Exp(t, big.NewInt(2), p)
		for i < m {
			if b.Cmp(one) == 0 {
				break
			}
			b.Exp(b, big.NewInt(2), p)
			i++
		}
		if i == m {
			return nil, errors.New("T–S failure")
		}
		// b = c^{2^{m-i-1}}
		b.Set(c)
		for j := 0; j < m-i-1; j++ {
			b.Exp(b, big.NewInt(2), p)
		}
		x = mulM(x, b, p)
		bb := mulM(b, b, p)
		t = mulM(t, bb, p)
		c = bb
		m = i
	}
	return x, nil
}

// ---------- curve & group law ----------

type Curve struct{ P, A, B *big.Int }

type Point struct {
	X, Y *big.Int
	Inf  bool
}

// isSingular reports whether Δ = -16(4A^3 + 27B^2) ≡ 0 mod p,
// i.e., the curve is singular over F_p. We just test 4A^3 + 27B^2 ≡ 0.
func (c Curve) isSingular() bool {
	p := c.P
	A2 := mulM(c.A, c.A, p)
	A3 := mulM(A2, c.A, p)
	term := addM(mulM(big.NewInt(4), A3, p), mulM(big.NewInt(27), mulM(c.B, c.B, p), p), p)
	return term.Sign() == 0
}

func (c Curve) on(Pt Point) bool {
	if Pt.Inf {
		return true
	}
	x3 := mulM(Pt.X, mulM(Pt.X, Pt.X, c.P), c.P)
	rhs := addM(addM(x3, mulM(c.A, Pt.X, c.P), c.P), c.B, c.P)
	y2 := mulM(Pt.Y, Pt.Y, c.P)
	return y2.Cmp(rhs) == 0
}

func (c Curve) neg(Pt Point) Point {
	if Pt.Inf {
		return Pt
	}
	return Point{X: new(big.Int).Set(Pt.X), Y: negM(Pt.Y, c.P)}
}

func (c Curve) add(P, Q Point) (Point, error) {
	p := c.P
	if P.Inf {
		return Q, nil
	}
	if Q.Inf {
		return P, nil
	}
	if P.X.Cmp(Q.X) == 0 {
		// P==±Q
		ysum := mod(new(big.Int).Add(P.Y, Q.Y), p)
		if ysum.Sign() == 0 {
			return Point{Inf: true}, nil
		} // P == -Q -> O
		// Doubling
		if P.Y.Sign() == 0 {
			return Point{Inf: true}, nil
		} // vertical tangent
		num := addM(mulM(big.NewInt(3), mulM(P.X, P.X, p), p), c.A, p)
		den := mulM(big.NewInt(2), P.Y, p)
		inv, err := invM(den, p)
		if err != nil {
			return Point{}, err
		}
		lam := mulM(num, inv, p)
		xr := subM(subM(mulM(lam, lam, p), P.X, p), Q.X, p)
		yr := subM(mulM(lam, subM(P.X, xr, p), p), P.Y, p)
		return Point{X: xr, Y: yr}, nil
	}
	// Secant
	num := subM(Q.Y, P.Y, p)
	den := subM(Q.X, P.X, p)
	inv, err := invM(den, p)
	if err != nil {
		return Point{}, err
	}
	lam := mulM(num, inv, p)
	xr := subM(subM(mulM(lam, lam, p), P.X, p), Q.X, p)
	yr := subM(mulM(lam, subM(P.X, xr, p), p), P.Y, p)
	return Point{X: xr, Y: yr}, nil
}

func (c Curve) double(P Point) (Point, error) { return c.add(P, P) }

// ---------- lines on the torus ----------

// Line: either non-vertical y = m x + c (mod p) or vertical x = v.
// For our use, all lines come from a tangent at P or a secant through P,Q.

type Line struct {
	Vertical bool
	M, C     *big.Int // y = M x + C
	V        *big.Int // x = V (if Vertical)
}

func (L Line) key() string {
	if L.Vertical {
		return "v:" + L.V.String()
	}
	return "m:" + L.M.String() + "|c:" + L.C.String()
}

// derive tangent line at P, or secant through P,Q
func lineThrough(c Curve, P Point, Q *Point) (Line, error) {
	p := c.P
	if Q == nil || (Q != nil && P.X.Cmp(Q.X) == 0 && mod(new(big.Int).Add(P.Y, Q.Y), p).Sign() != 0) {
		// Tangent at P
		if P.Y.Sign() == 0 { // vertical
			return Line{Vertical: true, V: new(big.Int).Set(P.X)}, nil
		}
		num := addM(mulM(big.NewInt(3), mulM(P.X, P.X, p), p), c.A, p)
		den := mulM(big.NewInt(2), P.Y, p)
		inv, err := invM(den, p)
		if err != nil {
			return Line{}, err
		}
		m := mulM(num, inv, p)
		cst := subM(P.Y, mulM(m, P.X, p), p)
		return Line{M: m, C: cst}, nil
	}
	// Secant through distinct P,Q
	if P.X.Cmp(Q.X) == 0 && mod(new(big.Int).Add(P.Y, Q.Y), p).Sign() == 0 { // vertical through P and -Q
		return Line{Vertical: true, V: new(big.Int).Set(P.X)}, nil
	}
	num := subM(Q.Y, P.Y, p)
	den := subM(Q.X, P.X, p)
	inv, err := invM(den, p)
	if err != nil {
		return Line{}, err
	}
	m := mulM(num, inv, p)
	cst := subM(P.Y, mulM(m, P.X, p), p)
	return Line{M: m, C: cst}, nil
}

// Compute the third intersection R for a tangent at P (Q=nil) or a secant through P,Q.
// Returns intersections slice (distinct affine points on this line that lie on E).
func thirdIntersection(c Curve, P Point, Q *Point) (Point, []Point, error) {
	if Q == nil {
		// Tangent
		R, err := c.double(P)
		if err != nil {
			return Point{}, nil, err
		}
		if R.Inf { // vertical tangent, only P and -P on the vertical line in affine chart
			return R, []Point{P, c.neg(P)}, nil
		}
		// Tangent meets at P (double) and R; both P and R are on the line; also -R by symmetry but not on same line generally.
		return R, []Point{P, R}, nil
	}
	// Secant
	if P.X.Cmp(Q.X) == 0 && mod(new(big.Int).Add(P.Y, Q.Y), c.P).Sign() == 0 {
		// vertical secant; third point is O (at infinity). Affine intersections: P and Q only.
		return Point{Inf: true}, []Point{P, *Q}, nil
	}
	R, err := c.add(P, *Q)
	if err != nil {
		return Point{}, nil, err
	}
	return R, []Point{P, *Q, R}, nil
}

// ---------- explicit p×p grid (optional) ----------

type Bitset struct {
	bits []uint64
	n    int
}

func newBitset(n int) *Bitset    { return &Bitset{bits: make([]uint64, (n+63)/64), n: n} }
func (b *Bitset) set(i int)      { b.bits[i>>6] |= 1 << (uint(i) & 63) }
func (b *Bitset) get(i int) bool { return (b.bits[i>>6]>>(uint(i)&63))&1 == 1 }

// Grid tracks FOUND and EXCLUDED points explicitly. Index = y*p + x.

type Grid struct {
	p           int
	found, excl *Bitset
}

func newGrid(p int) *Grid                { return &Grid{p: p, found: newBitset(p * p), excl: newBitset(p * p)} }
func (g *Grid) idx(x, y int) int         { return y*g.p + x }
func (g *Grid) markFound(x, y int)       { g.found.set(g.idx(x, y)) }
func (g *Grid) markExcl(x, y int)        { g.excl.set(g.idx(x, y)) }
func (g *Grid) isExcluded(x, y int) bool { return g.excl.get(g.idx(x, y)) }
func (g *Grid) isFound(x, y int) bool    { return g.found.get(g.idx(x, y)) }

// markLineExclusions excludes all points on L except those in keep map[key]=true
func (g *Grid) markLineExclusions(L Line, keep map[string]bool) {
	p := g.p
	if L.Vertical {
		x := int(new(big.Int).Set(L.V).Int64()) % p
		for y := 0; y < p; y++ {
			k := fmt.Sprintf("%d|%d", x, y)
			if keep[k] {
				continue
			}
			g.markExcl(x, y)
		}
		return
	}
	m := int(new(big.Int).Set(L.M).Int64()) % p
	c := int(new(big.Int).Set(L.C).Int64()) % p
	for x := 0; x < p; x++ {
		y := (m*x + c) % p
		if y < 0 {
			y += p
		}
		k := fmt.Sprintf("%d|%d", x, y)
		if keep[k] {
			continue
		}
		g.markExcl(x, y)
	}
}

// ---------- engine ----------

type Engine struct {
	C          Curve
	UseGrid    bool
	G          *Grid
	MaxLines   int
	CountFirst bool
	KnownCount *big.Int

	found       map[string]Point
	order       []Point         // NEW: discovery order
	indexOf     map[string]int  // NEW: for fast lookup if needed
	deadX       map[string]bool // x where both roots (or the single y=0 root) are already known
	linesDone   map[string]bool
	secantDone  map[string]bool // unordered pair key "x1|y1#x2|y2"
	tangentDone map[string]bool // by point key
}

func (e *Engine) pointKey(P Point) string {
	if P.Inf {
		return "inf"
	}
	return P.X.String() + "|" + P.Y.String()
}
func (e *Engine) pairKey(P, Q Point) string {
	k1 := e.pointKey(P)
	k2 := e.pointKey(Q)
	if k1 < k2 {
		return k1 + "#" + k2
	}
	return k2 + "#" + k1
}

func (e *Engine) addFound(P Point) bool {
	if !e.C.on(P) {
		return false
	}
	k := e.pointKey(P)
	if _, ok := e.found[k]; ok {
		return false
	}
	e.found[k] = P
	if !P.Inf {
		if e.indexOf == nil {
			e.indexOf = make(map[string]int)
		}
		e.indexOf[k] = len(e.order)
		e.order = append(e.order, P)
		if e.UseGrid {
			x := int(P.X.Int64()) % e.G.p
			if x < 0 {
				x += e.G.p
			}
			y := int(P.Y.Int64()) % e.G.p
			if y < 0 {
				y += e.G.p
			}
			e.G.markFound(x, y)
		}
	}
	return true
}

// process tangent at P or secant through P,Q; exclude other points on that line
func (e *Engine) processLineFrom(P Point, Q *Point) error {
	L, err := lineThrough(e.C, P, Q)
	if err != nil {
		return err
	}
	lk := L.key()
	if e.linesDone[lk] {
		return nil
	}
	R, inters, err := thirdIntersection(e.C, P, Q)
	if err != nil {
		return err
	}
	// Record found intersections
	for _, S := range inters {
		e.addFound(S)
	}
	if !R.Inf {
		e.addFound(e.C.neg(R))
	} // For walking we’ll also eventually see -R via other lines; optional.
	// Exclude rest of the line on explicit grid
	if e.UseGrid {
		keep := map[string]bool{}
		for _, S := range inters {
			if S.Inf {
				continue
			}
			x := int(S.X.Int64()) % e.G.p
			if x < 0 {
				x += e.G.p
			}
			y := int(S.Y.Int64()) % e.G.p
			if y < 0 {
				y += e.G.p
			}
			keep[fmt.Sprintf("%d|%d", x, y)] = true
		}
		e.G.markLineExclusions(L, keep)
	}
	e.linesDone[lk] = true
	return nil
}

// Linear pass over discovered points.
// For point i, process: (1) its tangent, (2) secants with j in [0..i-1].
func (e *Engine) walkAndExclude(maxLines int) error {
	processed := 0
	// start index at current length if this is a resume; else 0
	for i := 0; i < len(e.order); i++ {
		if maxLines > 0 && processed >= maxLines {
			break
		}

		P := e.order[i]
		pk := e.pointKey(P)

		// Tangent at P once
		if !e.tangentDone[pk] {
			if err := e.processLineFrom(P, nil); err != nil {
				return err
			}
			e.tangentDone[pk] = true
			processed++
		}

		// Secants P with all earlier points
		for j := 0; j < i; j++ {
			Q := e.order[j]
			if Q.Inf {
				continue
			}
			pair := e.pairKey(P, Q)
			if e.secantDone[pair] {
				continue
			}
			if err := e.processLineFrom(P, &Q); err != nil {
				return err
			}
			e.secantDone[pair] = true
			processed++
			if maxLines > 0 && processed >= maxLines {
				break
			}
		}

		// Early stop if we know point count
		if e.KnownCount != nil {
			finite := len(e.order)
			if new(big.Int).SetInt64(int64(finite)).
				Cmp(new(big.Int).Sub(e.KnownCount, big.NewInt(1))) == 0 {
				break
			}
		}
	}
	return nil
}

// findNextSeed: pick the next lattice point that is not excluded and (if on curve) not yet found.
// For implicit mode, we just random-search x until we get a new E point not in found.
func (e *Engine) findNextSeed() (Point, bool) {
	p := e.C.P
	tries := 0
	for tries < 200000 {
		x, _ := rand.Int(rand.Reader, p)
		kx := x.String()
		if e.deadX[kx] {
			tries++
			continue
		}

		t := addM(addM(mulM(x, mulM(x, x, p), p), mulM(e.C.A, x, p), p), e.C.B, p)
		lg := legendre(t, p)
		if lg == -1 {
			tries++
			continue
		}
		if lg == 0 {
			P := Point{X: x, Y: new(big.Int)}
			if _, ok := e.found[e.pointKey(P)]; !ok {
				return P, true
			}
			e.deadX[kx] = true
			tries++
			continue
		}
		y, err := sqrtModP(t, p)
		if err != nil {
			tries++
			continue
		}
		P1 := Point{X: x, Y: y}
		P2 := Point{X: x, Y: negM(y, p)}
		_, f1 := e.found[e.pointKey(P1)]
		_, f2 := e.found[e.pointKey(P2)]
		if !f1 {
			return P1, true
		}
		if !f2 {
			return P2, true
		}
		e.deadX[kx] = true
		tries++
	}
	return Point{}, false
}

// ---------- counting ----------

func countLegendre(c Curve) *big.Int {
	cnt := new(big.Int).Set(big.NewInt(1)) // include 0
	for x := new(big.Int).SetInt64(0); x.Cmp(c.P) < 0; x.Add(x, big.NewInt(1)) {
		t := addM(addM(mulM(x, mulM(x, x, c.P), c.P), mulM(c.A, x, c.P), c.P), c.B, c.P)
		lg := legendre(t, c.P)
		switch lg {
		case 0:
			cnt.Add(cnt, big.NewInt(1))
		case 1:
			cnt.Add(cnt, big.NewInt(2))
		}
	}
	return cnt
}

// ---------- output structs ----------

type Out struct {
	P          string   `json:"p"`
	A          string   `json:"A"`
	B          string   `json:"B"`
	KnownCount *big.Int `json:"pointCount,omitempty"`
	Complete   bool     `json:"complete"`
	Found      []Pt     `json:"found"`
	Lines      int      `json:"linesProcessed"`
	Notes      []string `json:"notes,omitempty"`
}

type Pt struct {
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
	Inf bool   `json:"inf"`
}

func toPt(P Point) Pt {
	if P.Inf {
		return Pt{Inf: true}
	}
	return Pt{X: P.X.String(), Y: P.Y.String()}
}

// ---------- main ----------

func main() {
	var AStr, BStr, PStr, seedXStr string
	var useGrid, jsonOut bool
	var maxLines int
	var countFirst bool

	flag.StringVar(&AStr, "A", "0", "curve A (dec or 0x-hex)")
	flag.StringVar(&BStr, "B", "0", "curve B (dec or 0x-hex)")
	flag.StringVar(&PStr, "p", "0", "prime p>3 (dec or 0x-hex)")
	flag.BoolVar(&useGrid, "grid", false, "use explicit p×p bitsets for found/excluded (memory ~ 2*p^2 bits)")
	flag.IntVar(&maxLines, "max_lines", 0, "cap number of lines processed (0 = no cap)")
	flag.BoolVar(&jsonOut, "json", false, "emit JSON")
	flag.BoolVar(&countFirst, "count_first", false, "count #E(F_p) first (Legendre scan) to know stopping target")
	flag.StringVar(&seedXStr, "seed_x", "", "optional x to try first when finding initial seed")
	flag.Parse()

	fmt.Fprintln(os.Stdout, "Parsing input parameters...")
	A, err := parseBig(AStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: parsing value for A")
		die(err)
	}
	B, err := parseBig(BStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: parsing value for B")
		die(err)
	}
	P, err := parseBig(PStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: parsing value for p")
		die(err)
	}

	fmt.Fprintln(os.Stdout, "Checking input parameters...")
	if P.Cmp(big.NewInt(3)) <= 0 {
		dieStr("p must be > 3")
	}
	if !P.ProbablyPrime(32) {
		fmt.Fprintln(os.Stderr, "warning: p may not be prime")
	}

	fmt.Fprintln(os.Stdout, "Creating curve...")
	curve := Curve{P: P, A: mod(A, P), B: mod(B, P)}
	// Early safety checks
	if curve.isSingular() {
		dieStr("singular curve: discriminant (4A^3+27B^2) ≡ 0 mod p")
	}
	if useGrid {
		fmt.Fprintln(os.Stdout, "Creating grid memory...")
		limit := big.NewInt(10_000)
		if P.Cmp(limit) > 0 {
			fmt.Fprintf(os.Stderr, "warning: -grid mode supports p ≤ %s; got p=%s. Exiting.", limit.String(), P.String())
			os.Exit(2)
		}
	}

	fmt.Fprintln(os.Stdout, "Creating engine...")
	eng := &Engine{C: curve, UseGrid: useGrid, MaxLines: maxLines, CountFirst: countFirst,
		found: map[string]Point{}, linesDone: map[string]bool{}, secantDone: map[string]bool{}, tangentDone: map[string]bool{}, indexOf: map[string]int{}, deadX: map[string]bool{}}
	if useGrid {
		pp := int(P.Int64())
		eng.G = newGrid(pp)
	}

	// Count first if requested (O(p))
	if eng.CountFirst {
		fmt.Fprintln(os.Stdout, "Counting points (Legendre)...")
		eng.KnownCount = countLegendre(curve)
	}

	// seed
	var seedX *big.Int
	if seedXStr != "" {
		fmt.Fprintln(os.Stdout, "Parsing seed...")
		sx, err := parseBig(seedXStr)
		if err != nil {
			die(err)
		}
		seedX = sx
	}
	seed, ok := eng.findNextSeedFromX(seedX)
	if !ok {
		dieStr("failed to find a seed point on E")
	}
	fmt.Fprintln(os.Stdout, "Found seed point on E...")
	eng.addFound(seed)

	// walk + exclude
	if err := eng.walkAndExclude(eng.MaxLines); err != nil {
		die(err)
	}

	// If not complete and we know count, keep sampling seeds until done
	linesProcessed := len(eng.linesDone)
	for eng.KnownCount != nil && !eng.isComplete() {
		next, ok := eng.findNextSeed()
		if !ok {
			break
		}
		eng.addFound(next)
		if err := eng.walkAndExclude(eng.MaxLines); err != nil {
			die(err)
		}
		linesProcessed = len(eng.linesDone)
	}

	// Collate output
	out := Out{P: P.String(), A: eng.C.A.String(), B: eng.C.B.String(), KnownCount: eng.KnownCount,
		Complete: eng.isComplete(), Lines: linesProcessed}
	for _, P := range eng.sortedFound() {
		out.Found = append(out.Found, toPt(P))
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(out)
		return
	}
	printHuman(out)
}

func (e *Engine) isComplete() bool {
	if e.KnownCount == nil {
		return false
	}
	finite := 0
	for _, P := range e.found {
		if !P.Inf {
			finite++
		}
	}
	return new(big.Int).SetInt64(int64(finite)).Cmp(new(big.Int).Sub(e.KnownCount, big.NewInt(1))) == 0
}

func (e *Engine) sortedFound() []Point {
	arr := make([]Point, 0, len(e.found))
	for _, P := range e.found {
		arr = append(arr, P)
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].Inf != arr[j].Inf {
			return !arr[i].Inf
		}
		if arr[i].X == nil || arr[j].X == nil {
			return false
		}
		cx := arr[i].X.Cmp(arr[j].X)
		if cx != 0 {
			return cx < 0
		}
		return arr[i].Y.Cmp(arr[j].Y) < 0
	})
	return arr
}

func (e *Engine) findNextSeedFromX(seedX *big.Int) (Point, bool) {
	fmt.Fprintln(os.Stdout, "Finding next seed from X...")
	p := e.C.P
	tryX := func(x *big.Int) (Point, bool) {
		t := addM(addM(mulM(x, mulM(x, x, p), p), mulM(e.C.A, x, p), p), e.C.B, p)
		lg := legendre(t, p)
		if lg == 0 {
			return Point{X: new(big.Int).Set(x), Y: new(big.Int)}, true
		}
		if lg == 1 {
			y, err := sqrtModP(t, p)
			if err == nil {
				return Point{X: new(big.Int).Set(x), Y: y}, true
			}
		}
		return Point{}, false
	}
	if seedX != nil {
		if P, ok := tryX(mod(seedX, p)); ok {
			return P, true
		}
	}
	for tries := 0; tries < 10000; tries++ {
		x, _ := rand.Int(rand.Reader, p)
		if P, ok := tryX(x); ok {
			return P, true
		}
	}
	return Point{}, false
}

func printHuman(o Out) {
	fmt.Printf("Curve: y^2 = x^3 + A x + B over F_p\nA = %s\nB = %s\np = %s\n\n", o.A, o.B, o.P)
	if o.KnownCount != nil {
		fmt.Printf("Point count (target): %s\n", o.KnownCount.String())
	}
	fmt.Printf("Lines processed: %d\n", o.Lines)
	fmt.Printf("Complete (matched target): %v\n\n", o.Complete)
	fmt.Println("Found points (affine first, then O if present):")
	for _, pt := range o.Found {
		if pt.Inf {
			fmt.Println("  O")
			continue
		}
		fmt.Printf("  (%s, %s)\n", pt.X, pt.Y)
	}
	if len(o.Notes) > 0 {
		fmt.Println("\nNotes:")
		for _, n := range o.Notes {
			fmt.Printf("  - %s\n", n)
		}
	}
}

func die(err error)   { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(2) }
func dieStr(s string) { fmt.Fprintln(os.Stderr, "error:", s); os.Exit(2) }
