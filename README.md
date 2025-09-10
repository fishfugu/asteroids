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

* `-A, -B, -p` — curve parameters (decimal or `0x…` hex), wit
