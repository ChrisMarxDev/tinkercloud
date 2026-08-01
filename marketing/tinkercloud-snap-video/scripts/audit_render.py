from __future__ import annotations

import hashlib
import math
import sys

import av
import numpy as np


for path in sys.argv[1:]:
    container = av.open(path)
    video_stream = container.streams.video[0]
    audio_stream = container.streams.audio[0]

    video_hash = hashlib.sha256()
    video_frames = 0
    for frame in container.decode(video=0):
        video_hash.update(frame.to_ndarray(format="rgb24").tobytes())
        video_frames += 1

    container.close()
    container = av.open(path)
    audio_blocks: list[np.ndarray] = []
    for frame in container.decode(audio=0):
        block = frame.to_ndarray().astype(np.float64)
        if np.issubdtype(frame.to_ndarray().dtype, np.integer):
            info = np.iinfo(frame.to_ndarray().dtype)
            block /= max(abs(info.min), info.max)
        audio_blocks.append(block.reshape(-1))

    samples = np.concatenate(audio_blocks)
    peak = float(np.max(np.abs(samples)))
    rms = float(np.sqrt(np.mean(np.square(samples))))
    peak_db = 20 * math.log10(max(peak, 1e-12))
    rms_db = 20 * math.log10(max(rms, 1e-12))

    print(
        f"{path}: video_frames={video_frames} "
        f"video_sha256={video_hash.hexdigest()} "
        f"audio_peak={peak_db:.2f}dBFS audio_rms={rms_db:.2f}dBFS "
        f"sample_rate={audio_stream.rate}"
    )
