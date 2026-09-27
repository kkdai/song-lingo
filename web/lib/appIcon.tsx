import { ImageResponse } from "next/og";

export const THEME_COLOR = "#f59e0b";

/**
 * Song Lingo app icon: a white eighth note on an amber square, drawn with plain shapes so it
 * needs no font or image assets. `maskable` shrinks the note into the safe zone Android crops to.
 */
export function renderAppIcon(size: number, { maskable = false } = {}) {
  const s = size / 100; // design on a 100-unit grid
  const scale = maskable ? 0.7 : 1;
  const u = (n: number) => n * s * scale;
  return new ImageResponse(
    (
      <div
        style={{
          width: size,
          height: size,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: THEME_COLOR,
          borderRadius: maskable ? 0 : size * 0.22,
        }}
      >
        <div style={{ position: "relative", width: u(46), height: u(62), display: "flex" }}>
          {/* stem */}
          <div style={{ position: "absolute", left: u(22), top: 0, width: u(7), height: u(50), background: "white" }} />
          {/* flag */}
          <div
            style={{
              position: "absolute",
              left: u(22),
              top: 0,
              width: u(22),
              height: u(16),
              background: "white",
              borderTopRightRadius: u(12),
              borderBottomRightRadius: u(4),
            }}
          />
          {/* note head */}
          <div
            style={{
              position: "absolute",
              left: 0,
              top: u(40),
              width: u(29),
              height: u(22),
              background: "white",
              borderRadius: "50%",
            }}
          />
        </div>
      </div>
    ),
    { width: size, height: size },
  );
}
