import {AbsoluteFill, Sequence} from "remotion";
import {Soundtrack} from "./Soundtrack";
import {BrandOutroScene} from "./scenes/BrandOutroScene";
import {BuildScene} from "./scenes/BuildScene";
import {DeployScene} from "./scenes/DeployScene";
import {ProtectedScene} from "./scenes/ProtectedScene";
import {UploadFlightScene} from "./scenes/UploadFlightScene";
import {CUTS} from "./timeline";

export type TinkercloudDynamicProps = {
  bgm: boolean;
};

export const TinkercloudDynamicVideo: React.FC<TinkercloudDynamicProps> = ({bgm}) => {
  return (
    <AbsoluteFill style={{backgroundColor: "#200675"}}>
      <Sequence
        from={CUTS.create}
        durationInFrames={CUTS.deploy - CUTS.create}
        premountFor={20}
      >
        <BuildScene />
      </Sequence>
      <Sequence
        from={CUTS.deploy}
        durationInFrames={CUTS.upload - CUTS.deploy}
        premountFor={20}
      >
        <DeployScene />
      </Sequence>
      <Sequence
        from={CUTS.upload}
        durationInFrames={CUTS.protected - CUTS.upload}
        premountFor={20}
      >
        <UploadFlightScene />
      </Sequence>
      <Sequence
        from={CUTS.protected}
        durationInFrames={CUTS.brand - CUTS.protected}
        premountFor={20}
      >
        <ProtectedScene />
      </Sequence>
      <Sequence
        from={CUTS.brand}
        durationInFrames={CUTS.end - CUTS.brand}
        premountFor={20}
      >
        <BrandOutroScene />
      </Sequence>
      <Soundtrack bgm={bgm} />
    </AbsoluteFill>
  );
};
