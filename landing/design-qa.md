# Design QA

## Visual truth

- Selected direction: `/Users/christophermarx/.codex/generated_images/019fa33d-9a0d-70a1-9bb3-3a34ff926e3d/call_wcDdDHxXjsUAzkqa2BE62nrA.png`
- Supplied brand asset: `public/tinkercloud-icon.svg`
- Desktop implementation: `/private/tmp/tinkercloud-value-p0-desktop.png`
- Mobile hero implementation: `/private/tmp/tinkercloud-value-p0-mobile.png`
- Mobile receipt implementation: `/private/tmp/tinkercloud-value-p0-mobile-receipt.png`

## Capture state

- Desktop viewport: 1440 × 1024 CSS pixels at DPR 2
- Mobile viewport: 390 × 844 CSS pixels at DPR 1
- Source artwork: 864 × 1821 pixels
- Page state: public static landing page, default state
- Interaction states checked: “See one deploy” anchor navigation, operator setup
  link semantics, deploy-command copy confirmation, and SDK code copy

## Comparison evidence

- Normalized hero comparison: `/private/tmp/tinkercloud-allowlist-comparison.png`
- Normalized full/lower-page comparison: `/private/tmp/tinkercloud-qa-full-comparison.png`
- Focused implementation captures:
  - `/private/tmp/tinkercloud-implementation-how.png`
  - `/private/tmp/tinkercloud-implementation-code.png`

The focused captures isolate the dense feature and code areas so typography,
spacing, and copy can be judged without full-page screenshot scaling.

## Fidelity review

- Typography: Fredoka supplies the rounded display character and Nunito Sans
  keeps supporting copy friendly and readable.
- Spacing: the large hero breathing room, compact three-step sequence, and
  grouped essentials preserve the selected direction's hierarchy.
- Color: warm cream, deep grape, cobalt, pink, and soft lilac match the visual
  target without introducing gradients.
- Assets: only the supplied Tinkercloud mark is used. No borrowed mascot or
  character artwork is present.
- Copy: auth, app-scoped storage, ephemeral live sockets, and the typed client
  SDK are all represented. The code card uses actual Tinkercloud SDK methods. The hero
  now names the team-tool category, deployer/agent use case, operator-owned VPS,
  and stable private URL outcome before explaining platform capabilities.
- Terminal receipt: the command quotes the wildcard viewer rule, wraps at
  semantic command/URL boundaries on mobile, and visualizes only existing
  activation and anonymous-denial evidence. Copy controls retain visible text,
  accessible names, and polite confirmation.
- Responsive behavior: no horizontal overflow at 390 px; cards, actions, code,
  and feature rows reflow cleanly.

## Interaction and runtime checks

- Primary CTA navigates to `#how-it-works`.
- Copy control writes the SDK example and changes to a “Copied” confirmation.
- Browser console checked with no warnings or errors.
- Production build, rendered-HTML tests, and lint all pass.

## Comparison history

- Pass 1: no P0, P1, or P2 issues found.
- Pass 2: added the `--allow` deploy flag and explicit
  internet-reachability copy. The updated hero comparison and 390 px capture
  show no overflow or hierarchy regression; no P0, P1, or P2 issues found.
- Pass 3: replaced the individual example address with the team-domain
  placeholder `*@acme.com`; the shorter value preserves the verified command
  wrapping at desktop and mobile widths.
- Pass 4: loosened the hero headline tracking and line height, with a slightly
  smaller mobile scale, to keep the rounded display type playful without
  crowding letters or lines.
- Pass 5: shifted the page canvas from yellow cream to a warm neutral
  off-white, keeping yellow concentrated in the privacy and folder accents.
- Pass 6: made platform ownership explicit in the hero, deploy step, and
  footer: Tinkercloud is open source and runs on the operator's own VPS.
- Pass 7: linked the hero ownership chip and footer to the configured GitHub
  repository, with a tooltip noting that the repository is private for now.
- Pass 8: added a compact bottom-of-page hosting card that sends operators to
  the repository README for VPS setup and first-deployment guidance.
- Pass 9: confirmed the Tinkercloud icon is the active brand
  asset, then added restrained pink, blue, yellow, and green micro-flourishes
  around each major section. Mobile keeps only two accents per section to
  preserve the quiet, simple layout.
- Pass 10: replaced the original cream-backed outline mark with
  `tinkercloud-transparent.svg` in both brand placements and adjusted its frame
  so the new cloud face and colorful shapes remain visible at small sizes.
- Pass 11: corrected the brand source to `tinkercloud-mark.svg`, keeping the
  transparent purple-and-blue cloud mark in the header and footer while the
  page-level flourishes provide the surrounding color.
- Pass 12: replaced the abstract folder-command-URL illustration with the dark
  grape “trusted deploy receipt,” rewrote the hero around the private team-tool
  outcome, and separated the deployer and operator actions. Desktop and 390 px
  checks show no horizontal overflow; reduced motion removes the receipt reveal.
- P3 intentional adaptation: the production code section uses a two-column
  desktop layout for readability instead of the artwork's full-width card.
- P3 intentional adaptation: the supplied SVG keeps its native proportions,
  making the navigation mark slightly smaller than the illustrated concept.

## Final result

passed
