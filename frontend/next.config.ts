import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  experimental: {
    allowedDevOrigins: ["127.0.0.1:3000", "localhost:3000"],
  },
  async rewrites() {
    return [
      {
        source: "/healthz",
        destination: "/api/health",
      },
    ];
  },
};

export default nextConfig;
