import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Allow opening the dev server via this Mac's LAN IP (e.g. from a phone on the same network).
  allowedDevOrigins: ["192.168.50.9"],
  // Loaded with require() at runtime instead of bundled: it reads its gRPC/protobuf files from disk.
  serverExternalPackages: ["@google-cloud/firestore"],
};

export default nextConfig;
