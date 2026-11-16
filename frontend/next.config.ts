import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  allowedDevOrigins: ["127.0.0.1", "localhost", "127.0.0.1:3000", "localhost:3000"],
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
