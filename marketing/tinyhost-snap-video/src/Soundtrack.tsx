import {Audio} from "@remotion/media";
import {Sequence, staticFile} from "remotion";
import {beat, CUTS, DURATION_IN_FRAMES, MUSIC_TRIM_BEFORE} from "./timeline";

const effects = [
  {frame: 0, file: "shimmer-sparkle-sweep.mp3", volume: 0.38},
  {frame: beat(7), file: "whoosh-fast.mp3", volume: 0.62},
  {frame: CUTS.deploy, file: "bass-hit-short.mp3", volume: 0.68},
  {frame: CUTS.deploy + 11, file: "typewriter-digital.mp3", volume: 0.32},
  {frame: beat(13), file: "zoom-air-fast.mp3", volume: 0.72},
  {frame: CUTS.upload, file: "impact-deep-whoosh.mp3", volume: 0.56},
  {frame: beat(19), file: "whoosh-fast.mp3", volume: 0.72},
  {frame: CUTS.protected, file: "bass-hit-short.mp3", volume: 0.6},
  {frame: beat(22), file: "lock-quick.mp3", volume: 0.74},
  {frame: CUTS.brand, file: "shimmer-sparkle-sweep.mp3", volume: 0.46},
] as const;

export const Soundtrack: React.FC<{bgm: boolean}> = ({bgm}) => {
  return (
    <>
      {bgm ? (
        <Audio
          src={staticFile("audio/cat-walk.mp3")}
          trimBefore={MUSIC_TRIM_BEFORE}
          volume={(frame) => {
            const fade =
              frame < 10
                ? frame / 10
                : frame > DURATION_IN_FRAMES - 20
                  ? Math.max(0, (DURATION_IN_FRAMES - frame) / 20)
                  : 1;
            return 0.34 * fade;
          }}
        />
      ) : null}
      {effects.map((effect) => (
        <Sequence key={`${effect.frame}-${effect.file}`} from={effect.frame}>
          <Audio
            src={staticFile(`audio/${effect.file}`)}
            volume={() => effect.volume}
          />
        </Sequence>
      ))}
    </>
  );
};
