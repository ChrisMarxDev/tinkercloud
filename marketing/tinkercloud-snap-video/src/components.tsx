import type {CSSProperties, ReactNode} from "react";
import {interpolate, spring, useVideoConfig} from "remotion";
import {BODY_FONT, DISPLAY_FONT, MONO_FONT, TINKER} from "./theme";

export const CloudMark: React.FC<{
  size?: number;
  color?: string;
  faceColor?: string;
  shadowColor?: string;
}> = ({
  size = 120,
  color = TINKER.ink,
  faceColor = "#fffbe8",
  shadowColor = TINKER.primary,
}) => {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 512 512"
      fill="none"
      aria-hidden="true"
    >
      <g transform="rotate(-1.5 256 256)">
        <path
          d="M102 350C72 329 56 298 59 264C62 222 98 190 144 189C157 134 204 98 260 98C319 98 367 137 379 192C422 193 457 225 462 266C468 315 433 353 374 365C294 381 191 378 126 360C117 358 109 354 102 350Z"
          fill={shadowColor}
          transform="translate(18 15)"
        />
        <path
          d="M102 350C72 329 56 298 59 264C62 222 98 190 144 189C157 134 204 98 260 98C319 98 367 137 379 192C422 193 457 225 462 266C468 315 433 353 374 365C294 381 191 378 126 360C117 358 109 354 102 350Z"
          fill={color}
        />
        <rect x="158" y="258" width="24" height="26" rx="7" fill={faceColor} />
        <rect x="200" y="258" width="24" height="26" rx="7" fill={faceColor} />
        <path
          d="M181 298C187 305 196 305 202 298C204 296 207 297 208 300C209 302 208 304 206 306C198 314 185 314 178 306C176 304 176 301 178 299C179 298 180 298 181 298Z"
          fill={faceColor}
        />
      </g>
    </svg>
  );
};

export const Grain: React.FC<{opacity?: number}> = ({opacity = 0.08}) => {
  return (
    <svg
      style={{
        position: "absolute",
        inset: 0,
        width: "100%",
        height: "100%",
        opacity,
        pointerEvents: "none",
        mixBlendMode: "soft-light",
      }}
      viewBox="0 0 1920 1080"
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      <filter id="tinker-noise">
        <feTurbulence
          type="fractalNoise"
          baseFrequency="0.72"
          numOctaves="4"
          stitchTiles="stitch"
        />
      </filter>
      <rect width="1920" height="1080" filter="url(#tinker-noise)" opacity="0.52" />
    </svg>
  );
};

export const Pill: React.FC<{
  children: ReactNode;
  dark?: boolean;
  style?: CSSProperties;
}> = ({children, dark = false, style}) => {
  return (
    <div
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 12,
        padding: "12px 20px",
        borderRadius: 999,
        color: dark ? "#fffef8" : TINKER.ink,
        backgroundColor: dark ? "rgba(255,255,255,0.12)" : TINKER.surface,
        border: `2px solid ${
          dark ? "rgba(255,255,255,0.18)" : TINKER.line
        }`,
        fontFamily: BODY_FONT,
        fontSize: 32,
        fontWeight: 800,
        letterSpacing: "0.03em",
        ...style,
      }}
    >
      {children}
    </div>
  );
};

export const Window: React.FC<{
  children: ReactNode;
  width?: number;
  height?: number;
  dark?: boolean;
  style?: CSSProperties;
}> = ({children, width = 920, height = 570, dark = false, style}) => {
  return (
    <div
      style={{
        width,
        height,
        overflow: "hidden",
        borderRadius: 34,
        border: `2px solid ${
          dark ? "rgba(255,255,255,0.14)" : TINKER.line
        }`,
        backgroundColor: dark ? "#12043f" : TINKER.surfaceRaised,
        boxShadow: dark
          ? "0 34px 90px rgba(8, 1, 34, 0.5)"
          : "0 28px 0 rgba(32,6,117,0.08), 0 48px 100px rgba(32,6,117,0.13)",
        ...style,
      }}
    >
      <div
        style={{
          height: 64,
          display: "flex",
          alignItems: "center",
          padding: "0 26px",
          gap: 12,
          borderBottom: `2px solid ${
            dark ? "rgba(255,255,255,0.11)" : TINKER.line
          }`,
          backgroundColor: dark ? "#19064f" : "#f7f2e9",
        }}
      >
        {[TINKER.pink, TINKER.yellow, TINKER.mint].map((color) => (
          <div
            key={color}
            style={{width: 15, height: 15, borderRadius: 999, background: color}}
          />
        ))}
      </div>
      {children}
    </div>
  );
};

export const FileTile: React.FC<{
  name: string;
  tone: "blue" | "pink" | "yellow" | "mint";
  style?: CSSProperties;
}> = ({name, tone, style}) => {
  const colors = {
    blue: {bg: TINKER.primarySoft, ink: TINKER.primary},
    pink: {bg: "#ffe6f2", ink: TINKER.pink},
    yellow: {bg: "#fff3bd", ink: "#7e5600"},
    mint: {bg: TINKER.mint, ink: TINKER.positive},
  } as const;
  const selected = colors[tone];

  return (
    <div
      style={{
        width: 260,
        height: 168,
        display: "flex",
        flexDirection: "column",
        justifyContent: "space-between",
        padding: 22,
        borderRadius: 26,
        border: "2px solid rgba(32,6,117,0.12)",
        background: selected.bg,
        boxShadow: "0 14px 0 rgba(32,6,117,0.11)",
        color: selected.ink,
        fontFamily: MONO_FONT,
        fontSize: 32,
        fontWeight: 800,
        ...style,
      }}
    >
      <svg width="45" height="54" viewBox="0 0 45 54" fill="none">
        <path
          d="M7 2H28L42 16V47C42 50 40 52 37 52H7C4 52 2 50 2 47V7C2 4 4 2 7 2Z"
          fill="white"
          stroke="currentColor"
          strokeWidth="4"
        />
        <path d="M28 2V16H42" stroke="currentColor" strokeWidth="4" />
      </svg>
      <span>{name}</span>
    </div>
  );
};

export const LockIcon: React.FC<{
  size?: number;
  color?: string;
  background?: string;
}> = ({size = 100, color = "#fffef8", background = TINKER.positive}) => {
  return (
    <div
      style={{
        width: size,
        height: size,
        display: "grid",
        placeItems: "center",
        borderRadius: size * 0.3,
        color,
        background,
        boxShadow: "0 14px 0 rgba(32,6,117,0.12)",
      }}
    >
      <svg width={size * 0.54} height={size * 0.58} viewBox="0 0 54 58">
        <path
          d="M13 24V17C13 9 19 3 27 3C35 3 41 9 41 17V24"
          fill="none"
          stroke="currentColor"
          strokeWidth="6"
          strokeLinecap="round"
        />
        <rect x="5" y="22" width="44" height="33" rx="10" fill="currentColor" />
        <circle cx="27" cy="38" r="4" fill={background} />
      </svg>
    </div>
  );
};

export const Caption: React.FC<{
  index: string;
  title: string;
  detail?: string;
  frame: number;
  enterAt: number;
  dark?: boolean;
  style?: CSSProperties;
}> = ({index, title, detail, frame, enterAt, dark = false, style}) => {
  const {fps} = useVideoConfig();
  const reveal = spring({
    frame: frame - enterAt,
    fps,
    config: {damping: 16, stiffness: 170, mass: 0.75},
  });
  const y = interpolate(reveal, [0, 1], [44, 0]);

  return (
    <div
      style={{
        position: "absolute",
        left: 100,
        bottom: 82,
        color: dark ? "#fffef8" : TINKER.ink,
        fontFamily: BODY_FONT,
        transform: `translateY(${y}px)`,
        opacity: reveal,
        ...style,
      }}
    >
      <div
        style={{
          marginBottom: 14,
          fontSize: 32,
          fontWeight: 900,
          letterSpacing: "0.16em",
          color: dark ? TINKER.yellow : TINKER.primary,
        }}
      >
        {index}
      </div>
      <div
        style={{
          fontFamily: DISPLAY_FONT,
          fontSize: 66,
          lineHeight: 0.95,
          fontWeight: 900,
          letterSpacing: "-0.055em",
        }}
      >
        {title}
      </div>
      {detail ? (
        <div
          style={{
            marginTop: 18,
            maxWidth: 520,
            color: dark ? "rgba(255,255,255,0.68)" : TINKER.muted,
            fontSize: 32,
            lineHeight: 1.25,
            fontWeight: 650,
          }}
        >
          {detail}
        </div>
      ) : null}
    </div>
  );
};

export const BrandBug: React.FC<{dark?: boolean}> = ({dark = false}) => {
  return (
    <div
      style={{
        position: "absolute",
        top: 54,
        left: 74,
        zIndex: 30,
        display: "flex",
        alignItems: "center",
        gap: 10,
        color: dark ? "#fffef8" : TINKER.ink,
        fontFamily: DISPLAY_FONT,
        fontSize: 32,
        fontWeight: 900,
        letterSpacing: "-0.05em",
      }}
    >
      <CloudMark
        size={54}
        color={dark ? "#fffef8" : TINKER.ink}
        faceColor={dark ? TINKER.ink : "#fffbe8"}
        shadowColor={TINKER.primary}
      />
      tinkercloud
    </div>
  );
};
