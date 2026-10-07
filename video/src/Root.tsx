import React from "react";
import { Composition, Freeze } from "remotion";
import { Explainer } from "./Video";
import { buildTimeline, FPS, HEIGHT, WIDTH } from "./timeline";
import { VERSIONS } from "./versions";

// Each version is a composition named after it ("curto"), and its
// poster a still: the ring of tools with the WhatsApp in it, at the end of the
// first scene, after its last subtitle.
export const Root: React.FC = () => (
  <>
    {VERSIONS.map((version) => {
      const timeline = buildTimeline(version);
      const first = timeline.scenes[0];
      return (
        <React.Fragment key={version.id}>
          <Composition
            id={version.id}
            component={Explainer}
            durationInFrames={timeline.total}
            fps={FPS}
            width={WIDTH}
            height={HEIGHT}
            defaultProps={{ version: version.id }}
          />
          {/* As long as the video, or its scenes would be cut short: rendered at frame 0. */}
          <Composition
            id={`${version.id}-poster`}
            component={() => (
              <Freeze frame={first.from + first.frames - 4}>
                <Explainer version={version.id} />
              </Freeze>
            )}
            durationInFrames={timeline.total}
            fps={FPS}
            width={WIDTH}
            height={HEIGHT}
          />
        </React.Fragment>
      );
    })}
  </>
);
