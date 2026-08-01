# Landing value-story contract

## Outcome

The public landing page lets a prospective operator or deployer understand in
one viewport that Tinkercloud turns small static apps into private team tools,
deploys them through one command, and runs on one operator-controlled VPS.

The hero demonstrates the product through one shell-safe deploy command. The
marketing copy owns the outcome; the terminal does not simulate CLI output.

## Trust boundary and ownership

- The landing page is explanatory static content. It has no authority over the
  Tinkercloud gateway, operators, deployers, viewers, deployments, policies, or
  app data.
- An operator owns the VPS and platform setup. A deployer owns the action of
  deploying an app they are authorized to manage. A viewer only opens an app
  after the gateway derives identity and evaluates the current policy.
- The command is explanatory static text. It must not imply that the marketing
  Worker performed a deployment or authenticated a viewer.

## Required first-viewport story

- Name the product category as self-hosted, private-by-default hosting for
  small team apps.
- Name both teams and coding agents as sources of dashboards, prototypes,
  reports, and utilities without assigning authority to an agent implicitly.
- State the one-VPS operator-ownership boundary.
- Give deployers a direct path to the deploy explanation and people setting up
  the host a plain-language `Setup on VPS` path to setup documentation.
- Render `tinker deploy . --allow '*@acme.com'` with the wildcard quoted so
  a shell cannot expand it.
- Frame the command as a terminal with a compact title bar and three inert
  window controls. Do not add simulated status output, a generated URL,
  interactive terminal controls, or copy controls.
- Describe the product as `AI-ready` by focusing on the private home it gives
  agent-created tools, without exposing roadmap status or claiming an embedded
  LLM runtime.
- Keep colorful decorative highlights on the page canvas. Cards, content
  surfaces, and controls must not contain decorative highlights.

## Deny-path test charter

- The landing page must not contain a form, credential input, executable
  deployment request, long-lived secret, app ID, or an untyped authorization
  claim.
- The example must not render the unsafe unquoted `--allow *@acme.com` command.
- The terminal must not claim public access, anonymous capabilities, a backend
  runtime, durable realtime history, deployment status, or any planned
  reach-and-insights feature as available.
- The AI-ready message must not imply that LLM execution is included in the
  current runtime.
- Metadata and visible copy must not describe Tinkercloud as a general-purpose
  PaaS or obscure that the operator controls the VPS.
