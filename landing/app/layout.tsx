import type { Metadata } from "next";
import { headers } from "next/headers";
import "@fontsource-variable/fredoka";
import "@fontsource-variable/nunito-sans";
import "./globals.css";

export async function generateMetadata(): Promise<Metadata> {
  const requestHeaders = await headers();
  const host =
    requestHeaders.get("x-forwarded-host") ?? requestHeaders.get("host");
  const forwardedProtocol = requestHeaders.get("x-forwarded-proto");
  const protocol =
    forwardedProtocol ?? (host?.startsWith("localhost") ? "http" : "https");
  const origin = host
    ? `${protocol}://${host}`
    : "https://github.com/ChrisMarxDev/tinkercloud";

  return {
    metadataBase: new URL(origin),
    title: "Tinkercloud — Turn small apps into trusted team tools",
    description:
      "Turn dashboards, prototypes, reports, and utilities into trusted private team tools on one VPS you control.",
    openGraph: {
      title: "Tinkercloud — Turn small apps into trusted team tools",
      description:
        "Self-hosted private app deployment with sign-in and access control built in.",
      type: "website",
    },
    twitter: {
      card: "summary",
      title: "Tinkercloud — Turn small apps into trusted team tools",
      description:
        "Self-hosted private app deployment with sign-in and access control built in.",
    },
  };
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
