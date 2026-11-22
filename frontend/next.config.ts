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
  allowedDevOrigins: [
    "127.0.0.1",
    "localhost",
    "127.0.0.1:3000",
    "localhost:3000",
    "prueba-mercadopago.testingpage.store",
  ],
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
