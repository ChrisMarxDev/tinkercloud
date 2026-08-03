const command = "tinker deploy . --allow '*@example.com'";

export function DeployCommand() {
  return (
    <div className="deploy-command">
      <div className="terminal-toolbar" aria-hidden="true">
        <span className="terminal-lights">
          <i />
          <i />
          <i />
        </span>
      </div>
      <div className="terminal-command-line">
        <span className="deploy-prompt" aria-hidden="true">
          $
        </span>
        <code aria-label={command}>
          <span>tinker deploy .</span>
          <span>--allow &apos;*@example.com&apos;</span>
        </code>
      </div>
    </div>
  );
}
