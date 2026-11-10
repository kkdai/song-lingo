import type { NextConfig } from "next";
import { BACKEND_URL } from "./lib/backend";

const nextConfig: NextConfig = {
  // Allow opening the dev server via this Mac's LAN IP (e.g. from a phone on the same network).
  allowedDevOrigins: ["192.168.50.9"],
  // Every /api/* request without a route here goes to the Go API (backend/). It runs after
  // proxy.ts, so the access check still applies.
  async rewrites() {
    return { beforeFiles: [], afterFiles: [], fallback: [{ source: "/api/:path*", destination: `${BACKEND_URL}/api/:path*` }] };
  },
  experimental: {
    // Re-annotating a song and scoring a recording wait on Gemini for well over the default 30s.
    proxyTimeout: 300_000,
  },
};

export default nextConfig;
