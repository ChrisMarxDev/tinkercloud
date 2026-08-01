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
    title: "Tinkercloud — Your small apps, securely shared",
    description:
      "Turn a folder of static files into a private, secure URL. Tinkercloud handles hosting, sign-in, and access control.",
    openGraph: {
      title: "Tinkercloud — Your small apps, securely shared",
      description:
        "Private static app hosting with sign-in and access control built in.",
      type: "website",
      images: [
        {
          url: "/og.png",
          width: 1733,
          height: 909,
          alt: "Tinkercloud — Your small apps, securely shared",
        },
      ],
    },
    twitter: {
      card: "summary_large_image",
      title: "Tinkercloud — Your small apps, securely shared",
      description:
        "Private static app hosting with sign-in and access control built in.",
      images: ["/og.png"],
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
