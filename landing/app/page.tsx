import Image from "next/image";
import {
  ArrowDown,
  ArrowRight,
  ArrowUpRight,
  BookOpenText,
  CaretDown,
  CheckCircle,
  Circle,
  CloudArrowUp,
  Code,
  Database,
  Diamond,
  FileCssIcon,
  FileHtmlIcon,
  FileJsIcon,
  FolderOpen,
  GithubLogo,
  HardDrives,
  Lightning,
  LockSimpleIcon,
  Sparkle,
  StarFour,
  TerminalWindow,
  UserCircle,
} from "@phosphor-icons/react/dist/ssr";
import { CodeExample } from "./CodeExample";

const steps = [
  {
    number: "1",
    icon: FolderOpen,
    title: "Build your app",
    body: "Build any static site or web app. Export the files to a folder.",
  },
  {
    number: "2",
    icon: TerminalWindow,
    title: "Deploy to your VPS",
    body: "Run one Tinker command with your allow list. Tinkercloud safely activates the release on your VPS.",
  },
  {
    number: "3",
    icon: LockSimpleIcon,
    title: "Share the private URL",
    body: "The URL is on the open internet, but only your allowed team can open it.",
  },
];

const essentials = [
  {
    icon: UserCircle,
    title: "Auth",
    body: "Email sign-in and app-scoped access.",
  },
  {
    icon: Database,
    title: "Storage",
    body: "App-scoped JSON key-value storage.",
  },
  {
    icon: Lightning,
    title: "Live sockets",
    body: "Ephemeral realtime updates.",
  },
  {
    icon: Code,
    title: "Client SDK",
    body: "One small, typed browser SDK.",
  },
  {
    icon: CloudArrowUp,
    title: "Blob storage",
    body: "Private, app-scoped files with bounded uploads.",
  },
  {
    icon: Sparkle,
    title: "LLM capability",
    body: "Operator-governed AI access without browser secrets.",
  },
];

function Brand({ footer = false }: { footer?: boolean }) {
  return (
    <span className={`brand ${footer ? "brand-footer" : ""}`}>
      <span className="brand-logo" aria-hidden="true">
        <Image
          src="/tinkercloud-mark.svg"
          alt=""
          width={72}
          height={72}
          priority={!footer}
        />
      </span>
      <span>tinkercloud</span>
    </span>
  );
}

function Flourishes({
  area,
}: {
  area: "hero" | "how" | "essentials" | "code" | "hosting";
}) {
  return (
    <div className={`flourishes flourishes-${area}`} aria-hidden="true">
      <Sparkle className="flourish flourish-a" weight="fill" />
      <Circle className="flourish flourish-b" weight="fill" />
      <Diamond className="flourish flourish-c" weight="fill" />
      <StarFour className="flourish flourish-d" weight="fill" />
    </div>
  );
}

export default function Home() {
  return (
    <main>
      <nav className="nav" aria-label="Primary navigation">
        <a href="#" aria-label="Tinkercloud home">
          <Brand />
        </a>
        <a className="nav-link" href="#how-it-works">
          How it works
          <CaretDown size={16} weight="bold" aria-hidden="true" />
        </a>
      </nav>

      <section className="hero" aria-labelledby="hero-title">
        <Flourishes area="hero" />
        <div className="hero-copy">
          <p className="private-pill">
            <LockSimpleIcon size={17} weight="bold" aria-hidden="true" />
            Private by default
          </p>
          <h1 id="hero-title">
            Your
            <br />
            small apps.
            <br />
            <span>Securely</span>
            <br />
            <span>shared.</span>
          </h1>
          <p className="lede">
            Tinkercloud is an open-source platform you host on your own VPS. Your
            apps live on the open internet, but Tinkercloud securely gates them
            so only your team can get in.
          </p>
          <div className="hero-facts" aria-label="Tinkercloud ownership">
            <span className="hero-fact">
              <HardDrives size={18} weight="bold" aria-hidden="true" />
              Runs on your VPS
            </span>
            <a
              className="hero-fact hero-fact-link"
              href="https://github.com/ChrisMarxDev/tinkercloud"
              target="_blank"
              rel="noreferrer"
              title="Repository is private for now"
            >
              <GithubLogo size={18} weight="fill" aria-hidden="true" />
              Open source on GitHub
            </a>
          </div>
          <a className="button" href="#how-it-works">
            See how it works
            <ArrowDown size={19} weight="bold" aria-hidden="true" />
          </a>
        </div>

        <div className="deploy-flow" aria-label="Tinkercloud deployment flow">
          <div className="app-folder">
            <FolderOpen size={112} weight="fill" aria-hidden="true" />
            <div className="file-icons" aria-hidden="true">
              <span>
                <FileHtmlIcon size={36} weight="bold" />
              </span>
              <span>
                <FileCssIcon size={36} weight="bold" />
              </span>
              <span>
                <FileJsIcon size={36} weight="bold" />
              </span>
            </div>
          </div>
          <ArrowDown className="flow-arrow" size={42} weight="bold" aria-hidden="true" />
          <div className="command-card">
            <TerminalWindow size={26} weight="bold" aria-hidden="true" />
            <code>
              <span>tinker deploy ./dist</span>
              <span className="allow-flag">{" --allow *@acme.com"}</span>
            </code>
          </div>
          <ArrowDown className="flow-arrow" size={42} weight="bold" aria-hidden="true" />
          <div className="private-url">
            <span className="lock-bubble">
              <LockSimpleIcon size={24} weight="bold" aria-hidden="true" />
            </span>
            <code>project.apps.example</code>
            <CheckCircle size={34} weight="fill" aria-hidden="true" />
          </div>
        </div>

      <div className="how" id="how-it-works" aria-labelledby="how-title">
        <Flourishes area="how" />
        <div className="section-heading">
          <p className="eyebrow">How it works</p>
          <h2 id="how-title">From folder to private.</h2>
        </div>

        <ol className="steps">
          {steps.map((step) => {
            const StepIcon = step.icon;
            return (
              <li key={step.number}>
                <span className="step-number">{step.number}</span>
                <div className="step-visual">
                  <StepIcon size={58} weight="bold" aria-hidden="true" />
                </div>
                <h3>{step.title}</h3>
                <p>{step.body}</p>
                {step.number !== "3" && (
                  <ArrowRight
                    className="step-arrow"
                    size={32}
                    weight="bold"
                    aria-hidden="true"
                  />
                )}
              </li>
            );
          })}
        </ol>
      </div>
    </section>

      <section className="essentials" aria-labelledby="essentials-title">
        <Flourishes area="essentials" />
        <p className="eyebrow">Ready when your app is</p>
        <h2 id="essentials-title">
          The small essentials.
          <br />
          Already inside.
        </h2>
        <div className="essential-list">
          {essentials.map((essential) => {
            const EssentialIcon = essential.icon;
            return (
              <article key={essential.title}>
                <EssentialIcon size={48} weight="bold" aria-hidden="true" />
                <h3>{essential.title}</h3>
                <p>{essential.body}</p>
              </article>
            );
          })}
        </div>
      </section>

      <section className="code-section" aria-labelledby="code-title">
        <Flourishes area="code" />
        <div className="code-copy">
          <p className="eyebrow">One Tinkercloud SDK</p>
          <h2 id="code-title">Useful from line one.</h2>
          <p>
            Identity, storage, and live updates share one typed, app-scoped API.
            No app ID or long-lived secret in your browser code.
          </p>
        </div>
        <CodeExample />
      </section>

      <aside className="honest-note" aria-label="Tinkercloud V1 limits">
        <LockSimpleIcon size={24} weight="bold" aria-hidden="true" />
        <p>
          <strong>Small, honest limits.</strong> Built for replaceable tools and
          prototypes. KV stores current recovery state; live events are
          ephemeral and may be lost across disconnects.
        </p>
      </aside>

      <section className="hosting" aria-labelledby="hosting-title">
        <Flourishes area="hosting" />
        <span className="hosting-icon" aria-hidden="true">
          <BookOpenText size={30} weight="bold" />
        </span>
        <div className="hosting-copy">
          <p className="eyebrow">How to host it</p>
          <h2 id="hosting-title">One VPS. One Tinkercloud.</h2>
          <p>
            One VPS is a deliberate constraint, not a shortcut. The gateway,
            access policy, releases, and app data stay together in one small
            system an operator can understand, secure, and recover—without
            assembling Docker, a reverse proxy, or a separate database service.
            The README walks through setup, DNS, and your first private
            deployment.
          </p>
        </div>
        <a
          className="hosting-link"
          href="https://github.com/ChrisMarxDev/tinkercloud#readme"
          target="_blank"
          rel="noreferrer"
          title="Repository is private for now"
        >
          Read the README
          <ArrowUpRight size={18} weight="bold" aria-hidden="true" />
        </a>
      </section>

      <footer>
        <Brand footer />
        <div className="footer-meta">
          <p>Open source. Your VPS. Your Tinkercloud.</p>
          <a
            className="github-link"
            href="https://github.com/ChrisMarxDev/tinkercloud"
            target="_blank"
            rel="noreferrer"
            title="Repository is private for now"
          >
            <GithubLogo size={18} weight="fill" aria-hidden="true" />
            View on GitHub
          </a>
        </div>
      </footer>
    </main>
  );
}
