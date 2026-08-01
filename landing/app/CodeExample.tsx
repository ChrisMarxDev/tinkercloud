"use client";

import { useState } from "react";
import { Check, Copy } from "@phosphor-icons/react";

const example = `import { tinker } from "@tinkercloud/sdk";
const viewer = await tinker.user.current();
await tinker.kv.set("hello", { from: viewer.identity.email });
tinker.live.onKvChange({ prefix: "hello" }, console.log);`;

export function CodeExample() {
  const [copied, setCopied] = useState(false);

  async function copyExample() {
    await navigator.clipboard.writeText(example);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  }

  return (
    <div className="code-card">
      <div className="code-toolbar">
        <span>TypeScript</span>
        <button type="button" onClick={copyExample} aria-label="Copy code example">
          {copied ? (
            <Check size={20} weight="bold" aria-hidden="true" />
          ) : (
            <Copy size={20} weight="bold" aria-hidden="true" />
          )}
          <span aria-live="polite">{copied ? "Copied" : "Copy"}</span>
        </button>
      </div>
      <pre>
        <code>
          <span className="line">
            <span className="line-number">1</span>
            <span className="keyword">import</span> {"{ tinker }"}{" "}
            <span className="keyword">from</span>{" "}
            <span className="string">&quot;@tinkercloud/sdk&quot;</span>;
          </span>
          <span className="line">
            <span className="line-number">2</span>
            <span className="keyword">const</span> viewer ={" "}
            <span className="keyword">await</span> tinker.user.current();
          </span>
          <span className="line">
            <span className="line-number">3</span>
            <span className="keyword">await</span> tinker.kv.set(
            <span className="string">&quot;hello&quot;</span>, {"{ from: "}
            viewer.identity.email {"}"});
          </span>
          <span className="line">
            <span className="line-number">4</span>
            tinker.live.onKvChange({"{ prefix: "}
            <span className="string">&quot;hello&quot;</span> {"}"}, console.log);
          </span>
        </code>
      </pre>
    </div>
  );
}
