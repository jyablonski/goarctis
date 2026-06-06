# GPU Thermal Guard — Design

Status: **implemented**
Scope: NVIDIA GPUs only (via NVML)
Default: **off** — opt-in, requires root

## Summary

Today the system monitor is entirely passive: it reads CPU/memory/temperature
data and surfaces it in the tray. The GPU Thermal Guard adds a single, bounded,
reversible control action — when an NVIDIA GPU crosses a temperature threshold,
reduce its power-management limit; when it cools back down, restore the card's
default limit. The feature is opt-in via a CLI flag and falls back to pure
monitoring when it cannot actuate (e.g. no root).

Non-goals: undervolting, clock locking, fan-curve control, CPU throttling,
multi-step/PID control. Those are explicitly out of scope for this first pass.

## Why power-limit clamping (and nothing more)

- `SetPowerManagementLimit` is a first-class, vendor-supported NVML API. We
  already read its full envelope every poll: `DefaultPowerLimitW`,
  `MinPowerLimitW`, `MaxPowerLimitW`, and current `PowerLimitW`.
- It is inherently bounded — we never write outside the card's reported
  `[min, max]` constraints.
- It is session-only and self-healing: power limits do not persist across a
  reboot, so even a hard crash leaves no permanent change.
- Undervolting and clock manipulation on Linux have no clean NVML API, need
  per-vendor tooling, and carry real stability/safety risk. Deliberately excluded.

## Decisions (locked)

| Decision        | Choice                                                        |
| --------------- | ------------------------------------------------------------- |
| Clamp target    | Single step to a **fraction of the default limit** (0.80), bounded by `[min, max]` |
| Config surface  | **A single `--gpu-thermal-guard` enable flag**; thresholds are fixed package defaults |
| Default state   | Off; enabling requires root to actuate                        |
| Revert behavior | Hysteresis release during normal cooling **and** restore-on-exit |

> Note: the config surface was deliberately reduced to one flag for simplicity.
> `GovernorConfig` still carries the thresholds as fields, and
> `DefaultGovernorConfig(enabled bool)` is the single seam the CLI uses, so
> per-value tuning flags can be layered back on later without reworking the
> governor.

## Architecture

### Where the control logic lives

The governor reconciles **inside `NVIDIASampler.collectNVMLGPU`**, at the seam
where the live `nvml.Device` handle and the freshly-read readings already exist.

```
NVIDIASampler.Sample()  ──per device──>  collectNVMLGPU(index, device)
                                              │ reads temp, default/min/max limits
                                              ▼
                                       governor.Reconcile(device, stats)   ← decides + actuates
                                              │
                                              ▼
                                   next Sample() reads back new PowerLimitW → tray reflects it
```

Rationale for this seam over a separate `system.State` consumer:

- No handle re-resolution — the actuator uses the same `nvml.Device` already in
  hand.
- No need to distinguish NVML-backed GPUs from hwmon-backed ones (they are only
  distinguishable by `GPUStats.ID` format).
- No second enumeration; reconciliation runs at the existing 2s poll cadence.
- The tray automatically reflects the clamped limit on the next poll because
  `PowerLimitW` is re-read every cycle.

### The actuator — no new interface plumbing

`nvml.Device` already exposes `SetPowerManagementLimit(uint32) nvml.Return`, and
the mock already in use (`nvmlmock.Device`) provides `SetPowerManagementLimitFunc`.

- **No change** to the `nvmlLib` interface.
- **No new hardware path** in tests — the existing mock fakes the write.

### State machine (hysteresis)

The "revert to defaults during normal operation" requirement *is* hysteresis:
two thresholds with a gap so the limit cannot oscillate.

```go
type GovernorConfig struct {
	Enabled        bool
	ClampTempC     float64 // start clamping at/above this (default 85)
	ReleaseTempC   float64 // restore default at/below this (default 75)
	TargetFraction float64 // clamp target as fraction of default (default 0.80)
}

type clampRecord struct {
	device    nvml.Device // valid until our Shutdown — used for restore
	uuid      string
	defaultMW uint32
	appliedMW uint32
}

type GPUGovernor struct {
	mu       sync.Mutex
	cfg      GovernorConfig
	clamped  map[string]clampRecord // keyed by UUID
	disabled bool                   // tripped on permission / not-supported
}
```

```go
func (g *GPUGovernor) Reconcile(device nvml.Device, stats GPUStats) {
	if !g.cfg.Enabled || g.disabled ||
		stats.TemperatureC == nil || stats.DefaultPowerLimitW == nil {
		return
	}
	temp := *stats.TemperatureC
	_, clamped := g.clamped[stats.ID]

	switch {
	case !clamped && temp >= g.cfg.ClampTempC:
		g.clampLocked(device, stats)   // default -> target, within [min, max]
	case clamped && temp <= g.cfg.ReleaseTempC:
		g.restoreLocked(stats.ID)      // back to default
	}
	// in-band (release < temp < clamp): hold current state, do nothing
}
```

Reconcile logic, in words:

```
not clamped & temp >= 85  ->  Set limit = clamp(default * 0.80, min, max)   [record default]
clamped     & temp <= 75  ->  Set limit = default                           [drop record]
otherwise                 ->  hold
```

## Safety properties

1. **Bounded writes.** The clamp target is always
   `clamp(default * fraction, minMW, maxMW)`. We never write outside the card's
   reported constraints, even with a misconfigured fraction.

2. **Restore guaranteed two ways.**
   - Hysteresis release during normal cooldown (the common path).
   - `RestoreAll()` invoked from a new `NVIDIASampler.Close()`, hooked into the
     existing `cleanup()` / `onExit()` path, so quitting un-clamps every GPU
     before `nvml.Shutdown()`.
   - A hard crash leaves only a session-only limit, cleared on next reboot.

3. **Graceful self-disable.** `SetPowerManagementLimit` returns
   `ERROR_NO_PERMISSION` without root (and `ERROR_NOT_SUPPORTED` on some cards).
   On the first such failure: set `disabled = true`, log a one-time warning, and
   fall back to pure monitoring — the same degradation pattern `NVIDIASampler`
   already uses for init failure.

4. **Input validation.** Reject configs where `release >= clamp` or
   `frac <= 0 || frac > 1`: warn and disable rather than actuate on garbage.

## Configuration

A single CLI flag, following the existing `--disable-*` convention:

```
--gpu-thermal-guard           enable (default off; needs root)
```

The fixed thresholds it applies (package constants in `governor.go`):

```
DefaultClampTempC     = 85     clamp at/above this °C
DefaultReleaseTempC   = 75     restore default at/below this °C  (hysteresis gap)
DefaultTargetFraction = 0.80   clamp target = default * frac, bounded by [min, max]
```

- `--gpu-thermal-guard` is a `flag.Bool`. `main.go` calls
  `system.DefaultGovernorConfig(enabled)` to build the config from the constants
  above — the one place tuning flags would be reintroduced if ever needed.
- The config flows `main.go` -> `NewMonitorWithConfig` -> `NewProcSamplerWithConfig`
  -> `NewDefaultThermalSamplerWithConfig` -> `NewNVIDIASamplerWithConfig`, which
  constructs the governor only when enabled.

## Privilege boundary

The app runs unprivileged today. `SetPowerManagementLimit` needs root
(`CAP_SYS_ADMIN`). The **code is identical** regardless; this is a packaging
decision for whoever opts in:

- run the binary (or a thin helper) with the capability, or
- a systemd unit with `AmbientCapabilities=CAP_SYS_ADMIN`, or
- `sudo`.

Keeping the feature default-off means the unprivileged common case is completely
untouched.

## Testing

Unit tests drive the full state machine against `nvmlmock.Device`
(`SetPowerManagementLimitFunc`) — **no hardware required**:

- clamp fires exactly once when temp first crosses 85 °C;
- holds (no extra writes) while in-band;
- restores default at/below 75 °C;
- `RestoreAll()` un-clamps every recorded GPU;
- `ERROR_NO_PERMISSION` trips self-disable and stops further writes;
- invalid config (`release >= clamp`, bad fraction) disables without actuating.

**Not covered by automated tests:** the real `SetPowerManagementLimit` write.
That requires an NVIDIA GPU + root. Manual verification:

```
# observe the limit before/after crossing the threshold under load
nvidia-smi -q -d POWER        # note "Power Limit"
# drive temperature up (e.g. gpu-burn) until it passes 85 °C
nvidia-smi -q -d POWER        # limit should drop to ~80% of default
# let it cool below 75 °C
nvidia-smi -q -d POWER        # limit should return to default
```

## Implementation map

- **`pkg/system/governor.go`** — `GPUGovernor`, `GovernorConfig`,
  `DefaultGovernorConfig`, and the `Reconcile` / `clampLocked` / `restoreLocked`
  / `RestoreAll` logic.
- **`pkg/system/nvidia.go`** — `NewNVIDIASamplerWithConfig` constructs the
  governor when enabled, `Sample` calls `Reconcile` per device in the collect
  loop, and `Close()` restores clamps before `nvml.Shutdown`.
- **`pkg/system/{thermal,proc,monitor}.go`** — `*WithConfig` constructors thread
  the config down, and each exposes `Close()` so `Monitor.Stop()` restores via
  `io.Closer`.
- **`pkg/system/governor_test.go`** — the state-machine tests above.
- **`cmd/goarctis/main.go`** — the `--gpu-thermal-guard` flag,
  `DefaultGovernorConfig`, and `NewMonitorWithConfig` wiring (restore happens
  through the existing `cleanup()` -> `Stop()` path).
- **Docs** — `README.md` and `docs/how_it_works.md`: the flag and the root
  requirement.

## Open questions / future work

- Progressive (multi-step or PID) control instead of a single clamp step.
- Extend the same opt-in, reversible, bounded pattern to CPU
  (`scaling_max_freq` via `intel_pstate` / `amd_pstate`).
- Surface guard status in the tray (e.g. a "⚡ clamped" indicator) so the action
  is visible to the user, not silent.
