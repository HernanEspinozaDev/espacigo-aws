import type { NextConfig } from "next";

const stablePreviewDistDir = process.env.ESPACIGO_NEXT_DIST_DIR;

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
            value: "camera=(), microphone=(), geolocation=(self)",
          },
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
