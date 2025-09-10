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
go build -o ectorus .
```

### Run

```bash
# small demo with explicit grid
./ectorus -A 0 -B 1 -p 11 -grid -count_first

# medium p, implicit mode (no grid)
./ectorus -A 0 -B 7 -p 1009 -count_first

# JSON output for scripting
./ectorus -A 0 -B 1 -p 11 -count_first -json
```

**Flags**

* `-A, -B, -p` — curve parameters (decimal or `0x…` hex), with prime `p > 3`.
* `-grid` — enable explicit grid (FOUND/EXCLUDED bitsets). Memory ≈ `p^2/4` bytes.
* `-max_lines N` — cap how many lines to process (tangents + secants).
* `-seed_x x` — try this x first when searching a seed point.
* `-count_first` — compute $\#E(\mathbb F_p)$ by a simple **Legendre scan** ($O(p)$) to give a precise stopping target.
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
go build -o ectorus ./ectorus
go build -o bench ./cmd/bench

# run all built‑in scenarios once each (best‑of‑1)
./bench -ectorus ./ectorus

# repeat 3× and report the best time
./bench -ectorus ./ectorus -reps 3
```

---

## Design choices & trade‑offs

* **Why two modes?** A $p\times p$ grid is great for intuition and demos, but memory grows as $\Theta(p^2)$. Implicit mode avoids that by never materialising candidates; it deduplicates lines/points algebraically.
* **Counting first**: Knowing $N=\#E(\mathbb F_p)$ gives a clean stop rule (have $N-1$ finite points). We currently use a simple Legendre scan ($O(p)$); swapping in SEA later would give polylog counting.
* **Not a faster‑than‑$O(p)$ enumerator**: listing $\sim p$ points inherently costs $\Theta(p)$. This project is about clarity and experimentation, not asymptotics.

---

## Roadmap (nice‑to‑haves)

* Replace `-count_first` scan with **SEA** (Schoof–Elkies–Atkin) backend for large primes.
* Factor into packages: `internal/ec` (field+group ops), `internal/torus` (line/exclusion), `cmd/ectorus`.
* Progress meter & stats (lines/sec, inversions count, etc.).
* Deterministic PRNG seed for reproducible runs.
* Unit tests for edge cases (verticals, y=0, duplicate lines, etc.).

---

## License & attribution

MIT

This is an informal side project intended for teaching/experimentation - and to help with my own understanding of these ideas - nothing more.

---

## FAQ

* **Does grid mode exclude the same points if I use slope `M/N` or its reduced field slope `R`?** Yes. They define the same line in $\mathbb F_p$; our line key canonicalises to `y ≡ m x + c` (or vertical), so they’re identical.
* **Why does a point with `y=0` double to infinity?** That’s the 2‑torsion case; the tangent is vertical, so $2P=\mathcal O$.
* **What happens on singular curves?** We fail early (discriminant test) because the group law formulas break at nodes/cusps.
