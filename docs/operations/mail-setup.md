# Outbound Mail Setup

Tinkercloud sends one plain-text authentication code through Resend, Postmark,
SendGrid, or an authenticated SMTP submission server. It does not receive mail,
run an SMTP listener, process bounces, or expose provider credentials to apps,
deployers, viewers, or the browser SDK.

## How Tinkercloud selects the adapter

The root-owned systemd environment file at
`/etc/tinkercloud/credentials/tinkercloud.env` is authoritative. On every
process start, Tinkercloud selects the first non-empty provider in this fixed
order:

1. `RESEND_API_KEY`
2. `POSTMARK_SERVER_TOKEN`
3. `SENDGRID_API_KEY`
4. the `TINKERCLOUD_SMTP_*` variable set

The first provider wins; later provider variables are ignored. Keep only the
intended provider's variables in the file so a future operator can see the
choice immediately. The non-secret `email.from` value remains in
`/etc/tinkercloud/config.yaml`.

After changing mail variables, restart and verify the service:

```sh
sudo systemctl restart tinkercloud.service
sudo tinkercloud doctor
```

`doctor` reports only `email_provider: credential accepted` or `unavailable`.
It never prints the selected provider, credential, upstream response, account
metadata, or environment-file path. API adapters perform a read-only credential
request. SMTP negotiates TLS and authenticates without sending a message.

## Common preparation

Before initializing Tinkercloud:

1. Choose a sender such as `Tinkercloud <access@example.com>`.
2. Verify that address or its domain with the provider. Configure the provider's
   DKIM and return-path DNS records for reliable delivery.
3. Create the narrowest send-capable credential offered by the provider.
4. Save the credential as a single line in a root-owned mode-`0600` file. Do not
   put it in a shell command, command-line flag, tracked file, or YAML config.
   The current systemd credential-file contract accepts non-empty values without
   whitespace, NUL, `#`, quotes, or backslashes; generate a dedicated app
   password if the service permits operator-chosen SMTP credentials.
5. Create a separate mode-`0600` file containing at least 32 random bytes for
   `--hmac-key-file`.

The examples below assume `/root/tinkercloud-hmac.key` already exists and use
`example.com` placeholders. Replace all domains, addresses, and file paths with
operator-owned values.

## Resend

1. Add and verify the sending domain in Resend.
2. Create an API key with **Sending access**, restricted to the sending domain
   when possible. Resend displays the key only once.
3. Store it in `/root/tinkercloud-resend.key` with mode `0600`.
4. Initialize:

```sh
sudo tinkercloud init --non-interactive \
  --domain example.com \
  --operator-email operator@example.com \
  --email-provider resend \
  --email-from 'Tinkercloud <access@example.com>' \
  --email-api-key-file /root/tinkercloud-resend.key \
  --hmac-key-file /root/tinkercloud-hmac.key
```

Initialization writes the credential as `RESEND_API_KEY`. Resend setup and
least-privilege key options are documented in the
[Resend API-key guide](https://resend.com/docs/dashboard/api-keys/introduction).

## Postmark

1. Create or select a Postmark server.
2. Verify the sender signature or, preferably, authenticate the sending domain.
3. Open the server's **API Tokens** tab and copy a **Server API token**. Do not
   use an account token.
4. Store it in `/root/tinkercloud-postmark.key` with mode `0600`.
5. Initialize:

```sh
sudo tinkercloud init --non-interactive \
  --domain example.com \
  --operator-email operator@example.com \
  --email-provider postmark \
  --email-from 'Tinkercloud <access@example.com>' \
  --email-api-key-file /root/tinkercloud-postmark.key \
  --hmac-key-file /root/tinkercloud-hmac.key
```

Initialization writes `POSTMARK_SERVER_TOKEN`. Each Postmark server has its own
token; Tinkercloud sends through its default transactional `outbound` stream.
See Postmark's [getting-started guide](https://postmarkapp.com/developer/get-started)
and [sender-signature guidance](https://postmarkapp.com/developer/user-guide/managing-your-account/managing-sender-signatures).

## SendGrid

1. In **Settings → Sender Authentication**, authenticate the sending domain and
   publish the DNS records SendGrid provides. A single-sender identity can work
   for initial testing, but domain authentication is the production path.
2. Create a **Custom Access** API key with the `mail.send` permission. Record the
   value when it is shown; it cannot be retrieved later.
3. Store it in `/root/tinkercloud-sendgrid.key` with mode `0600`.
4. Initialize:

```sh
sudo tinkercloud init --non-interactive \
  --domain example.com \
  --operator-email operator@example.com \
  --email-provider sendgrid \
  --email-from 'Tinkercloud <access@example.com>' \
  --email-api-key-file /root/tinkercloud-sendgrid.key \
  --hmac-key-file /root/tinkercloud-hmac.key
```

Initialization writes `SENDGRID_API_KEY`. Tinkercloud uses SendGrid's global
fixed `https://api.sendgrid.com/v3/mail/send` origin; EU regional subusers are
not currently supported because they require a different origin. See the
[SendGrid domain-authentication guide](https://www.twilio.com/docs/sendgrid/ui/account-and-settings/how-to-set-up-domain-authentication)
and [API-key guide](https://www.twilio.com/docs/sendgrid/ui/account-and-settings/api-keys).

## Generic SMTP submission

Use SMTP when an existing mail service exposes authenticated submission and no
direct adapter is desired. Tinkercloud supports:

- `starttls`: connect in SMTP mode and require a successful STARTTLS upgrade;
  port `587` is the default;
- `tls`: establish implicit TLS before the SMTP greeting; port `465` is common.

Plaintext SMTP, opportunistic TLS, unauthenticated relay, inbound SMTP, custom
certificate authorities, and disabled certificate verification are not
supported. The server certificate must validate for `--smtp-host`. The adapter
uses SMTP `AUTH PLAIN` only after TLS is active.

Obtain the submission host, port, username, password, and TLS mode from the mail
service. Store the password in `/root/tinkercloud-smtp-password` with mode
`0600`, then initialize:

```sh
sudo tinkercloud init --non-interactive \
  --domain example.com \
  --operator-email operator@example.com \
  --email-provider smtp \
  --email-from 'Tinkercloud <access@example.com>' \
  --email-api-key-file /root/tinkercloud-smtp-password \
  --smtp-host smtp.example.net \
  --smtp-port 587 \
  --smtp-username access@example.com \
  --smtp-tls starttls \
  --hmac-key-file /root/tinkercloud-hmac.key
```

Initialization writes exactly these mail variables:

```text
TINKERCLOUD_SMTP_HOST=smtp.example.net
TINKERCLOUD_SMTP_PORT=587
TINKERCLOUD_SMTP_USERNAME=access@example.com
TINKERCLOUD_SMTP_PASSWORD=<secret from the protected file>
TINKERCLOUD_SMTP_TLS=starttls
```

All five variables are required when SMTP is reached in the cascade. A partial
set, invalid port, unsafe host, or TLS value other than `starttls` or `tls`
prevents startup.

## Switching an existing installation

1. Prepare and verify the new provider before touching the running service.
2. Back up the root-owned environment file using the operator's normal secure
   host procedure.
3. Run `sudoedit /etc/tinkercloud/credentials/tinkercloud.env`.
4. Remove the previous provider's mail variables and add the complete new set.
   Preserve `TINKERCLOUD_HMAC_KEY`, `TINKERCLOUD_LLM_ROOT_KEY` when present, and
   all unrelated Tinkercloud secrets exactly.
5. Keep ownership `root:root` and mode `0600`.
6. Restart the service and run `sudo tinkercloud doctor`.
7. Complete a real viewer login to confirm delivery. Doctor deliberately does
   not send test mail.

Do not leave an old higher-priority key in the file. For example,
`RESEND_API_KEY` wins over `SENDGRID_API_KEY` and every SMTP variable.

## Troubleshooting

If `email_provider` is unavailable:

- confirm the intended variable is non-empty and no earlier cascade variable
  remains;
- confirm the sender exactly matches a verified sender or authenticated domain;
- confirm the API key has send permission and has not been revoked;
- for SMTP, confirm all five variables, the TLS mode, the submission port, and
  that the certificate covers the configured host;
- check provider-side activity and rejection logs, which Tinkercloud never
  copies into its own output;
- correct the protected file, restart, and rerun doctor.

Email failure never creates an authentication or recovery bypass. Existing
valid sessions may continue, new email-code login remains unavailable, and the
operator can still use root-only recovery on the VPS.
