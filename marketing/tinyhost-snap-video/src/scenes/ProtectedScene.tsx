import {
  AbsoluteFill,
  Easing,
  interpolate,
  spring,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import {
  BrandBug,
  Caption,
  Grain,
  LockIcon,
  Pill,
  Window,
} from "../components";
import {DISPLAY_FONT, TINY} from "../theme";

export const ProtectedScene: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const reveal = spring({
    frame: frame - 4,
    fps,
    config: {damping: 15, stiffness: 155, mass: 0.8},
  });
  const lockLand = spring({
    frame: frame - 24,
    fps,
    config: {damping: 11, stiffness: 180, mass: 0.65},
  });
  const handoff = interpolate(frame, [41, 54], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: Easing.bezier(0.2, 0.85, 0.2, 1),
  });

  return (
    <AbsoluteFill
      style={{
        overflow: "hidden",
        background: TINY.primary,
        color: "#fffef8",
      }}
    >
      <div
        style={{
          position: "absolute",
          inset: 0,
          background:
            "radial-gradient(circle at 72% 45%, rgba(255,255,255,0.2), transparent 42%)",
        }}
      />
      <BrandBug dark />
      <Caption
        index="03 / LIVE"
        title="Get a protected live URL."
        detail="Private by default. Anonymous access is denied."
        frame={frame}
        enterAt={5}
        dark
      />

      <div
        style={{
          position: "absolute",
          left: 610,
          top: 190,
          transformOrigin: "66% 50%",
          transform: `translateY(${interpolate(reveal, [0, 1], [70, 0])}px) scale(${
            interpolate(reveal, [0, 1], [0.88, 1]) *
            interpolate(handoff, [0, 1], [1, 0.72])
          })`,
          opacity: reveal * interpolate(handoff, [0, 0.55], [1, 0], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          }),
        }}
      >
        <Window width={1160} height={640}>
          <div
            style={{
              padding: "54px 60px",
              color: TINY.ink,
              background: TINY.canvas,
              height: "100%",
            }}
          >
            <Pill
              style={{
                width: "100%",
                padding: "18px 26px",
                fontSize: 32,
                background: "#ffffff",
              }}
            >
              <span style={{color: TINY.positive, fontSize: 25}}>●</span>
              https://my-tiny-app.example.com/
            </Pill>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 34,
                marginTop: 54,
                padding: "38px 42px",
                borderRadius: 30,
                border: `2px solid ${TINY.line}`,
                background: TINY.surfaceRaised,
                boxShadow: TINY.shadow,
              }}
            >
              <div
                style={{
                  transform: `translateY(${interpolate(
                    lockLand,
                    [0, 1],
                    [-120, 0],
                  )}px) rotate(${interpolate(lockLand, [0, 1], [-16, 0])}deg) scale(${
                    0.65 + lockLand * 0.35
                  })`,
                  opacity: lockLand,
                }}
              >
                <LockIcon size={110} />
              </div>
              <div>
                <div
                  style={{
                    fontFamily: DISPLAY_FONT,
                    fontSize: 44,
                    fontWeight: 900,
                    letterSpacing: "-0.045em",
                  }}
                >
                  Protected and live.
                </div>
                <div
                  style={{
                    marginTop: 10,
                    color: TINY.muted,
                    fontSize: 32,
                    fontWeight: 650,
                  }}
                >
                  Access stays at the gateway.
                </div>
              </div>
            </div>
          </div>
        </Window>
      </div>

      <div
        style={{
          position: "absolute",
          left: "50%",
          top: "50%",
          zIndex: 40,
          opacity: interpolate(handoff, [0.28, 0.6], [0, 1], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          }),
          transform: `translate(-50%, -50%) scale(${interpolate(
            handoff,
            [0, 1],
            [0.55, 1],
          )})`,
        }}
      >
        <LockIcon size={178} color={TINY.ink} background={TINY.mint} />
      </div>

      <div
        style={{
          position: "absolute",
          left: interpolate(frame, [0, 11], [-340, 2180], {
            extrapolateRight: "clamp",
            easing: Easing.out(Easing.quad),
          }),
          top: -320,
          width: 2600,
          height: 1800,
          zIndex: 60,
          borderRadius: 70,
          background: TINY.pink,
          border: "18px solid #ff82bb",
          transform: "rotate(-13deg)",
          boxShadow: "0 0 0 40px rgba(250,75,155,0.22)",
        }}
      />
      <Grain opacity={0.08} />
    </AbsoluteFill>
  );
};
