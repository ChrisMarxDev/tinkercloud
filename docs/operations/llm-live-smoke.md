# Gemini LLM live smoke

This is a manual, bounded post-V1 acceptance run for the operator-owned
`llm.chat` adapter. It is separate from the ordinary VPS suite: it makes one
real bounded chat completion and must use a disposable app and a low token
budget. API-key verification is a separate provider credential-validation
request; it is not a chat completion.

Read the [LLM chat contract](../../specs/capabilities/llm-chat-contract.md)
before running it. The operator owns the provider key and performs the
dashboard mutations; the deployer owns the sample app; the viewer proves only
app-scoped access. Never paste the key into a terminal, source file, browser
console, issue, chat, or test output.

## Bounds

- Use a disposable private app named `llm-chat` and a viewer email that is
  explicitly allowed by its normal app policy.
- Use the Gemini model `gemini-2.5-flash` unless the operator's Gemini account
  shows it is unavailable; it is a stable low-cost text model at the time this
  procedure was written. Do not use an image, tool, search, or preview model.
- Set a profile with one message, 256 bytes per message and request, 16 maximum
  output tokens, a 10-second timeout, one viewer request and one app request
  per 60 seconds, concurrency one, and a monthly budget of 64 tokens.
- Send exactly one harmless request: `Reply with exactly: tinkercloud smoke`.
  Do not retry on a provider, quota, or network failure. Record only outcome,
  status, and timestamp—not provider responses, cookies, prompts, or keys.

## Prepare the app

From the repository checkout, build and deploy the existing fixture as its
deployer. The manifest contains no LLM declaration:

```sh
cd examples/sdk-apps
npm run build
cd llm-chat
tinker deploy .
```

Open `https://llm-chat.<domain>` as its allowed viewer. Before the server has a
usable default profile, the page must remain usable but show the safe capability-unavailable
state with Send disabled. In browser DevTools, or through the app's network
request, confirm its same-origin `/_tinker/api/v1/capabilities` response does
not contain `llm.chat`. Do not copy session cookies into another tool.

## Configure through the dashboard

If **API keys** is unavailable, the operator runs on the VPS:

```sh
sudo tinkercloud llm enable
sudo systemctl restart tinkercloud.service
```

Then, as the operator at `https://admin.<domain>/dashboard`:

1. In **API keys**, select **Gemini**, enter the key once in the password
   field, and choose **Verify and save API key**. Reload the dashboard. It may
   show the safe label `Gemini API key`, opaque ID, and status, but must never
   show the entered key or a provider response.
2. In **LLM chat**, create the bounded profile above using that active
   connection and `gemini-2.5-flash`. The first active profile becomes the
   server default automatically; for later profiles choose **Use as default**.
3. Leave the host allowance unlimited for this single request and confirm the
   `llm-chat` app policy is enabled and inherits the host setting.

Reload the sample app as the allowed viewer. Its same-origin capability
discovery must now contain `llm.chat` and a disclosure plus numeric safe
limits, but no provider name, model, connection ID, key, or endpoint. Send the
single bounded smoke request. A successful response must show the fixed text
and leave the dashboard usable; audit/usage may show safe outcome and token
counts only.

## Required denial matrix

Run these after the one allowed call. They must have no provider details in the
body or UI and must not result in a second provider invocation.

| Case | Expected evidence |
| --- | --- |
| Anonymous `/_tinker/api/v1/capabilities` and `/_tinker/api/v1/llm/chat` | `401`; no `llm.chat`, app bytes, or provider details. |
| Allowed viewer while no usable default profile exists | Discovery omits `llm.chat`; chat returns `403` safe capability-unavailable response. |
| Wrong app/session | `401` or `403`; no `llm.chat`, policy, profile, connection, provider, or provider response. |
| Disable `llm-chat` in its dashboard LLM policy | Reloaded discovery omits `llm.chat`; chat is denied on the next request. |
| Disable the Gemini connection | Reloaded discovery omits `llm.chat`; chat is denied on the next request. |

For operator mutations, use only the rendered dashboard controls. Do not guess
IDs or send raw form requests. Re-enable only if a later test deliberately needs
a new bounded run; otherwise leave the app policy or connection disabled, or
delete the disposable app through its normal owner controls.

## Offline gates

Before the live run, execute:

```sh
go test ./test/integration -run TestLLMGatewaySQLiteTwoAppBoundary -count=1
go test ./internal/controlapi ./internal/persistence ./internal/llm/gemini
npm --prefix examples/sdk-apps run check
```

These cover the no-provider two-app boundary, write-only dashboard controls,
encrypted connection/profile/policy behavior, Gemini adapter conformance, and
the packaged sample. They are necessary evidence, not a substitute for the
single real-provider call.
