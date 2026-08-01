# Landing value-story contract

## Outcome

The public landing page lets a prospective operator or deployer understand in
one viewport that Tinkercloud turns small static apps into private team tools,
deploys them through one command, and runs on one operator-controlled VPS.

The hero demonstrates a private deployment as a compact receipt: a shell-safe
command, the existing activation and anonymous-denial evidence, the allowed
viewer domain from the command, and an illustrative stable private URL.

## Trust boundary and ownership

- The landing page is explanatory static content. It has no authority over the
  Tinkercloud gateway, operators, deployers, viewers, deployments, policies, or
  app data.
- An operator owns the VPS and platform setup. A deployer owns the action of
  deploying an app they are authorized to manage. A viewer only opens an app
  after the gateway derives identity and evaluates the current policy.
- Terminal output may summarize only stable evidence already defined by the
  deployment contract. It must not imply that the marketing Worker performed a
  deployment or authenticated a viewer.

## Required first-viewport story

- Name the product category as self-hosted, private-by-default hosting for
  small team apps.
- Name both teams and coding agents as sources of dashboards, prototypes,
  reports, and utilities without assigning authority to an agent implicitly.
- State the one-VPS operator-ownership boundary.
- Give deployers a direct path to the deploy explanation and operators a direct
  path to setup documentation.
- Render `tinker deploy ./dist --allow '*@acme.com'` with the wildcard quoted so
  a shell cannot expand it.
- Label the result as an example and keep the private URL non-interactive.
- Provide keyboard-accessible copy controls for the command and URL, visible
  copy confirmation, visible focus, and a reduced-motion fallback.

## Deny-path test charter

- The landing page must not contain a form, credential input, executable
  deployment request, long-lived secret, app ID, or an untyped authorization
  claim.
- The example must not render the unsafe unquoted `--allow *@acme.com` command.
- The receipt must not claim public access, anonymous capabilities, a backend
  runtime, durable realtime history, deployment to an edge platform, or any
  planned reach-and-insights feature as available.
- A decorative or animated status must not be the only representation of the
  command, activation evidence, anonymous denial, allowed viewers, or URL.
- The illustrative URL must not be a live navigation target.
- Metadata and visible copy must not describe Tinkercloud as a general-purpose
  PaaS or obscure that the operator controls the VPS.
