import type { NextConfig } from "next";

const stablePreviewDistDir = process.env.ESPACIGO_NEXT_DIST_DIR;

// Observe violations in browser developer tools while validating production flows.
// Next.js uses inline scripts for hydration and inline styles for UI components.
const contentSecurityPolicy = [
  "default-src 'self'",
  "base-uri 'self'",
  "object-src 'none'",
  "frame-ancestors 'none'",
  "form-action 'self'",
  "script-src 'self' 'unsafe-inline' https://sdk.mercadopago.com",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob: https:",
  "font-src 'self' data:",
  "connect-src 'self' https://*.amazonaws.com https://*.mercadopago.com",
  "worker-src 'self' blob:",
  "frame-src 'self' blob: https://*.mercadopago.com",
  "media-src 'self' blob:",
].join("; ");

if (
  stablePreviewDistDir !== undefined &&
  !/^\.next-preview-[A-Za-z0-9-]+$/.test(stablePreviewDistDir)
) {
  throw new Error(
    "ESPACIGO_NEXT_DIST_DIR must name a local preview build directory",
  );
}

const nextConfig: NextConfig = {
  ...(stablePreviewDistDir ? { distDir: stablePreviewDistDir } : {}),
  output: "standalone",
  poweredByHeader: false,
  allowedDevOrigins: [
    "127.0.0.1",
    "localhost",
    "127.0.0.1:3000",
    "localhost:3000",
    "prueba-mercadopago.testingpage.store",
  ],
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          {
            key: "X-Content-Type-Options",
            value: "nosniff",
          },
          {
            key: "X-Frame-Options",
            value: "DENY",
          },
          {
            key: "Referrer-Policy",
            value: "strict-origin-when-cross-origin",
          },
          {
            key: "Permissions-Policy",
            value: "camera=(self), microphone=(), geolocation=(self)",
          },
          ...(process.env.NODE_ENV === "production"
            ? [
                {
                  key: "Content-Security-Policy-Report-Only",
                  value: contentSecurityPolicy,
                },
              ]
            : []),
        ],
      },
    ];
  },
  async rewrites() {
    return [
      {
        source: "/healthz",
        destination: "/api/health",
      },
      {
        source: "/api/v1/:path*",
        destination: "/api/backend/:path*",
      },
    ];
  },
};

export default nextConfig;
