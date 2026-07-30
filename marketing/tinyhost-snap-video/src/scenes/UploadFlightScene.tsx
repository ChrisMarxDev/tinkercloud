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
  CloudMark,
  FileTile,
  Grain,
} from "../components";
import {TINY} from "../theme";

const flights = [
  {name: "index.html", tone: "yellow" as const, y: 210, delay: 3},
  {name: "app.js", tone: "pink" as const, y: 382, delay: 8},
  {name: "styles.css", tone: "blue" as const, y: 550, delay: 13},
  {name: "tiny.yaml", tone: "mint" as const, y: 714, delay: 18},
];

export const UploadFlightScene: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const cloudEnter = spring({
    frame: frame - 3,
    fps,
    config: {damping: 13, stiffness: 120, mass: 0.9},
  });
  const sweep = interpolate(frame, [67, 82], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: Easing.inOut(Easing.quad),
  });

  return (
    <AbsoluteFill
      style={{
        overflow: "hidden",
        background: TINY.canvas,
        color: TINY.ink,
      }}
    >
      <div
        style={{
          position: "absolute",
          width: 880,
          height: 880,
          right: -40,
          top: 100,
          borderRadius: 999,
          background: TINY.primarySoft,
          transform: `scale(${0.76 + cloudEnter * 0.24})`,
        }}
      />
      <BrandBug />
      <Caption
        index="02 / UPLOAD"
        title="Watch it fly."
        detail="TinyHost verifies the release before it goes live."
        frame={frame}
        enterAt={2}
      />

      <div
        style={{
          position: "absolute",
          right: 150,
          top: 190,
          width: 750,
          height: 650,
          display: "grid",
          placeItems: "center",
          transform: `scale(${0.72 + cloudEnter * 0.28})`,
        }}
      >
        <div
          style={{
            position: "absolute",
            width: 560,
            height: 560,
            borderRadius: 999,
            border: "3px solid rgba(47,103,245,0.2)",
            transform: `rotate(${frame * 0.5}deg) scale(${0.92 + Math.sin(frame * 0.08) * 0.025})`,
          }}
        />
        <CloudMark size={510} color={TINY.ink} shadowColor={TINY.primary} />
        <div
          style={{
            position: "absolute",
            bottom: 58,
            padding: "14px 23px",
            borderRadius: 999,
            color: TINY.positive,
            background: "#fffef8",
            border: `2px solid ${TINY.line}`,
            boxShadow: TINY.shadowSmall,
            fontSize: 32,
            fontWeight: 900,
          }}
        >
          VERIFYING RELEASE
        </div>
      </div>

      {flights.map((tile, index) => {
        const flight = spring({
          frame: frame - tile.delay,
          fps,
          config: {damping: 13, stiffness: 115, mass: 0.75},
          durationInFrames: 25,
        });
        const arc = Math.sin(flight * Math.PI);
        const x = interpolate(flight, [0, 1], [-250, 1275 + index * 28]);
        const y = tile.y - arc * (120 + index * 16);
        const rotation = interpolate(flight, [0, 1], [-16 + index * 9, 4]);
        const scale = interpolate(flight, [0, 0.78, 1], [1, 0.88, 0.22]);
        const alpha = interpolate(flight, [0, 0.88, 1], [1, 1, 0], {
          extrapolateLeft: "clamp",
          extrapolateRight: "clamp",
        });

        return (
          <div key={tile.name}>
            {[1, 2].map((trail) => (
              <FileTile
                key={trail}
                name={tile.name}
                tone={tile.tone}
                style={{
                  position: "absolute",
                  left: x - trail * 70,
                  top: y,
                  opacity: alpha * (0.18 / trail),
                  transform: `rotate(${rotation}deg) scale(${scale})`,
                  filter: `blur(${trail * 3}px)`,
                }}
              />
            ))}
            <FileTile
              name={tile.name}
              tone={tile.tone}
              style={{
                position: "absolute",
                left: x,
                top: y,
                opacity: alpha,
                transform: `rotate(${rotation}deg) scale(${scale})`,
              }}
            />
          </div>
        );
      })}

      <div
        style={{
          position: "absolute",
          left: interpolate(sweep, [0, 1], [-2850, -340]),
          top: -320,
          width: 2600,
          height: 1800,
          borderRadius: 70,
          background: TINY.pink,
          border: "18px solid #ff82bb",
          boxShadow: "0 0 0 40px rgba(250,75,155,0.22)",
          transform: `rotate(-13deg)`,
          filter: sweep > 0.05 && sweep < 0.95 ? "blur(2px)" : "none",
          zIndex: 50,
        }}
      />
      <Grain opacity={0.07} />
    </AbsoluteFill>
  );
};
