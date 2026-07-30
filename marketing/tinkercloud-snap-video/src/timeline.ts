export const FPS = 30;
export const BEAT_SECONDS = 0.461558;
export const BEAT_FRAMES = BEAT_SECONDS * FPS;
export const MUSIC_TRIM_BEFORE = Math.round(74.094875 * FPS);

export const beat = (index: number) => Math.round(index * BEAT_FRAMES);

export const CUTS = {
  create: 0,
  deploy: beat(8),
  upload: beat(14),
  protected: beat(20),
  brand: beat(24),
  end: beat(28),
} as const;

export const DURATION_IN_FRAMES = CUTS.end;

export const clamp = (
  value: number,
  inputStart: number,
  inputEnd: number,
) => {
  if (inputStart === inputEnd) {
    return value >= inputEnd ? 1 : 0;
  }

  return Math.min(1, Math.max(0, (value - inputStart) / (inputEnd - inputStart)));
};
