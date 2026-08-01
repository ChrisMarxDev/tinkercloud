const command = "tinker deploy . --allow '*@acme.com'";

export function DeployCommand() {
  return (
    <div className="deploy-command">
      <span className="deploy-prompt" aria-hidden="true">
        $
      </span>
      <code aria-label={command}>
        <span>tinker deploy .</span>
        <span>--allow &apos;*@acme.com&apos;</span>
      </code>
    </div>
  );
}
