import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Allow opening the dev server via this Mac's LAN IP (e.g. from a phone on the same network).
  allowedDevOrigins: ["192.168.50.9"],
};

export default nextConfig;
