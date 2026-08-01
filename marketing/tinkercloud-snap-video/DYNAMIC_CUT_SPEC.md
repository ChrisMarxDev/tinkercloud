# Tinkercloud — Dynamic Product Film

## Creative decision

This is an autonomous free-creation cut. The requested “snap” is interpreted as
the transition character: fast acceleration, an exact impact, and a short
settle. The word **SNAP** never appears on screen.

“Apple-like” means:

- one hero object at a time;
- minimal copy and generous negative space;
- transitions made by camera movement and object continuity;
- crisp material, restrained light, and decisive sound;
- enough hold time to understand every product state;
- a 20–30 second runtime with a clear low → build → peak energy curve.

It does not mean copying Apple branding, typography, devices, or campaign
assets. The film remains unmistakably Tinkercloud.

## Product promise

Tinkercloud makes the shortest path from a small project to a protected live URL:

1. Create your project.
2. Upload it with `tinker deploy .`.
3. Receive a protected live URL.

Supporting claims stay inside the project principles: self-hosted, private by
default, and anonymous access denied.

## Visual system

- Canvas: `#fbf8f1`
- Ink: `#200675`
- Action blue: `#2f67f5`
- Energy accents: pink `#fa4b9b`, yellow `#ffd865`, mint `#c9f4d6`
- Rounded system display type, rounded cards, opaque lift shadows
- The Tinkercloud mark is the final brand object

The approved first render established the local design tokens, so a separate
styleframe approval gate is unnecessary. This pass retains those tokens while
expanding the story and replacing the complete soundtrack.

## Requirement-to-execution decisions

| Requirement | Execution |
|---|---|
| 20–30 second marketing-fluff runtime | 52-beat, 25.16-second master |
| Preserve the approved visual direction | Same Tinkercloud palette, typography, cloud mark, rounded UI, and opaque lift shadows |
| Keep the “snap” dynamic rather than literal | Accelerating object continuity, camera dives, one full-frame hidden cut, and a lock-to-brand squeeze |
| Better music | Replace fashion EDM with the driving corporate tech-house track “Deep Urban” |
| Better sound effects | Remove digital confirmation/typewriter sounds; use air movement, real keyboard texture, mechanical lock, deep impacts, and a restrained cinematic finale |
| Product accuracy | `tinker.yaml`, `tinker deploy .`, hostname-derived app URL, gateway authorization, private-by-default copy |
| Data safety | Fictional `my-tinker-app.example.com`; no people, customers, credentials, or internal addresses |

## Music grid

- Track: “Deep Urban” by Eugenio Mininni
- Source: Mixkit Stock Music, asset 623
- Analysed tempo: 124.0067 BPM
- Beat interval: 0.483845 seconds / 14.51535 frames at 30 fps
- Selected high-energy 52-beat window: 10.424087s–35.584017s
- Runtime: 755 frames / 25.17 seconds

Every major scene boundary lands on beats 12, 22, 34, and 44. Beat 0 starts on
one of the track’s strongest measured kicks.

## Shot map

Only `transition-hidden-cut / invisible-cut` is claimed as a direct Gallery
variant. The other named cards were research references, then deliberately
adapted into custom Tinkercloud motions. This distinction avoids claiming Gallery
fidelity where the target film uses different counts, timings, or geometry.

| Beats | Product beat | Primary motion | Implementation status |
|---|---|---|---|
| 0–12 | Create your project | The hero window rises first; four project files then assemble in a readable cadence and hold | Custom four-tile assembly, informed by `deck-deal-flyin / deck-deal-flyin`; not a variant claim |
| 12–22 | Deploy it | A real-speed command types, packaging completes, and “Release staged” holds before the camera dive | Custom command ignition, informed by `typewriter-moves / terminal-typewriter` and `crash-zoom-punch / crash-zoom-punch`; not a variant claim |
| 22–34 | Upload and verify | Four files arc into the Tinkercloud cloud; verification visibly resolves before the hidden cut | Custom accelerated file flight and verification state; no Gallery style key claimed |
| 34–44 | Protected live URL | A full-frame foreground file hides the cut; the browser resolves, the gateway lock lands, and private defaults become explicit | Direct `transition-hidden-cut / invisible-cut`, followed by a custom lock landing |
| 44–52 | Brand resolve | Lock reaches edge-on width, swaps, blooms into the Tinkercloud mark, and the wordmark settles character by character | Custom lock-to-brand squeeze, informed by `ui-to-brand-morph / icon-flip-bloom`; not a variant claim |

The cream bridge at the end of the deploy scene is an intentional flash-cut
impact inside the custom camera dive. It is not a standalone title card and
carries no copy. Upload begins on beat 22.

## Transition contract

- No separate title cards between steps.
- The outgoing hero becomes the incoming transition object.
- Fast spans last 8–20 frames, followed by a visible settle or hold.
- Motion blur and trails only appear during high-velocity spans.
- Batch assembly receives at least 0.5 seconds of rest.
- The final wordmark holds for more than 1 second.

## Audio contract

The bed is “Deep Urban,” a 124 BPM driving tech-house track selected for its
strong four-on-the-floor pulse and lower melodic competition. The effect
vocabulary is physical and cinematic:

- air whooshes for camera and batch movement;
- real keyboard texture for `tinker deploy .`;
- mechanical movement for packaging and a metallic lock for verification;
- deep impacts only at scene landings;
- a restrained riser → impact → shimmer finale.

Digital confirmation bleeps, cartoon pops, and game-like notification sounds
are excluded. Sound effects are declarative, documented by visual action, and
locked to relative scene/beat positions. Music can be disabled with the `bgm`
input prop; the no-music render retains all effects.
