from __future__ import annotations

import sys

import librosa
import numpy as np
from scipy.signal import butter, sosfilt


path = sys.argv[1]
window_beats = int(sys.argv[2]) if len(sys.argv) > 2 else 52

y, sr = librosa.load(path, sr=None, mono=True)
percussive = librosa.effects.hpss(y)[1]
_, detected_beats = librosa.beat.beat_track(
    y=percussive,
    sr=sr,
    tightness=400,
    units="time",
)

indices = np.arange(len(detected_beats))
matrix = np.vstack([indices, np.ones_like(indices)]).T
(interval, phase), *_ = np.linalg.lstsq(matrix, detected_beats, rcond=None)
residual = detected_beats - (phase + indices * interval)

kick_filter = butter(4, [40, 160], btype="band", fs=sr, output="sos")
kick = sosfilt(kick_filter, y)
onset = librosa.onset.onset_strength(y=kick, sr=sr)
onset_times = librosa.times_like(onset, sr=sr)

grid_count = int((len(y) / sr - phase) / interval)
grid_times = phase + np.arange(grid_count) * interval
grid_energy = np.array(
    [onset[np.argmin(np.abs(onset_times - beat_time))] for beat_time in grid_times]
)

windows: list[tuple[float, int, float, float]] = []
for start in range(0, max(0, grid_count - window_beats)):
    energy = grid_energy[start : start + window_beats]
    if len(energy) != window_beats:
        continue
    score = float(np.mean(energy) + np.percentile(energy, 75) * 0.35)
    windows.append(
        (
            score,
            start,
            float(grid_times[start]),
            float(grid_times[start + window_beats]),
        )
    )

print(
    f"BPM={60 / interval:.6f} phase={phase:.6f}s "
    f"interval={interval:.6f}s residual_max_ms={np.max(np.abs(residual)) * 1000:.2f}"
)
print(f"duration={len(y) / sr:.3f}s sample_rate={sr} grid_beats={grid_count}")
print("top_windows:")
for score, start, start_time, end_time in sorted(windows, reverse=True)[:12]:
    print(
        f"  beat={start:3d} start={start_time:9.6f}s "
        f"end={end_time:9.6f}s duration={end_time - start_time:7.3f}s "
        f"score={score:.4f}"
    )

print("top_hits:")
for beat_index in np.argsort(grid_energy)[::-1][:24]:
    print(
        f"  beat={int(beat_index):3d} time={grid_times[beat_index]:9.6f}s "
        f"energy={grid_energy[beat_index]:.4f}"
    )
