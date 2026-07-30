import {
  AbsoluteFill,
  Easing,
  interpolate,
  spring,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import {BrandBug, Caption, Grain, Window} from "../components";
import {MONO_FONT, TINKER} from "../theme";

const command = "tinker deploy .";

export const DeployScene: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const intro = spring({
    frame,
    fps,
    config: {damping: 15, stiffness: 170, mass: 0.75},
  });
  const typedChars = Math.max(0, Math.min(command.length, Math.floor((frame - 11) / 2)));
  const typed = command.slice(0, typedChars);
  const submitted = frame >= 43;
  const exit = interpolate(frame, [69, 82], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: Easing.bezier(0.84, 0, 1, 0.7),
  });
  const cameraScale = interpolate(exit, [0, 1], [1, 4.8]);
  const cameraY = interpolate(exit, [0, 1], [0, -90]);
  const terminalOpacity = interpolate(exit, [0.82, 1], [1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <AbsoluteFill
      style={{
        overflow: "hidden",
        color: "#fffef8",
        background: "#12043f",
      }}
    >
      <div
        style={{
          position: "absolute",
          inset: -160,
          opacity: 0.5,
          background:
            "radial-gradient(circle at 68% 45%, rgba(47,103,245,0.85) 0%, rgba(47,103,245,0.2) 30%, transparent 57%)",
          transform: `scale(${1 + exit * 0.5})`,
        }}
      />
      <BrandBug dark />

      <div
        style={{
          position: "absolute",
          left: 620,
          top: 170,
          transformOrigin: "63% 53%",
          transform: `translateY(${cameraY}px) scale(${interpolate(
            intro,
            [0, 1],
            [2.4, cameraScale],
          )})`,
          opacity: terminalOpacity,
          filter: exit > 0.62 ? `blur(${(exit - 0.62) * 12}px)` : "none",
        }}
      >
        <Window width={1120} height={680} dark>
          <div
            style={{
              padding: "58px 64px",
              fontFamily: MONO_FONT,
              color: "#fffef8",
            }}
          >
            <div
              style={{
                marginBottom: 42,
                color: "rgba(255,255,255,0.46)",
                fontSize: 32,
                fontWeight: 700,
              }}
            >
              ~/projects/my-tinker-app
            </div>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                minHeight: 92,
                fontSize: 58,
                fontWeight: 800,
                letterSpacing: "-0.045em",
              }}
            >
              <span style={{marginRight: 24, color: TINKER.mint}}>›</span>
              <span>{typed}</span>
              <span
                style={{
                  width: 5,
                  height: 64,
                  marginLeft: 8,
                  borderRadius: 9,
                  background: TINKER.yellow,
                  opacity: Math.floor(frame / 7) % 2 === 0 ? 1 : 0.2,
                }}
              />
            </div>

            <div
              style={{
                marginTop: 38,
                opacity: submitted
                  ? interpolate(frame, [43, 49], [0, 1], {
                      extrapolateRight: "clamp",
                    })
                  : 0,
                transform: `translateY(${interpolate(
                  frame,
                  [43, 49],
                  [24, 0],
                  {extrapolateLeft: "clamp", extrapolateRight: "clamp"},
                )}px)`,
              }}
            >
                <div style={{fontSize: 32, color: "rgba(255,255,255,0.58)"}}>
                Packaging 4 files
              </div>
              <div
                style={{
                  width: 780,
                  height: 14,
                  marginTop: 22,
                  overflow: "hidden",
                  borderRadius: 99,
                  background: "rgba(255,255,255,0.12)",
                }}
              >
                <div
                  style={{
                    width: `${interpolate(frame, [44, 65], [0, 100], {
                      extrapolateLeft: "clamp",
                      extrapolateRight: "clamp",
                      easing: Easing.out(Easing.cubic),
                    })}%`,
                    height: "100%",
                    borderRadius: 99,
                    background: TINKER.mint,
                  }}
                />
              </div>
            </div>
          </div>
        </Window>
      </div>

      <Caption
        index="02 / DEPLOY"
        title="Upload it."
        detail="One command. One atomic release."
        frame={frame}
        enterAt={4}
        dark
      />

      <div
        style={{
          position: "absolute",
          inset: 0,
          pointerEvents: "none",
          opacity: interpolate(exit, [0.5, 1], [0, 1], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          }),
          background: TINKER.canvas,
        }}
      />
      <Grain opacity={0.09} />
    </AbsoluteFill>
  );
};
