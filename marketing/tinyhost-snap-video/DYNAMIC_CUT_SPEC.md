# TinyHost — Dynamic Product Film

## Creative decision

This is an autonomous free-creation cut. The requested “snap” is interpreted as
the transition character: fast acceleration, an exact impact, and a short
settle. The word **SNAP** never appears on screen.

“Apple-like” means:

- one hero object at a time;
- minimal copy and generous negative space;
- transitions made by camera movement and object continuity;
- crisp material, restrained light, and decisive sound;
- enough hold time to understand every product state.

It does not mean copying Apple branding, typography, devices, or campaign
assets. The film remains unmistakably TinyHost.

## Product promise

TinyHost makes the shortest path from a small project to a protected live URL:

1. Create your project.
2. Upload it with `tiny deploy .`.
3. Receive a protected live URL.

Supporting claims stay inside the project principles: self-hosted, private by
default, and anonymous access denied.

## Visual system

- Canvas: `#fbf8f1`
- Ink: `#200675`
- Action blue: `#2f67f5`
- Energy accents: pink `#fa4b9b`, yellow `#ffd865`, mint `#c9f4d6`
- Rounded system display type, rounded cards, opaque lift shadows
- TinyHost cloud mark is the final brand object

The first render already established the local design tokens, so a separate
styleframe approval gate is unnecessary. This cut keeps those tokens and
changes the motion system.

## Music grid

- Track: “Cat Walk” by Arulo
- Analysed tempo: 129.995 BPM
- Beat interval: 0.461558 seconds / 13.84674 frames at 30 fps
- Selected high-energy 28-beat window: 74.094875s–87.016780s
- Runtime: 388 frames / 12.93 seconds

Every major scene boundary lands on beats 8, 14, 20, and 24.

## Shot map

Only `transition-hidden-cut / invisible-cut` is claimed as a direct Gallery
variant. The other named cards were research references, then deliberately
adapted into custom TinyHost motions. This distinction avoids claiming Gallery
fidelity where the target film uses different counts, timings, or geometry.

| Beats | Product beat | Primary motion | Implementation status |
|---|---|---|---|
| 0–8 | Create your project | Four project files spring into a single hero window | Custom four-tile assembly, informed by `deck-deal-flyin / deck-deal-flyin`; not a variant claim |
| 8–14 | Upload it | Command types at 2f/character, holds, then camera-dives into a cream impact bridge | Custom command ignition, informed by `typewriter-moves / terminal-typewriter` and `crash-zoom-punch / crash-zoom-punch`; not a variant claim |
| 14–20 | Upload flight | Four files accelerate in 5f offsets toward the TinyHost cloud | Custom accelerated file flight; no Gallery style key claimed |
| 20–24 | Protected live URL | A full-frame foreground file hides the cut; browser resolves and a lock lands | Direct `transition-hidden-cut / invisible-cut`, followed by a custom lock landing |
| 24–28 | Brand resolve | Lock reaches edge-on width, swaps, and blooms into the TinyHost mark | Custom lock-to-brand squeeze, informed by `ui-to-brand-morph / icon-flip-bloom`; not a variant claim |

The white/cream bridge at frames 191–193 is an intentional flash-cut impact at
the end of the custom camera dive. It is not a standalone title card and carries
no copy. Upload begins on beat 14 at frame 194.

## Transition contract

- No separate title cards between steps.
- The outgoing hero becomes the incoming transition object.
- Fast spans last 6–14 frames, followed by a visible 3–6 frame settle.
- Motion blur and trails only appear during high-velocity spans.
- The final wordmark holds for at least 30 frames.

## Audio contract

Sound effects are declarative and locked to global frames. Music can be disabled
with the `bgm` input prop; the no-music render retains all effects.
