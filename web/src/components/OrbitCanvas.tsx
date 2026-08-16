import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Box, Chip, Typography } from "@mui/material";
import type { OrbitNode } from "../types";

interface DrawNode extends OrbitNode {
  px: number;
  py: number;
  radius: number;
  color: string;
}
const colors = [
  "#a99bf8",
  "#7fd4b0",
  "#78b7f1",
  "#f0bd69",
  "#d58fce",
  "#8fcf72",
];
function hash(text: string) {
  let h = 2166136261;
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

export function OrbitCanvas({
  nodes,
  centerName,
  onSelect,
}: {
  nodes: OrbitNode[];
  centerName: string;
  onSelect: (node: OrbitNode) => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ width: 800, height: 600 });
  const [view, setView] = useState({ x: 0, y: 0, zoom: 1 });
  const [hovered, setHovered] = useState<string>();
  const dragging = useRef<
    | { x: number; y: number; viewX: number; viewY: number; didMove: boolean }
    | undefined
  >(undefined);
  const layout = useMemo<DrawNode[]>(() => {
    const scale = Math.min(size.width, size.height) * 0.44;
    const raw = nodes.map((node) => {
      const angle = Math.atan2(node.y, node.x);
      const distance = scale * (0.22 + 0.74 * (1 - node.closeness));
      return {
        ...node,
        px: Math.cos(angle) * distance,
        py: Math.sin(angle) * distance,
        radius: 10 + node.importance * 16,
        color: colors[hash(node.categories[0] ?? node.id) % colors.length],
      };
    });
    for (let pass = 0; pass < 24; pass++)
      for (let i = 0; i < raw.length; i++)
        for (let j = i + 1; j < raw.length; j++) {
          const a = raw[i],
            b = raw[j],
            dx = b.px - a.px,
            dy = b.py - a.py,
            d = Math.max(1, Math.hypot(dx, dy)),
            min = a.radius + b.radius + 14;
          if (d < min) {
            const push = (min - d) / 2 / d;
            a.px -= dx * push;
            a.py -= dy * push;
            b.px += dx * push;
            b.py += dy * push;
          }
        }
    return raw;
  }, [nodes, size]);
  const toScreen = useCallback(
    (x: number, y: number) => ({
      x: size.width / 2 + view.x + x * view.zoom,
      y: size.height / 2 + view.y + y * view.zoom,
    }),
    [size, view],
  );
  const hit = useCallback(
    (x: number, y: number) => {
      for (let i = layout.length - 1; i >= 0; i--) {
        const p = toScreen(layout[i].px, layout[i].py);
        if (Math.hypot(x - p.x, y - p.y) < layout[i].radius * view.zoom + 10)
          return layout[i];
      }
      return undefined;
    },
    [layout, toScreen, view.zoom],
  );
  useEffect(() => {
    if (!wrapRef.current) return;
    const observer = new ResizeObserver(([entry]) =>
      setSize({
        width: Math.max(300, entry.contentRect.width),
        height: Math.max(420, entry.contentRect.height),
      }),
    );
    observer.observe(wrapRef.current);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = size.width * dpr;
    canvas.height = size.height * dpr;
    canvas.style.width = `${size.width}px`;
    canvas.style.height = `${size.height}px`;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, size.width, size.height);
    const grad = ctx.createRadialGradient(
      size.width / 2,
      size.height / 2,
      10,
      size.width / 2,
      size.height / 2,
      Math.max(size.width, size.height) * 0.65,
    );
    grad.addColorStop(0, "rgba(57,49,103,.25)");
    grad.addColorStop(1, "rgba(7,9,21,0)");
    ctx.fillStyle = grad;
    ctx.fillRect(0, 0, size.width, size.height);
    for (let i = 0; i < 80; i++) {
      const x = ((hash("x" + i) % 1000) / 1000) * size.width,
        y = ((hash("y" + i) % 1000) / 1000) * size.height;
      ctx.fillStyle = `rgba(220,216,255,${0.08 + (hash("a" + i) % 20) / 100})`;
      ctx.beginPath();
      ctx.arc(x, y, (hash("r" + i) % 13) / 10 + 0.3, 0, Math.PI * 2);
      ctx.fill();
    }
    const center = toScreen(0, 0);
    for (const ratio of [0.35, 0.62, 0.9]) {
      ctx.strokeStyle = "rgba(169,155,248,.10)";
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.arc(
        center.x,
        center.y,
        Math.min(size.width, size.height) * 0.44 * ratio * view.zoom,
        0,
        Math.PI * 2,
      );
      ctx.stroke();
    }
    for (const node of layout) {
      const p = toScreen(node.px, node.py);
      ctx.strokeStyle =
        node.id === hovered ? "rgba(212,203,255,.58)" : "rgba(169,155,248,.15)";
      ctx.lineWidth = node.id === hovered ? 1.5 : 1;
      ctx.beginPath();
      ctx.moveTo(center.x, center.y);
      ctx.lineTo(p.x, p.y);
      ctx.stroke();
    }
    for (const node of layout) {
      const p = toScreen(node.px, node.py),
        radius = Math.max(7, node.radius * view.zoom);
      ctx.save();
      ctx.shadowBlur = node.id === hovered ? 28 : 14;
      ctx.shadowColor = node.color;
      const g = ctx.createRadialGradient(
        p.x - radius * 0.25,
        p.y - radius * 0.3,
        1,
        p.x,
        p.y,
        radius,
      );
      g.addColorStop(0, "#f7f4ff");
      g.addColorStop(0.22, node.color);
      g.addColorStop(1, "rgba(42,36,75,.95)");
      ctx.fillStyle = g;
      ctx.beginPath();
      ctx.arc(p.x, p.y, radius, 0, Math.PI * 2);
      ctx.fill();
      if (node.momentum > 0.12) {
        ctx.strokeStyle = "rgba(123,226,169,.75)";
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.arc(p.x, p.y, radius + 4, -Math.PI * 0.35, Math.PI * 0.55);
        ctx.stroke();
      }
      ctx.restore();
      if (view.zoom > 0.64 || node.importance > 0.72) {
        ctx.fillStyle = "#f3f1fb";
        ctx.font = `${node.id === hovered ? "700" : "600"} ${Math.max(12, 14 * view.zoom)}px Pretendard, sans-serif`;
        ctx.textAlign = "center";
        ctx.fillText(node.name, p.x, p.y + radius + 18 * view.zoom);
        if (view.zoom > 1.25 && node.label) {
          ctx.fillStyle = "rgba(210,207,225,.72)";
          ctx.font = `${12 * view.zoom}px Pretendard, sans-serif`;
          ctx.fillText(node.label, p.x, p.y + radius + 35 * view.zoom);
        }
      }
    }
    ctx.save();
    ctx.shadowBlur = 30;
    ctx.shadowColor = "#f0c96f";
    ctx.fillStyle = "#f6d981";
    ctx.beginPath();
    ctx.arc(center.x, center.y, 24 * view.zoom, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
    ctx.fillStyle = "#fff8dc";
    ctx.font = `700 ${Math.max(13, 14 * view.zoom)}px Pretendard,sans-serif`;
    ctx.textAlign = "center";
    ctx.fillText(centerName, center.x, center.y + 39 * view.zoom);
  }, [layout, size, toScreen, view.zoom, hovered, centerName]);
  const point = (event: React.PointerEvent) => {
    const rect = event.currentTarget.getBoundingClientRect();
    return { x: event.clientX - rect.left, y: event.clientY - rect.top };
  };
  return (
    <Box
      ref={wrapRef}
      sx={{
        height: { xs: "62vh", md: "calc(100vh - 160px)" },
        minHeight: 480,
        position: "relative",
        overflow: "hidden",
        borderRadius: 4,
        border: "1px solid",
        borderColor: "divider",
        bgcolor: "rgba(5,7,18,.68)",
        touchAction: "none",
      }}
    >
      <canvas
        ref={canvasRef}
        aria-label="관계 우주 지도"
        tabIndex={0}
        onWheel={(e) => {
          e.preventDefault();
          setView((v) => ({
            ...v,
            zoom: Math.max(
              0.55,
              Math.min(2.2, v.zoom * (e.deltaY > 0 ? 0.9 : 1.1)),
            ),
          }));
        }}
        onPointerDown={(e) => {
          const p = point(e);
          dragging.current = {
            x: p.x,
            y: p.y,
            viewX: view.x,
            viewY: view.y,
            didMove: false,
          };
          e.currentTarget.setPointerCapture(e.pointerId);
        }}
        onPointerMove={(e) => {
          const p = point(e);
          const d = dragging.current;
          if (d) {
            const dx = p.x - d.x,
              dy = p.y - d.y;
            if (Math.abs(dx) + Math.abs(dy) > 4) d.didMove = true;
            setView((v) => ({ ...v, x: d.viewX + dx, y: d.viewY + dy }));
          } else setHovered(hit(p.x, p.y)?.id);
        }}
        onPointerUp={(e) => {
          const p = point(e),
            d = dragging.current;
          if (d && !d.didMove) {
            const node = hit(p.x, p.y);
            if (node) onSelect(node);
          }
          dragging.current = undefined;
        }}
        onPointerLeave={() => {
          dragging.current = undefined;
          setHovered(undefined);
        }}
      />
      <Box
        sx={{
          position: "absolute",
          top: 16,
          left: 16,
          display: "flex",
          gap: 1,
          flexWrap: "wrap",
          pointerEvents: "none",
        }}
      >
        <Chip size="small" label={`${nodes.length}개의 행성`} />
        <Chip
          size="small"
          variant="outlined"
          label="스크롤로 확대 · 드래그로 이동"
        />
      </Box>
      <Box
        sx={{
          position: "absolute",
          bottom: 15,
          right: 15,
          display: "flex",
          gap: 0.5,
        }}
      >
        <button
          className="orbit-zoom"
          aria-label="축소"
          onClick={() =>
            setView((v) => ({ ...v, zoom: Math.max(0.55, v.zoom - 0.2) }))
          }
        >
          −
        </button>
        <button
          className="orbit-zoom"
          aria-label="지도 초기화"
          onClick={() => setView({ x: 0, y: 0, zoom: 1 })}
        >
          ◎
        </button>
        <button
          className="orbit-zoom"
          aria-label="확대"
          onClick={() =>
            setView((v) => ({ ...v, zoom: Math.min(2.2, v.zoom + 0.2) }))
          }
        >
          +
        </button>
      </Box>
      <style>{`.orbit-zoom{width:42px;height:42px;border-radius:12px;border:1px solid rgba(255,255,255,.13);background:rgba(18,21,40,.9);color:#f4f3fb;font-size:20px;cursor:pointer}.orbit-zoom:hover{background:#292544}.orbit-zoom:focus-visible{outline:2px solid #a99bf8;outline-offset:2px}`}</style>
      <Typography sx={{ position: "absolute", left: -10000, top: "auto" }}>
        행성을 선택하면 관계 상세 화면으로 이동합니다.
      </Typography>
    </Box>
  );
}
