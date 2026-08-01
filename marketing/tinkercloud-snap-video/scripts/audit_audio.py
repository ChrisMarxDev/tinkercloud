from __future__ import annotations

import math
import sys

import librosa
import numpy as np


for path in sys.argv[1:]:
    samples, sample_rate = librosa.load(path, sr=None, mono=True)
    peak = float(np.max(np.abs(samples)))
    rms = float(np.sqrt(np.mean(np.square(samples))))
    peak_db = 20 * math.log10(max(peak, 1e-12))
    rms_db = 20 * math.log10(max(rms, 1e-12))
    print(
        f"{path}: duration={len(samples) / sample_rate:.3f}s "
        f"peak={peak_db:.2f}dBFS rms={rms_db:.2f}dBFS"
    )
