import "./index.css";
import {Composition, Folder} from "remotion";
import {
  TinkercloudDynamicProps,
  TinkercloudDynamicVideo,
} from "./Composition";
import {BrandOutroScene} from "./scenes/BrandOutroScene";
import {BuildScene} from "./scenes/BuildScene";
import {DeployScene} from "./scenes/DeployScene";
import {ProtectedScene} from "./scenes/ProtectedScene";
import {UploadFlightScene} from "./scenes/UploadFlightScene";
import {CUTS, DURATION_IN_FRAMES, FPS} from "./timeline";

export const RemotionRoot: React.FC = () => {
  return (
    <>
      <Folder name="Dynamic-cut-scenes">
        <Composition
          id="CreateProjectDynamic"
          component={BuildScene}
          durationInFrames={CUTS.deploy - CUTS.create}
          fps={FPS}
          width={1920}
          height={1080}
        />
        <Composition
          id="DeployCommandDynamic"
          component={DeployScene}
          durationInFrames={CUTS.upload - CUTS.deploy}
          fps={FPS}
          width={1920}
          height={1080}
        />
        <Composition
          id="UploadFlightDynamic"
          component={UploadFlightScene}
          durationInFrames={CUTS.protected - CUTS.upload}
          fps={FPS}
          width={1920}
          height={1080}
        />
        <Composition
          id="ProtectedUrlDynamic"
          component={ProtectedScene}
          durationInFrames={CUTS.brand - CUTS.protected}
          fps={FPS}
          width={1920}
          height={1080}
        />
        <Composition
          id="BrandResolveDynamic"
          component={BrandOutroScene}
          durationInFrames={CUTS.end - CUTS.brand}
          fps={FPS}
          width={1920}
          height={1080}
        />
      </Folder>
      <Composition
        id="TinkercloudSnap"
        component={TinkercloudDynamicVideo}
        durationInFrames={DURATION_IN_FRAMES}
        fps={FPS}
        width={1920}
        height={1080}
        defaultProps={
          {
            bgm: true,
          } satisfies TinkercloudDynamicProps
        }
      />
    </>
  );
};
