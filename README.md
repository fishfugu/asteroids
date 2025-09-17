# Asteroids

**Torus‑exclusion line walk for elliptic curves over finite fields**

> A tiny, readable tool to explore enumerating points on
> $E: y^2 = x^3 + A x + B$ over $\mathbb F_p$ by walking tangents/secants and, optionally,
> excluding every *other* lattice point along those lines on the $p\times p$ torus.
>
> ⚠️ Not production crypto. Not the fastest possible. It’s meant to be simple to read and
> easy to tinker with.

---

## What’s here

* `ectorus/` — a stand‑alone Go program that:

  * finds a seed point on $E$ (random‑x + Legendre + Tonelli–Shanks),
  * generates tangents/secants, computes their third intersections via the group law,
  * (optionally) in **grid mode** marks all other lattice points on those lines as **excluded**,
  * keeps going until done (or a cap), and can use a counted target to know when to stop.
* **Two operating modes**

  * **Implicit mode** (default): no $p^2$ grid. Deduplicates by line keys and point/pair sets. Good for medium/large $p$.
  * **Grid mode** (`-grid`): explicit FOUND/EXCLUDED bitsets over the $p\times p$ plane. Great for tiny $p$ to *see* exclusions.
* **Safety checks**

  * Rejects **singular curves** (discriminant $4A^3+27B^2\equiv 0\pmod p$).
  * In grid mode, warns and exits if `p > 10000` (RAM/overflow safety margin).

---

## Quick start

### Build

```bash
# clone this repo then:
cd ectorus
go build -o bin/ectorus ./ectorus
```

### Run

```bash
# small demo with explicit grid
./bin/ectorus -A 0 -B 1 -p 11 -grid -count_first

# medium p, implicit mode (no grid)
./bin/ectorus -A 0 -B 7 -p 1009 -count_first

# JSON output for scripting
./bin/ectorus -A 0 -B 1 -p 11 -count_first -json
```

**Flags**

* `-A, -B, -p` — curve parameters (decimal or `0x…` hex), with prime `p > 3`.
* `-grid` — enable explicit grid (FOUND/EXCLUDED bitsets). Memory ≈ `p^2/4` bytes.
* `-max_lines N` — cap how many lines to process (tangents + secants).
* `-seed_x x` — try this x first when searching a seed point.
* `-count_first` — compute $\\#E(\mathbb F_p)$ by a simple **Legendre scan** ($O(p)$) to give a precise stopping target.
* `-json` — JSON output (fields: `p, A, B, pointCount, complete, found[], linesProcessed`).

**Current limits**

* Implicit mode: no hard size limit — big.Int throughout. (Enumerating all points is still $\Theta(p)$.)
* Grid mode: enforced `p ≤ 10000`.

---

## What it does

1. **Seed**: pick random $x$, compute $t = x^3 + A x + B$. If $t$ is a square (Legendre), extract $y$ (Tonelli–Shanks). That gives a point $P\in E(\mathbb F_p)$.
2. **Lines**: for each known point (tangent) and each pair (secant), build the line modulo $p$:

   * tangent slope: $\lambda = (3x_P^2 + A) / (2y_P)$,
   * secant slope: $\lambda = (y_Q - y_P)/(x_Q - x_P)$,
   * vertical handled specially.
3. **Third intersection**: use group‑law formulas to get the third intersection $R$ and add the corresponding point $P\!\oplus Q$ (or $2P$).
4. **Deduplicate**: we key lines (`y ≡ m x + c` or `x ≡ v`), keyed point pairs, and points found — so we don’t redo work.
5. **Grid mode only**: rasterise the line on the $p\times p$ torus and mark all *other* lattice points as excluded (keep the true intersections).

This is **exploratory**: you can watch how fast exclusions shrink the candidate space, and compare implicit vs grid behavior.

---

## Benchmark helper (companion tool)

A tiny harness that shells out to the `ectorus` binary, runs a few scenarios, parses its `-json` output, and prints timings.

### Use the bench tool

```bash
# build the main tool and the bench
go build -o bin/ectorus ./ectorus
go build -o bin/bench   ./cmd

# run all built‑in scenarios once each (best‑of‑1)
./bin/bench -ectorus ./bin/ectorus

# repeat 3× and report the best time
./bench -ectorus ./ectorus -reps 3
```

---

## Design choices & trade‑offs

* **Why two modes?** A $p\times p$ grid is great for intuition and demos, but memory grows as $\Theta(p^2)$. Implicit mode avoids that by never materialising candidates; it deduplicates lines/points algebraically.
* **Counting first**: Knowing $N=\\#E(\mathbb F_p)$ gives a clean stop rule (have $N-1$ finite points). We currently use a simple Legendre scan ($O(p)$); swapping in SEA later would give polylog counting.
* **Not a faster‑than‑$O(p)$ enumerator**: listing $\sim p$ points inherently costs $\Theta(p)$. This project is about clarity and experimentation, not asymptotics.

---

## Roadmap (nice‑to‑haves)

* Replace `-count_first` scan with **SEA** (Schoof–Elkies–Atkin) backend for large primes.
* Factor into packages: `internal/ec` (field+group ops), `internal/torus` (line/exclusion), `cmd/ectorus`.
* Progress meter & stats (lines/sec, inversions count, etc.).
* Deterministic PRNG seed for reproducible runs.
* Unit tests for edge cases (verticals, y=0, duplicate lines, etc.).

---

## ecscan — Elliptic-curve point enumerator (parallel)

`ecscan` enumerates all affine points on the curve
\[
y^2 \equiv x^3 + A x + B \pmod p
\]
and streams them to a file or stdout. It parallelises across goroutines and
auto-selects the fastest mode for your RAM cap.

### Build

```bash
go build -o bin/ecscan ./cmd/ecscan
./bin/ecscan --p=<prime> --A=<A> --B=<B> \
  [--mode=auto|table|onthefly] \
  [--max-mem=48GB] \
  [--out=points.txt] \
  [--workers=N]
```

--mode=auto (default): uses a sqrt table if it fits under ~80% of --max-mem, otherwise on-the-fly.

--mode=table: refuses to run if the estimated table (p * (4 or 8 bytes)) exceeds ~80% of --max-mem.

--mode=onthefly: Euler check + Tonelli–Shanks per quadratic residue.

--out: file path or - for stdout.

--workers: defaults to GOMAXPROCS*4.

```
# small p, likely table mode
./bin/ecscan --p=101 --A=2 --B=3 --max-mem=48GB --out=-

# larger p with auto decision
./bin/ecscan --p=10000019 --A=2 --B=3 --max-mem=48GB --out=points.txt

# force table mode; will exit with a helpful error if it won't fit
./bin/ecscan --p=10000019 --A=2 --B=3 --mode=table --max-mem=48GB

# huge p beyond uint64: supported via big.Int path (onthefly only)
./bin/ecscan --p=1000000000000000000000003 --A=2 --B=3 --mode=onthefly --out=-
```

Implementation notes:

The CLI is a thin wrapper over internal/ecscan (ParseFlags + Run).

Run decides the mode and calls a parallel engine that:

Table mode: builds a sqrt table of y^2 mod p and scans x (linear time).

On-the-fly: uses Legendre to skip non-residues and Tonelli–Shanks to recover y.

Output: newline-delimited x y pairs; a final sentinel marks the point at infinity.

## License & attribution

MIT

This is an informal side project intended for teaching/experimentation - and to help with my own understanding of these ideas - nothing more.

---

## FAQ

* **Does grid mode exclude the same points if I use slope `M/N` (from the formal derivative of the equation of the curve) or its reduced field slope `R`?** Yes. They define the same line in $\mathbb F_p$; our line key canonicalises to `y ≡ m x + c` (or vertical), so they’re identical.
* **Why does a point with `y=0` double to infinity?** That’s the 2‑torsion case; the tangent is vertical, so $2P=\mathcal O$.
* **What happens on singular curves?** We fail early (discriminant test) because the group law formulas break at nodes/cusps.
