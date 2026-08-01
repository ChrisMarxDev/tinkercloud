from __future__ import annotations

import sys

import av
import numpy as np


left = av.open(sys.argv[1])
right = av.open(sys.argv[2])
left_frames = left.decode(video=0)
right_frames = right.decode(video=0)

different = 0
maximum = 0
sum_absolute = 0
sample_count = 0
first_difference = None
largest_frame = None
largest_bbox = None

for index, (left_frame, right_frame) in enumerate(zip(left_frames, right_frames)):
    left_rgb = left_frame.to_ndarray(format="rgb24").astype(np.int16)
    right_rgb = right_frame.to_ndarray(format="rgb24").astype(np.int16)
    delta = np.abs(left_rgb - right_rgb)
    frame_max = int(delta.max())
    if frame_max:
        different += 1
        if first_difference is None:
            first_difference = index
        if frame_max > maximum:
            changed = np.any(delta > 0, axis=2)
            ys, xs = np.nonzero(changed)
            largest_frame = index
            largest_bbox = (
                int(xs.min()),
                int(ys.min()),
                int(xs.max()),
                int(ys.max()),
            )
    maximum = max(maximum, frame_max)
    sum_absolute += int(delta.sum())
    sample_count += delta.size

print(
    f"different_frames={different} first_difference={first_difference} "
    f"max_channel_delta={maximum} largest_frame={largest_frame} "
    f"largest_bbox={largest_bbox} "
    f"mean_absolute_delta={sum_absolute / sample_count:.8f}"
)
