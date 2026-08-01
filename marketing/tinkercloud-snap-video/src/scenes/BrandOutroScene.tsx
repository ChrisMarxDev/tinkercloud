import {
  AbsoluteFill,
  Easing,
  interpolate,
  spring,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import {CloudMark, Grain, LockIcon} from "../components";
import {DISPLAY_FONT, TINKER} from "../theme";

const wordmark = "tinkercloud";

export const BrandOutroScene: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const compress = interpolate(frame, [4, 16], [1, 0.04], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: Easing.in(Easing.cubic),
  });
  const bloom = spring({
    frame: frame - 17,
    fps,
    config: {damping: 12, stiffness: 170, mass: 0.7},
  });
  const markLeft = interpolate(frame, [16, 34], [490, 250], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: Easing.out(Easing.cubic),
  });
  const showCloud = frame >= 17;
  const tagline = spring({
    frame: frame - 44,
    fps,
    config: {damping: 18, stiffness: 145, mass: 0.7},
  });

  return (
    <AbsoluteFill
      style={{
        overflow: "hidden",
        alignItems: "center",
        justifyContent: "center",
        color: "#fffef8",
        background: TINKER.primary,
      }}
    >
      <div
        style={{
          position: "absolute",
          width: 1000,
          height: 1000,
          borderRadius: 999,
          background: TINKER.ink,
          transform: `scale(${0.78 + bloom * 0.22})`,
          boxShadow: "0 0 0 1000px #200675",
        }}
      />
      <div
        style={{
          position: "absolute",
          width: 680,
          height: 680,
          borderRadius: 999,
          border: "2px solid rgba(255,255,255,0.13)",
          transform: `scale(${0.86 + bloom * 0.14}) rotate(${frame * 0.35}deg)`,
        }}
      />

      <div
        style={{
          position: "absolute",
          left: "50%",
          top: "50%",
          width: 1200,
          height: 220,
          zIndex: 5,
          transform: "translate(-50%, -50%)",
        }}
      >
        <div
          style={{
            position: "absolute",
            left: markLeft,
            top: 0,
            width: 220,
            height: 220,
            display: "grid",
            placeItems: "center",
            transform: showCloud
              ? `scaleX(${interpolate(bloom, [0, 1], [0.04, 1])}) scale(${
                  0.7 + bloom * 0.3
                })`
              : `scaleX(${compress}) rotate(${Math.sin(frame * 0.9) * 2}deg)`,
          }}
        >
          {showCloud ? (
            <CloudMark
              size={220}
              color="#fffef8"
              faceColor={TINKER.ink}
              shadowColor={TINKER.primary}
            />
          ) : (
            <LockIcon size={178} color={TINKER.ink} background={TINKER.mint} />
          )}
        </div>

        <div
          style={{
            position: "absolute",
            left: 498,
            top: 47,
            width: 700,
            overflow: "hidden",
            whiteSpace: "nowrap",
            fontFamily: DISPLAY_FONT,
            fontSize: 126,
            lineHeight: 1,
            fontWeight: 900,
            letterSpacing: "-0.075em",
          }}
        >
          {wordmark.split("").map((character, index) => {
            const characterEnter = spring({
              frame: frame - (24 + index * 1.7),
              fps,
              config: {damping: 15, stiffness: 190, mass: 0.58},
            });

            return (
              <span
                key={`${character}-${index}`}
                style={{
                  display: "inline-block",
                  opacity: characterEnter,
                  filter: `blur(${interpolate(characterEnter, [0, 1], [13, 0])}px)`,
                  transform: `translateY(${interpolate(
                    characterEnter,
                    [0, 1],
                    [34, 0],
                  )}px) scale(${interpolate(characterEnter, [0, 1], [0.72, 1])})`,
                }}
              >
                {character}
              </span>
            );
          })}
        </div>
      </div>

      <div
        style={{
          position: "absolute",
          top: 700,
          zIndex: 5,
          color: TINKER.yellow,
          fontFamily: DISPLAY_FONT,
          fontSize: 36,
          fontWeight: 850,
          letterSpacing: "-0.025em",
          opacity: tagline,
          transform: `translateY(${interpolate(tagline, [0, 1], [26, 0])}px)`,
        }}
      >
        Build small. Share safely.
      </div>
      <Grain opacity={0.1} />
    </AbsoluteFill>
  );
};
