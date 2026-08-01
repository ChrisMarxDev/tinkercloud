import {Audio} from "@remotion/media";
import {Sequence, staticFile} from "remotion";
import {beat, CUTS, DURATION_IN_FRAMES, MUSIC_TRIM_BEFORE} from "./timeline";

type Effect = {
  frame: number;
  file: string;
  volume: number;
  durationInFrames?: number;
};

const effects: Effect[] = [
  // Opening atmosphere: the project window lifts into view.
  {frame: CUTS.create, file: "transition-soft.mp3", volume: 0.28},
  // Four project files assemble around the hero window.
  {frame: CUTS.create + 40, file: "air-woosh-deep.mp3", volume: 0.24},
  // Camera accelerates through the project into the terminal.
  {frame: beat(11), file: "air-whoosh-powerful.mp3", volume: 0.48},
  // Terminal lands on the first deploy beat.
  {frame: CUTS.deploy, file: "impact-zoom-quick.mp3", volume: 0.38},
  // Physical keyboard texture, cropped to the command typing action.
  {
    frame: CUTS.deploy + 26,
    file: "keyboard.mp3",
    volume: 0.68,
    durationInFrames: 40,
  },
  // Packaging mechanism runs with the progress bar.
  {
    frame: CUTS.deploy + 68,
    file: "mech-tech-movement.mp3",
    volume: 0.58,
    durationInFrames: 52,
  },
  // Camera dives from the staged release into upload.
  {frame: beat(21), file: "whoosh-big.mp3", volume: 0.5},
  // Upload scene lands.
  {frame: CUTS.upload, file: "impact-deep-whoosh.mp3", volume: 0.34},
  // One broad air movement carries the complete file flight.
  {frame: CUTS.upload + 18, file: "air-woosh-deep.mp3", volume: 0.36},
  // Verification locks the immutable release.
  {frame: CUTS.upload + 108, file: "gear-lock-metallic.mp3", volume: 0.58},
  // Full-frame pink object creates the hidden cut.
  {frame: beat(33), file: "sweep-fast.mp3", volume: 0.56},
  // Protected browser resolves on the next downbeat.
  {frame: CUTS.protected, file: "impact-zoom-quick.mp3", volume: 0.34},
  // The gateway lock lands inside the browser.
  {frame: CUTS.protected + 52, file: "gear-lock-metallic.mp3", volume: 0.7},
  // Browser compresses into the centered lock.
  {frame: beat(43), file: "transition-soft.mp3", volume: 0.42},
  // Finale sentence: riser → impact → shimmer.
  {
    frame: CUTS.brand - 24,
    file: "riser-cine.mp3",
    volume: 0.22,
    durationInFrames: 140,
  },
  {frame: CUTS.brand + 11, file: "impact-deep-whoosh.mp3", volume: 0.5},
  {
    frame: CUTS.brand + 36,
    file: "shimmer-sparkle-sweep.mp3",
    volume: 0.34,
  },
];

export const Soundtrack: React.FC<{bgm: boolean}> = ({bgm}) => {
  return (
    <>
      {bgm ? (
        <Audio
          src={staticFile("audio/deep-urban.mp3")}
          trimBefore={MUSIC_TRIM_BEFORE}
          volume={(frame) => {
            const fade =
              frame < 18
                ? frame / 18
                : frame > DURATION_IN_FRAMES - 36
                  ? Math.max(0, (DURATION_IN_FRAMES - frame) / 36)
                  : 1;
            return 0.27 * fade;
          }}
        />
      ) : null}
      {effects.map((effect) => (
        <Sequence
          key={`${effect.frame}-${effect.file}`}
          from={effect.frame}
          durationInFrames={effect.durationInFrames}
        >
          <Audio
            src={staticFile(`audio/${effect.file}`)}
            volume={() => effect.volume}
          />
        </Sequence>
      ))}
    </>
  );
};
