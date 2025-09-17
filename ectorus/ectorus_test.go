package main

import (
	"math/big"
	"testing"
)

// ---------- helpers ----------

func bi(v int64) *big.Int { return big.NewInt(v) }

func mustCurve(t *testing.T, p, A, B int64) Curve {
	t.Helper()
	P := bi(p)
	return Curve{P: P, A: mod(bi(A), P), B: mod(bi(B), P)}
}

func pt(x, y int64) Point {
	return Point{X: bi(x), Y: bi(y)}
}

// count finite points recorded in Engine (excludes O)
func finiteCount(e *Engine) int {
	n := 0
	for _, P := range e.found {
		if !P.Inf {
			n++
		}
	}
	return n
}

// ---------- unit tests ----------

func TestParseBig(t *testing.T) {
	z, err := parseBig("12345")
	if err != nil || z.Cmp(bi(12345)) != 0 {
		t.Fatalf("parse dec failed: %v %v", z, err)
	}
	z, err = parseBig("0x2a")
	if err != nil || z.Cmp(bi(42)) != 0 {
		t.Fatalf("parse hex failed: %v %v", z, err)
	}
	z, err = parseBig(" 12345 ")
	if err != nil || z.Cmp(bi(12345)) != 0 {
		t.Fatalf("parse white space around dec failed: %v %v", z, err)
	}
}

func TestModOps(t *testing.T) {
	p := bi(11)
	if mod(bi(-1), p).Cmp(bi(10)) != 0 { // -1 ≡ 10 mod 11
		t.Fatal("mod neg wrong")
	}
	if addM(bi(8), bi(5), p).Cmp(bi(2)) != 0 { // (8 + 5) = 13 ≡ 2 mod 11
		t.Fatal("addM wrong")
	}
	if subM(bi(3), bi(5), p).Cmp(bi(9)) != 0 { // (3 - 5) = -2 ≡ 9 mod 11
		t.Fatal("subM wrong")
	}
	if mulM(bi(7), bi(5), p).Cmp(bi(2)) != 0 { // (7 x 5) = 35 ≡ 2 mod 11
		t.Fatal("mulM wrong")
	}
	inv, err := invM(bi(5), p)
	if err != nil || inv.Cmp(bi(9)) != 0 { // (5 * 9) = 45 ≡ 1 mod 11
		t.Fatalf("invM wrong: %v %v", inv, err)
	}
}

// non-residue -> an element a ∈ F_p for which there is no x ∈ F_p with 𝑥^2 ≡ 𝑎 (mod 𝑝)
func TestLegendreAndSqrt(t *testing.T) {
	p := bi(11)
	if legendre(bi(0), p) != 0 {
		t.Fatal("Legendre(0) != 0") // by definition
	}
	if legendre(bi(2), p) != -1 { // 2 is non-residue mod 11
		t.Fatal("Legendre(2) != -1")
	}
	if legendre(bi(4), p) != 1 {
		t.Fatal("Legendre(4) != 1") // 4 is residue mod 11, because (9 * 9) = 81 ≡ 4 mod 11
	}
	y, err := sqrtModP(bi(4), p)
	if err != nil {
		t.Fatalf("sqrtModP failed: %v", err)
	}
	if y.Cmp(big.NewInt(9)) != 0 {
		t.Fatalf("sqrtModP is wrong, should be 9, gives: %v", y) // (9 * 9) = 81 ≡ 4 mod 11
	}
	// y^2 == 4; the other root is p - y
	yy := mulM(y, y, p)
	if yy.Cmp(bi(4)) != 0 {
		t.Fatalf("sqrt square wrong: got %v", yy)
	}
}

func TestSingularCheck(t *testing.T) {
	// y^2 = x^3 (A=0,B=0) is singular over any p
	c := mustCurve(t, 11, 0, 0)
	if !c.isSingular() {
		t.Fatal("expected singular curve")
	}
	// y^2 = x^3 + 1 over p=11 is nonsingular (4A^3+27B^2 = 27 ≡ 5 != 0)
	c = mustCurve(t, 11, 0, 1)
	if c.isSingular() {
		t.Fatal("expected nonsingular curve")
	}
}

func TestOnNegAndAddBasics(t *testing.T) {
	c := mustCurve(t, 11, 0, 1) // y^2 = x^3 + 1
	P := pt(0, 1)               // 1^2 = 0^3 + 1
	if !c.on(P) {
		t.Fatal("P not on curve")
	}
	mP := c.neg(P)
	if mP.X.Cmp(P.X) != 0 || mP.Y.Cmp(bi(10)) != 0 { // -1 ≡ 10 mod 11
		t.Fatal("negation wrong")
	}
	O := Point{Inf: true}
	Q, _ := c.add(P, O)
	if !c.on(Q) || Q.X.Cmp(P.X) != 0 || Q.Y.Cmp(P.Y) != 0 {
		t.Fatal("P + O != P")
	}
	Q, _ = c.add(P, c.neg(P))
	if !Q.Inf {
		t.Fatal("P + (-P) != O")
	}
}

func TestDoubleVerticalAtYZero(t *testing.T) {
	c := mustCurve(t, 11, 0, 1)
	// y=0 ⇒ x^3 + 1 = 0 mod 11 ⇒ x^3 ≡ 10; x=10 works.
	P := pt(10, 0)
	if !c.on(P) {
		t.Fatal("point (10,0) should be on curve")
	}
	R, _ := c.double(P)
	if !R.Inf {
		t.Fatal("doubling y=0 should go to O (vertical tangent)")
	}
}

func TestLineThroughTangentAndSecant(t *testing.T) {
	c := mustCurve(t, 11, 0, 1)
	P := pt(0, 1)
	// Tangent at P (y ≠ 0) is non-vertical
	L, err := lineThrough(c, P, nil)
	if err != nil || L.Vertical {
		t.Fatalf("tangent should be non-vertical: %v %v", L, err)
	}
	// Secant with -P should be vertical
	mP := c.neg(P)
	L2, err := lineThrough(c, P, &mP)
	if err != nil || !L2.Vertical {
		t.Fatalf("secant P,-P should be vertical: %v %v", L2, err)
	}
	// Key stability
	if L.key() == "" || L2.key() == "" {
		t.Fatal("empty line key")
	}
}

func TestThirdIntersectionContracts(t *testing.T) {
	c := mustCurve(t, 11, 0, 1)
	P := pt(0, 1)
	// Tangent
	R, inter, err := thirdIntersection(c, P, nil)
	if err != nil {
		t.Fatalf("thirdIntersection tangent err: %v", err)
	}
	if len(inter) < 2 {
		t.Fatalf("tangent intersections expected ≥2, got %d", len(inter))
	}
	// Secant with -P → R = O
	mP := c.neg(P)
	R, inter, err = thirdIntersection(c, P, &mP)
	if err != nil {
		t.Fatalf("thirdIntersection secant err: %v", err)
	}
	if !R.Inf {
		t.Fatal("secant through P,-P should have R=O")
	}
	if len(inter) != 2 {
		t.Fatalf("expected 2 affine intersections on vertical secant, got %d", len(inter))
	}
}

func TestGridMarkLineExclusions(t *testing.T) {
	p := 11
	g := newGrid(p)
	// y = 2x + 3 mod 11
	L := Line{Vertical: false, M: bi(2), C: bi(3)}
	keep := map[string]bool{
		"0|3": true, // x=0 → y=3
	}
	g.markLineExclusions(L, keep)
	// All points on that line should be excluded except kept ones
	for x := 0; x < p; x++ {
		y := (2*x + 3) % p
		key := (fmtInt(x) + "|" + fmtInt(y))
		if keep[key] {
			if g.isExcluded(x, y) {
				t.Fatalf("kept point (%d,%d) marked excluded", x, y)
			}
		} else if !g.isExcluded(x, y) {
			t.Fatalf("point (%d,%d) on line should be excluded", x, y)
		}
	}
}

func fmtInt(x int) string { return big.NewInt(int64(x)).String() }

func TestEngineAddFoundInitializesIndexMap(t *testing.T) {
	c := mustCurve(t, 11, 0, 1)
	e := &Engine{
		C:           c,
		UseGrid:     false,
		found:       map[string]Point{},
		linesDone:   map[string]bool{},
		secantDone:  map[string]bool{},
		tangentDone: map[string]bool{},
		// indexOf intentionally left nil to exercise guard
	}
	P := pt(0, 1)
	if !e.addFound(P) {
		t.Fatal("addFound failed to insert P")
	}
	if e.indexOf == nil {
		t.Fatal("indexOf should have been initialized")
	}
	if len(e.order) != 1 || e.order[0].X.Cmp(bi(0)) != 0 {
		t.Fatal("order not updated")
	}
}

func TestEngineWalkSmallCurveCompletes(t *testing.T) {
	// Supersingular example: y^2 = x^3 + 1 over p=11 → #E(F_p) = p+1 = 12
	c := mustCurve(t, 11, 0, 1)
	e := &Engine{
		C:           c,
		UseGrid:     false,
		found:       map[string]Point{},
		linesDone:   map[string]bool{},
		secantDone:  map[string]bool{},
		tangentDone: map[string]bool{},
		indexOf:     map[string]int{},
		deadX:       map[string]bool{},
	}
	// count first to set a stopping target
	e.KnownCount = countLegendre(c) // expect 12
	if e.KnownCount.Cmp(bi(12)) != 0 {
		t.Fatalf("countLegendre expected 12, got %s", e.KnownCount.String())
	}
	// Seed deterministically: x=0 gives y=±1
	seed := Point{X: bi(0), Y: bi(1)}
	e.addFound(seed)
	if err := e.walkAndExclude(0); err != nil {
		t.Fatalf("walkAndExclude err: %v", err)
	}
	// Keep sampling seeds until complete (reuse engine path like main)
	for !e.isComplete() {
		next, ok := e.findNextSeed()
		if !ok {
			t.Fatal("failed to find next seed")
		}
		e.addFound(next)
		if err := e.walkAndExclude(0); err != nil {
			t.Fatalf("walk err: %v", err)
		}
	}
	if finiteCount(e) != int(e.KnownCount.Int64()-1) {
		t.Fatalf("finite points mismatch: got %d, want %d", finiteCount(e), e.KnownCount.Int64()-1)
	}
}

func TestFindNextSeedFromX(t *testing.T) {
	c := mustCurve(t, 11, 0, 1)
	e := &Engine{
		C:           c,
		UseGrid:     false,
		found:       map[string]Point{},
		linesDone:   map[string]bool{},
		secantDone:  map[string]bool{},
		tangentDone: map[string]bool{},
	}
	// Ask for x=0 explicitly
	Pt, ok := e.findNextSeedFromX(bi(0))
	if !ok || !e.C.on(Pt) {
		t.Fatalf("findNextSeedFromX failed: ok=%v P=%v", ok, Pt)
	}
}

func TestProcessLineFromSetsLinesDone(t *testing.T) {
	c := mustCurve(t, 11, 0, 1)
	e := &Engine{
		C:           c,
		UseGrid:     false,
		found:       map[string]Point{},
		linesDone:   map[string]bool{},
		secantDone:  map[string]bool{},
		tangentDone: map[string]bool{},
		indexOf:     map[string]int{},
	}
	P := pt(0, 1)
	e.addFound(P)
	if err := e.processLineFrom(P, nil); err != nil {
		t.Fatalf("processLineFrom tangent err: %v", err)
	}
	// Tangent line should be marked done
	L, err := lineThrough(c, P, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !e.linesDone[L.key()] {
		t.Fatal("line not marked done")
	}
}

func TestVerticalSecantThirdIsInfinity(t *testing.T) {
	c := mustCurve(t, 11, 0, 1)
	P := pt(0, 1)
	mP := c.neg(P)
	R, inters, err := thirdIntersection(c, P, &mP)
	if err != nil {
		t.Fatalf("thirdIntersection error: %v", err)
	}
	if !R.Inf {
		t.Fatalf("expected R=O for vertical secant, got %v", R)
	}
	if len(inters) != 2 {
		t.Fatalf("expected exactly P and -P intersections, got %d", len(inters))
	}
}

// --- add to ectorus_test.go ---

// Enumerate a few affine points on the curve (for small p) without using sqrtModP.
// This uses a O(p^2) scan, fine for tiny primes in tests.
func enumeratePoints(c Curve, max int) []Point {
	var pts []Point
	for x := new(big.Int).SetInt64(0); x.Cmp(c.P) < 0; x.Add(x, big.NewInt(1)) {
		// rhs = x^3 + A x + B
		rhs := addM(addM(mulM(x, mulM(x, x, c.P), c.P), mulM(c.A, x, c.P), c.P), c.B, c.P)
		// try all y to avoid any sqrtModP dependency
		for y := new(big.Int).SetInt64(0); y.Cmp(c.P) < 0; y.Add(y, big.NewInt(1)) {
			if mulM(y, y, c.P).Cmp(rhs) == 0 {
				pts = append(pts, Point{X: new(big.Int).Set(x), Y: new(big.Int).Set(y)})
				if len(pts) >= max {
					return pts
				}
			}
		}
	}
	return pts
}

func TestVerticalTangentLineThrough(t *testing.T) {
	// y^2 = x^3 + 1 over p=11 has (10,0); doubling should be O and tangent vertical.
	c := mustCurve(t, 11, 0, 1)
	P := pt(10, 0)
	L, err := lineThrough(c, P, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !L.Vertical || L.V.Cmp(bi(10)) != 0 {
		t.Fatalf("expected vertical tangent x=10, got %+v", L)
	}
	R, _ := c.double(P)
	if !R.Inf {
		t.Fatal("doubling y=0 should return O")
	}
}

func TestLineDeDupNoReprocess(t *testing.T) {
	c := mustCurve(t, 11, 0, 1)
	e := &Engine{
		C:           c,
		UseGrid:     false,
		found:       map[string]Point{},
		linesDone:   map[string]bool{},
		secantDone:  map[string]bool{},
		tangentDone: map[string]bool{},
		indexOf:     map[string]int{},
		deadX:       map[string]bool{},
	}
	// Seed and walk once
	e.addFound(pt(0, 1))
	if err := e.walkAndExclude(0); err != nil {
		t.Fatal(err)
	}
	lines := len(e.linesDone)
	// Walk again without new points — linesDone should not increase.
	if err := e.walkAndExclude(0); err != nil {
		t.Fatal(err)
	}
	if len(e.linesDone) != lines {
		t.Fatalf("lines reprocessed: before=%d after=%d", lines, len(e.linesDone))
	}
}

func TestGridVerticalLineExclusions(t *testing.T) {
	p := 11
	g := newGrid(p)
	L := Line{Vertical: true, V: bi(3)} // x = 3
	keep := map[string]bool{
		"3|0": true,
		"3|7": true,
	}
	g.markLineExclusions(L, keep)
	for y := 0; y < p; y++ {
		key := fmtInt(3) + "|" + fmtInt(y)
		if keep[key] {
			if g.isExcluded(3, y) {
				t.Fatalf("kept (%d,%d) marked excluded", 3, y)
			}
		} else if !g.isExcluded(3, y) {
			t.Fatalf("(%d,%d) on vertical should be excluded", 3, y)
		}
	}
}

func TestSqrtModP_Peq1mod4Branch(t *testing.T) {
	// p=13 (1 mod 4) forces Tonelli–Shanks branch; 10 is a residue mod 13.
	p := bi(13)
	if legendre(bi(10), p) != 1 {
		t.Fatal("sanity: 10 should be residue mod 13")
	}
	r, err := sqrtModP(bi(10), p)
	if err != nil {
		t.Fatalf("sqrtModP err: %v", err)
	}
	if mulM(r, r, p).Cmp(bi(10)) != 0 {
		t.Fatalf("r^2 != 10 mod 13, got %v", r)
	}
}

func TestSqrtModP_NonResidueErrors(t *testing.T) {
	p := bi(11)
	if _, err := sqrtModP(bi(2), p); err == nil {
		t.Fatal("expected error for non-residue")
	}
	// a=0 case: should return 0, no error
	r, err := sqrtModP(bi(0), p)
	if err != nil || r.Sign() != 0 {
		t.Fatalf("sqrt(0) failed: %v %v", r, err)
	}
}

func TestGroupBasics_CommuteNegateAssoc(t *testing.T) {
	c := mustCurve(t, 101, 1, 1) // small ordinary-ish curve
	pts := enumeratePoints(c, 5)
	if len(pts) < 3 {
		t.Fatal("need ≥3 points")
	}
	P, Q, R := pts[0], pts[1], pts[2]

	// Commutativity: P+Q == Q+P
	a, err := c.add(P, Q)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.add(Q, P)
	if err != nil {
		t.Fatal(err)
	}
	if a.Inf != b.Inf || (!a.Inf && (a.X.Cmp(b.X) != 0 || a.Y.Cmp(b.Y) != 0)) {
		t.Fatal("P+Q != Q+P")
	}

	// Negation involution: -(-P) == P
	if nn := c.neg(c.neg(P)); nn.Inf != P.Inf || (!nn.Inf && (nn.X.Cmp(P.X) != 0 || nn.Y.Cmp(P.Y) != 0)) {
		t.Fatal("negation not an involution")
	}

	// Associativity: (P+Q)+R == P+(Q+R)
	pq, err := c.add(P, Q)
	if err != nil {
		t.Fatal(err)
	}
	left, err := c.add(pq, R)
	if err != nil {
		t.Fatal(err)
	}
	qr, err := c.add(Q, R)
	if err != nil {
		t.Fatal(err)
	}
	right, err := c.add(P, qr)
	if err != nil {
		t.Fatal(err)
	}
	if left.Inf != right.Inf || (!left.Inf && (left.X.Cmp(right.X) != 0 || left.Y.Cmp(right.Y) != 0)) {
		t.Fatal("associativity failed")
	}
}

func TestWalkGridModeMarksSomeExclusions(t *testing.T) {
	// Tiny curve in grid mode: ensure we exclude *something* on first line.
	c := mustCurve(t, 11, 0, 1)
	e := &Engine{
		C:           c,
		UseGrid:     true,
		G:           newGrid(11),
		found:       map[string]Point{},
		linesDone:   map[string]bool{},
		secantDone:  map[string]bool{},
		tangentDone: map[string]bool{},
		indexOf:     map[string]int{},
		deadX:       map[string]bool{},
	}
	e.addFound(pt(0, 1))
	// Record excluded count before
	before := 0
	for y := 0; y < 11; y++ {
		for x := 0; x < 11; x++ {
			if e.G.isExcluded(x, y) {
				before++
			}
		}
	}
	if err := e.walkAndExclude(0); err != nil {
		t.Fatal(err)
	}
	after := 0
	for y := 0; y < 11; y++ {
		for x := 0; x < 11; x++ {
			if e.G.isExcluded(x, y) {
				after++
			}
		}
	}
	if after <= before {
		t.Fatalf("expected exclusions to increase: before=%d after=%d", before, after)
	}
}

func TestFindNextSeedFromX_YZero(t *testing.T) {
	// On y^2 = x^3 + 1 over p=11, x=10 gives y=0; make sure we get it.
	e := &Engine{C: mustCurve(t, 11, 0, 1)}
	P, ok := e.findNextSeedFromX(bi(10))
	if !ok || P.Inf || P.Y.Sign() != 0 || P.X.Cmp(bi(10)) != 0 {
		t.Fatalf("expected (10,0) seed, got ok=%v P=%+v", ok, P)
	}
}

func TestLineKeyUniqueness(t *testing.T) {
	c := mustCurve(t, 11, 0, 1)
	P := pt(0, 1)
	L1, err := lineThrough(c, P, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Recompute tangent; key must be identical
	L2, err := lineThrough(c, P, nil)
	if err != nil {
		t.Fatal(err)
	}
	if L1.key() != L2.key() {
		t.Fatalf("same line produced different keys: %s vs %s", L1.key(), L2.key())
	}
	// A different line should produce a different key
	Q := pt(1, 0) // not on this curve; pick another on-curve point
	// Find a second point on curve deterministically
	list := enumeratePoints(c, 3)
	if len(list) < 2 {
		t.Fatal("need 2 points")
	}
	Q = list[1]
	L3, err := lineThrough(c, P, &Q)
	if err != nil {
		t.Fatal(err)
	}
	if L1.key() == L3.key() {
		t.Fatalf("distinct lines share key: %s", L1.key())
	}
}
