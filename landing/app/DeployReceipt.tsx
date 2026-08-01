"use client";

import { useState } from "react";
import {
  Check,
  CheckCircle,
  Copy,
  LockSimple,
} from "@phosphor-icons/react";

const command = "tinker deploy ./dist --allow '*@acme.com'";
const privateUrl = "https://launch-map.apps.acme.com";

type CopyTarget = "command" | "url";

export function DeployReceipt() {
  const [copied, setCopied] = useState<CopyTarget | null>(null);

  async function copy(value: string, target: CopyTarget) {
    await navigator.clipboard.writeText(value);
    setCopied(target);
    window.setTimeout(() => setCopied(null), 1600);
  }

  return (
    <div className="deploy-receipt" aria-label="Example private deployment receipt">
      <div className="receipt-toolbar">
        <span className="receipt-lights" aria-hidden="true">
          <i />
          <i />
          <i />
        </span>
        <span className="receipt-title">Private deploy</span>
        <span className="receipt-state">
          <CheckCircle size={16} weight="fill" aria-hidden="true" />
          Verified
        </span>
      </div>

      <div className="receipt-body">
        <p className="receipt-context">Example · deployer terminal</p>
        <div className="receipt-command-row deploy-reveal">
          <span className="receipt-prompt" aria-hidden="true">
            $
          </span>
          <code aria-label={command}>
            <span>tinker deploy ./dist</span>
            <span>--allow &apos;*@acme.com&apos;</span>
          </code>
          <CopyButton
            label="Copy deploy command"
            copied={copied === "command"}
            onClick={() => copy(command, "command")}
          />
        </div>

        <div className="receipt-evidence" aria-label="Deployment verification">
          <p className="deploy-reveal deploy-reveal-1">
            <Check size={17} weight="bold" aria-hidden="true" />
            <span>Release activated</span>
          </p>
          <p className="deploy-reveal deploy-reveal-2">
            <Check size={17} weight="bold" aria-hidden="true" />
            <span>Anonymous access denied</span>
          </p>
          <p className="deploy-reveal deploy-reveal-3">
            <Check size={17} weight="bold" aria-hidden="true" />
            <span>
              Allowed viewers <code>*@acme.com</code>
            </span>
          </p>
        </div>

        <div className="receipt-result deploy-reveal deploy-reveal-4">
          <div>
            <span className="receipt-result-label">Private URL</span>
            <code aria-label={privateUrl}>
              <span>https://launch-map.</span>
              <span>apps.acme.com</span>
            </code>
          </div>
          <CopyButton
            label="Copy private URL"
            copied={copied === "url"}
            onClick={() => copy(privateUrl, "url")}
          />
        </div>

        <p className="receipt-footnote">
          <LockSimple size={16} weight="bold" aria-hidden="true" />
          Access is checked before app bytes are served.
        </p>
      </div>
    </div>
  );
}

function CopyButton({
  label,
  copied,
  onClick,
}: {
  label: string;
  copied: boolean;
  onClick: () => void;
}) {
  return (
    <button
      className="receipt-copy"
      type="button"
      aria-label={label}
      onClick={onClick}
    >
      {copied ? (
        <Check size={17} weight="bold" aria-hidden="true" />
      ) : (
        <Copy size={17} weight="bold" aria-hidden="true" />
      )}
      <span aria-live="polite">{copied ? "Copied" : "Copy"}</span>
    </button>
  );
}
