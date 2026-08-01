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
  FileTile,
  Grain,
  Pill,
  Window,
} from "../components";
import {DISPLAY_FONT, TINKER} from "../theme";

const fileTiles = [
  {name: "index.html", tone: "yellow" as const, x: -310, y: -250, r: -16},
  {name: "app.js", tone: "pink" as const, x: 330, y: -290, r: 12},
  {name: "styles.css", tone: "blue" as const, x: 410, y: 250, r: 17},
  {name: "tinker.yaml", tone: "mint" as const, x: -370, y: 260, r: -13},
];

export const BuildScene: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const enter = spring({
    frame,
    fps,
    config: {damping: 15, stiffness: 125, mass: 0.85},
  });
  const windowEnter = spring({
    frame: frame - 12,
    fps,
    config: {damping: 17, stiffness: 115, mass: 0.9},
  });
  const exit = interpolate(frame, [158, 173], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: Easing.bezier(0.78, 0, 1, 0.62),
  });
  const cameraScale = interpolate(exit, [0, 1], [1, 4.2]);
  const cameraX = interpolate(exit, [0, 1], [0, -330]);

  return (
    <AbsoluteFill
      style={{
        overflow: "hidden",
        color: "#fffef8",
        backgroundColor: TINKER.ink,
      }}
    >
      <div
        style={{
          position: "absolute",
          width: 1160,
          height: 1160,
          right: -150,
          top: -80,
          borderRadius: 999,
          background:
            "radial-gradient(circle at 35% 34%, #5a8dff 0%, #2f67f5 42%, #173cba 74%, #200675 75%)",
          transform: `scale(${0.78 + enter * 0.22})`,
          opacity: 0.96,
        }}
      />
      <div
        style={{
          position: "absolute",
          width: 780,
          height: 780,
          right: 40,
          top: 110,
          border: "2px solid rgba(255,255,255,0.16)",
          borderRadius: 999,
          transform: `scale(${0.8 + enter * 0.2}) rotate(${frame * 0.08}deg)`,
        }}
      />

      <BrandBug dark />

      <div
        style={{
          position: "absolute",
          left: 94,
          top: 205,
          width: 610,
          fontFamily: DISPLAY_FONT,
          fontSize: 106,
          lineHeight: 0.9,
          fontWeight: 900,
          letterSpacing: "-0.07em",
          transform: `translateY(${interpolate(enter, [0, 1], [70, 0])}px)`,
          opacity: enter,
        }}
      >
        Small apps.
        <br />
        Big launch
        <br />
        energy.
      </div>

      <div
        style={{
          position: "absolute",
          left: 785,
          top: 170,
          transformOrigin: "67% 50%",
          transform: `translateY(${interpolate(windowEnter, [0, 1], [90, 0])}px) translateX(${cameraX}px) scale(${
            interpolate(windowEnter, [0, 1], [0.88, 1]) * cameraScale
          })`,
          opacity: interpolate(exit, [0.84, 1], [1, 0], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          }) * windowEnter,
        }}
      >
        <Window width={980} height={650}>
          <div
            style={{
              height: "100%",
              padding: "56px 58px",
              color: TINKER.ink,
              background: TINKER.canvas,
            }}
          >
            <Pill style={{marginBottom: 34}}>
              <span
                style={{
                  width: 12,
                  height: 12,
                  borderRadius: 999,
                  background: TINKER.positive,
                }}
              />
              my-tinker-app
            </Pill>
            <div
              style={{
                fontFamily: DISPLAY_FONT,
                fontSize: 66,
                lineHeight: 1,
                fontWeight: 900,
                letterSpacing: "-0.055em",
              }}
            >
              Ready to share.
            </div>
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "1.5fr 1fr",
                gap: 26,
                marginTop: 52,
              }}
            >
              <div
                style={{
                  height: 210,
                  padding: 28,
                  borderRadius: 26,
                  background: TINKER.primarySoft,
                }}
              >
                {[1, 0.78, 0.9, 0.52].map((width, index) => (
                  <div
                    key={index}
                    style={{
                      width: `${width * 100}%`,
                      height: 15,
                      marginBottom: 22,
                      borderRadius: 99,
                      background:
                        index === 0 ? TINKER.primary : "rgba(47,103,245,0.28)",
                    }}
                  />
                ))}
              </div>
              <div
                style={{
                  height: 210,
                  borderRadius: 26,
                  background: TINKER.mint,
                  display: "grid",
                  placeItems: "center",
                }}
              >
                <div
                  style={{
                    width: 102,
                    height: 102,
                    borderRadius: 999,
                    background: TINKER.yellow,
                    boxShadow: "0 12px 0 rgba(32,6,117,0.09)",
                  }}
                />
              </div>
            </div>
          </div>
        </Window>

        {fileTiles.map((tile, index) => {
          const tileEnter = spring({
            frame: frame - (42 + index * 11),
            fps,
            config: {damping: 13, stiffness: 145, mass: 0.78},
          });
          const settleX = [-65, 710, 760, -90][index];
          const settleY = [-60, -40, 470, 490][index];
          const x = interpolate(tileEnter, [0, 1], [tile.x, settleX]);
          const y = interpolate(tileEnter, [0, 1], [tile.y, settleY]);
          const rotation = interpolate(tileEnter, [0, 1], [tile.r, tile.r * 0.2]);

          return (
            <FileTile
              key={tile.name}
              name={tile.name}
              tone={tile.tone}
              style={{
                position: "absolute",
                left: x,
                top: y,
                transform: `rotate(${rotation}deg) scale(${0.78 + tileEnter * 0.22})`,
                opacity: tileEnter,
              }}
            />
          );
        })}
      </div>

      <Caption
        index="01 / CREATE"
        title="Create your project."
        detail="Keep the app small. Keep control of where it runs."
        frame={frame}
        enterAt={18}
        dark
      />
      <Grain opacity={0.1} />
    </AbsoluteFill>
  );
};
