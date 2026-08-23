import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Box, Chip, Typography } from "@mui/material";
import type { OrbitNode } from "../types";
import {
  constellationEdges,
  nebulaRadius,
  readGrammar,
  STATE_META,
  STATE_ORDER,
  type Grammar,
  type RelationState,
} from "../orbitGrammar";

interface DrawNode extends OrbitNode {
  px: number;
  py: number;
  radius: number;
  color: string;
  grammar: Grammar;
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

const COMET_PERIOD = 9000;

/**
 * Memory Comet — 오래된 기억이 화면 밖에서 그 사람의 행성을 향해 날아옵니다.
 * Rediscover를 화면 위쪽 카드가 아니라 우주의 사건으로 보여주기 위한 장치입니다.
 */
function drawComet(
  ctx: CanvasRenderingContext2D,
  target: DrawNode,
  toScreen: (x: number, y: number) => { x: number; y: number },
  zoom: number,
  scale: number,
  elapsed: number,
) {
  const angle = Math.atan2(target.py, target.px);
  const distance = Math.hypot(target.px, target.py) || 1;
  // 목표 행성 바깥에서 크게 휘어 들어오는 접근 궤도.
  const from = {
    x: Math.cos(angle + 0.62) * (distance + scale * 1.05),
    y: Math.sin(angle + 0.62) * (distance + scale * 1.05),
  };
  const control = {
    x: Math.cos(angle + 0.95) * (distance + scale * 0.3),
    y: Math.sin(angle + 0.95) * (distance + scale * 0.3),
  };
  const at = (t: number) => {
    const inv = 1 - t;
    return toScreen(
      inv * inv * from.x + 2 * inv * t * control.x + t * t * target.px,
      inv * inv * from.y + 2 * inv * t * control.y + t * t * target.py,
    );
  };
  const t = Math.min(1, ((elapsed % COMET_PERIOD) / COMET_PERIOD) * 1.12);
  const head = at(t);
  ctx.save();
  // 꼬리는 지나온 궤적을 따라 옅어집니다.
  for (let i = 1; i <= 16; i++) {
    const back = t - i * 0.018;
    if (back < 0) break;
    const p = at(back);
    ctx.fillStyle = `rgba(246,217,129,${0.4 * (1 - i / 16)})`;
    ctx.beginPath();
    ctx.arc(p.x, p.y, Math.max(0.6, (3.4 - i * 0.19) * zoom), 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.shadowBlur = 16;
  ctx.shadowColor = "#f6d981";
  ctx.fillStyle = "#fff6dc";
  ctx.beginPath();
  ctx.arc(head.x, head.y, 3.6 * zoom, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();
  // 도착 무렵에는 행성이 함께 울립니다.
  const landing = Math.max(0, (t - 0.78) / 0.22);
  const planet = toScreen(target.px, target.py);
  const radius = Math.max(7, target.radius * zoom);
  if (landing > 0) {
    ctx.strokeStyle = `rgba(246,217,129,${0.55 * (1 - landing)})`;
    ctx.lineWidth = 1.6;
    ctx.beginPath();
    ctx.arc(planet.x, planet.y, radius + 6 + landing * 16, 0, Math.PI * 2);
    ctx.stroke();
  }
  ctx.fillStyle = "rgba(246,217,129,.85)";
  ctx.font = `700 ${Math.max(9, 10 * zoom)}px Pretendard, sans-serif`;
  ctx.textAlign = "center";
  ctx.fillText("REDISCOVER", planet.x, planet.y - radius - 12 * zoom);
}

export function OrbitCanvas({
  nodes,
  centerName,
  onSelect,
  constellation,
  rediscover,
}: {
  nodes: OrbitNode[];
  centerName: string;
  onSelect: (node: OrbitNode) => void;
  /** 선택된 별자리(카테고리). 해당 인물들이 선으로 이어지고 나머지는 물러납니다. */
  constellation?: string;
  /** 다시 꺼내볼 기억. 그 사람의 행성으로 향하는 혜성으로 나타납니다. */
  rediscover?: { person_id: string; title: string };
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
  const scale = Math.min(size.width, size.height) * 0.37;
  const layout = useMemo<DrawNode[]>(() => {
    const now = Date.now();
    const raw = nodes.map((node) => {
      const grammar = readGrammar(node, now);
      const angle = Math.atan2(node.y, node.x);
      // 오래 교류가 없는 관계는 closeness와 무관하게 Event Horizon 밖에 놓입니다.
      const distance =
        grammar.state === "dormant"
          ? scale * (1.06 + 0.12 * ((hash(node.id) % 100) / 100))
          : scale * (0.22 + 0.68 * (1 - node.closeness));
      return {
        ...node,
        grammar,
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
  }, [nodes, scale]);
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
    const reduceMotion =
      document.documentElement.dataset.reduceMotion === "true";
    const comet = rediscover
      ? layout.find((node) => node.id === rediscover.person_id)
      : undefined;
    const draw = (elapsed: number) => {
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
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
      const ring = (ratio: number) => scale * ratio * view.zoom;
      // 궤도권역: 안쪽부터 Inner / Stable / Outer, 그 바깥이 Event Horizon.
      const bandLabel = (text: string, y: number, color: string) => {
        ctx.font = `600 ${Math.max(9, 10 * view.zoom)}px Pretendard, sans-serif`;
        ctx.textAlign = "center";
        const width = ctx.measureText(text).width + 10;
        ctx.fillStyle = "rgba(7,9,21,.72)";
        ctx.beginPath();
        ctx.roundRect(center.x - width / 2, y - 9, width, 13, 6);
        ctx.fill();
        ctx.fillStyle = color;
        ctx.fillText(text, center.x, y);
      };
      const bands: [number, string][] = [
        [0.35, "INNER"],
        [0.62, "STABLE"],
        [0.9, "OUTER"],
      ];
      for (const [ratio, name] of bands) {
        ctx.strokeStyle = "rgba(169,155,248,.10)";
        ctx.lineWidth = 1;
        ctx.setLineDash([]);
        ctx.beginPath();
        ctx.arc(center.x, center.y, ring(ratio), 0, Math.PI * 2);
        ctx.stroke();
        if (view.zoom > 0.7)
          bandLabel(name, center.y - ring(ratio) - 6, "rgba(169,155,248,.42)");
      }
      // Event Horizon — 이 선 밖은 오래 교류가 끊긴 Dark Orbit입니다.
      const horizon = ring(0.99);
      ctx.strokeStyle = "rgba(124,134,168,.34)";
      ctx.lineWidth = 1.2;
      ctx.setLineDash([5, 6]);
      ctx.beginPath();
      ctx.arc(center.x, center.y, horizon, 0, Math.PI * 2);
      ctx.stroke();
      ctx.setLineDash([]);
      if (layout.some((node) => node.grammar.state === "dormant")) {
        bandLabel("EVENT HORIZON", center.y - horizon - 7, "rgba(150,160,196,.62)");
        bandLabel("DARK ORBIT", center.y + horizon + 17, "rgba(124,134,168,.5)");
      }
      const lit = (node: DrawNode) =>
        !constellation || node.categories.includes(constellation);
      // Memory Nebula: 기억이 쌓인 사람 주변에 성운이 형성됩니다.
      for (const node of layout) {
        const p = toScreen(node.px, node.py),
          radius = Math.max(7, node.radius * view.zoom),
          nebula = nebulaRadius(node.memory_count, radius) * view.zoom;
        if (nebula <= 0) continue;
        const cloud = ctx.createRadialGradient(
          p.x,
          p.y,
          radius * 0.7,
          p.x,
          p.y,
          nebula,
        );
        cloud.addColorStop(0, "rgba(169,155,248,.22)");
        cloud.addColorStop(0.55, "rgba(120,183,241,.09)");
        cloud.addColorStop(1, "rgba(169,155,248,0)");
        ctx.globalAlpha = lit(node) ? 1 : 0.25;
        ctx.fillStyle = cloud;
        ctx.beginPath();
        ctx.arc(p.x, p.y, nebula, 0, Math.PI * 2);
        ctx.fill();
        ctx.globalAlpha = 1;
      }
      // 중심과 잇는 중력선
      for (const node of layout) {
        const p = toScreen(node.px, node.py);
        ctx.globalAlpha = lit(node) ? 1 : 0.22;
        ctx.strokeStyle =
          node.id === hovered ? "rgba(212,203,255,.58)" : "rgba(169,155,248,.15)";
        ctx.lineWidth = node.id === hovered ? 1.5 : 1;
        ctx.beginPath();
        ctx.moveTo(center.x, center.y);
        ctx.lineTo(p.x, p.y);
        ctx.stroke();
        ctx.globalAlpha = 1;
      }
      // 별자리: 선택된 그룹의 사람들을 최소 신장 트리로 이어 하나의 형상으로 보여줍니다.
      if (constellation) {
        const members = layout.filter((node) => lit(node));
        ctx.save();
        ctx.strokeStyle = "rgba(226,220,255,.5)";
        ctx.lineWidth = 1.1;
        ctx.shadowBlur = 8;
        ctx.shadowColor = "rgba(169,155,248,.7)";
        for (const [a, b] of constellationEdges(members)) {
          const pa = toScreen(a.px, a.py),
            pb = toScreen(b.px, b.py);
          ctx.beginPath();
          ctx.moveTo(pa.x, pa.y);
          ctx.lineTo(pb.x, pb.y);
          ctx.stroke();
        }
        ctx.restore();
        if (members.length) {
          const top = members.reduce((a, b) => (a.py < b.py ? a : b));
          const anchor = toScreen(
            members.reduce((sum, n) => sum + n.px, 0) / members.length,
            top.py,
          );
          ctx.fillStyle = "rgba(226,220,255,.72)";
          ctx.font = `700 ${Math.max(10, 11 * view.zoom)}px Pretendard, sans-serif`;
          ctx.textAlign = "center";
          ctx.fillText(
            `${constellation} 별자리 · ${members.length}`,
            anchor.x,
            anchor.y - top.radius * view.zoom - 22,
          );
        }
      }
      for (const node of layout) {
        const p = toScreen(node.px, node.py),
          radius = Math.max(7, node.radius * view.zoom),
          { state, tone, vector } = node.grammar,
          frozen = state === "dormant";
        const alpha =
          (state === "approaching"
            ? 1
            : state === "stable"
              ? 0.95
              : state === "drifting"
                ? 0.8
                : 0.45) * (lit(node) ? 1 : 0.25);
        ctx.save();
        ctx.globalAlpha = alpha;
        ctx.shadowBlur = node.id === hovered ? 28 : frozen ? 4 : 14;
        ctx.shadowColor = tone;
        const g = ctx.createRadialGradient(
          p.x - radius * 0.25,
          p.y - radius * 0.3,
          1,
          p.x,
          p.y,
          radius,
        );
        // 관계 온도: 행성 본체 색은 상태가 정합니다. 활발할수록 밝고, 휴면이면 식습니다.
        g.addColorStop(0, frozen ? "#c9d0e6" : "#fbfaff");
        g.addColorStop(0.24, tone);
        g.addColorStop(1, frozen ? "rgba(28,33,54,.95)" : "rgba(42,36,75,.95)");
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.arc(p.x, p.y, radius, 0, Math.PI * 2);
        ctx.fill();
        // 카테고리(소속)는 테두리 색으로만 남깁니다. 두 축이 서로를 가리지 않습니다.
        ctx.shadowBlur = 0;
        ctx.strokeStyle = node.color;
        ctx.globalAlpha = alpha * (frozen ? 0.5 : 0.95);
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.arc(p.x, p.y, radius + 2, 0, Math.PI * 2);
        ctx.stroke();
        ctx.globalAlpha = alpha;
        if (state === "approaching") {
          // 다가오는 관계는 넓어지는 중력장으로 표시합니다.
          ctx.strokeStyle = "rgba(123,226,169,.28)";
          ctx.lineWidth = 1;
          ctx.beginPath();
          ctx.arc(p.x, p.y, radius + 7, 0, Math.PI * 2);
          ctx.stroke();
        }
        if (node.anchored) {
          // Anchored Star: 궤도에 못 박힌 별. 네 방향 고정 침으로 표시합니다.
          ctx.strokeStyle = "rgba(246,217,129,.8)";
          ctx.lineWidth = 1.5;
          for (let i = 0; i < 4; i++) {
            const spoke = Math.PI / 4 + (i * Math.PI) / 2;
            ctx.beginPath();
            ctx.moveTo(
              p.x + Math.cos(spoke) * (radius + 4),
              p.y + Math.sin(spoke) * (radius + 4),
            );
            ctx.lineTo(
              p.x + Math.cos(spoke) * (radius + 9),
              p.y + Math.sin(spoke) * (radius + 9),
            );
            ctx.stroke();
          }
        }
        if (frozen) {
          ctx.strokeStyle = "rgba(124,134,168,.5)";
          ctx.lineWidth = 1;
          ctx.setLineDash([2, 3]);
          ctx.beginPath();
          ctx.arc(p.x, p.y, radius + 6, 0, Math.PI * 2);
          ctx.stroke();
          ctx.setLineDash([]);
        }
        // Momentum Vector: 숫자 대신 이동 방향과 세기를 화살표로 읽힙니다.
        if (state !== "stable" && Math.abs(vector) > 0.08) {
          const distance = Math.hypot(node.px, node.py) || 1,
            sign = vector > 0 ? 1 : -1,
            ux = (-node.px / distance) * sign,
            uy = (-node.py / distance) * sign,
            from = radius + 5,
            length = 9 + Math.min(1, Math.abs(vector)) * 22 * view.zoom,
            ax = p.x + ux * from,
            ay = p.y + uy * from,
            bx = p.x + ux * (from + length),
            by = p.y + uy * (from + length);
          ctx.strokeStyle = tone;
          ctx.lineWidth = 1.6;
          ctx.setLineDash(frozen ? [3, 4] : []);
          ctx.beginPath();
          ctx.moveTo(ax, ay);
          ctx.lineTo(bx, by);
          ctx.stroke();
          ctx.setLineDash([]);
          const head = 5.5,
            spread = 2.6;
          ctx.beginPath();
          ctx.moveTo(bx, by);
          ctx.lineTo(
            bx - ux * head + uy * spread,
            by - uy * head - ux * spread,
          );
          ctx.moveTo(bx, by);
          ctx.lineTo(
            bx - ux * head - uy * spread,
            by - uy * head + ux * spread,
          );
          ctx.stroke();
        }
        ctx.restore();
        if (view.zoom > 0.64 || node.importance > 0.72) {
          ctx.globalAlpha = lit(node) ? 1 : 0.3;
          ctx.fillStyle = frozen ? "#c3c8db" : "#f3f1fb";
          ctx.font = `${node.id === hovered ? "700" : "600"} ${Math.max(12, 14 * view.zoom)}px Pretendard, sans-serif`;
          ctx.textAlign = "center";
          ctx.fillText(node.name, p.x, p.y + radius + 18 * view.zoom);
          if (view.zoom > 1.25 && node.label) {
            ctx.fillStyle = "rgba(210,207,225,.72)";
            ctx.font = `${12 * view.zoom}px Pretendard, sans-serif`;
            ctx.fillText(node.label, p.x, p.y + radius + 35 * view.zoom);
          }
          if (node.id === hovered && node.memory_count) {
            ctx.fillStyle = "rgba(169,155,248,.9)";
            ctx.font = `600 ${Math.max(11, 12 * view.zoom)}px Pretendard, sans-serif`;
            ctx.fillText(
              `기억 ${node.memory_count}개`,
              p.x,
              p.y + radius + (view.zoom > 1.25 ? 52 : 35) * view.zoom,
            );
          }
          ctx.globalAlpha = 1;
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
      if (comet) drawComet(ctx, comet, toScreen, view.zoom, scale, elapsed);
    };
    let frame = 0;
    if (comet && !reduceMotion) {
      const started = performance.now();
      const loop = (now: number) => {
        draw(now - started);
        frame = requestAnimationFrame(loop);
      };
      frame = requestAnimationFrame(loop);
    } else draw(COMET_PERIOD * 0.82);
    return () => cancelAnimationFrame(frame);
  }, [
    layout,
    size,
    scale,
    toScreen,
    view.zoom,
    hovered,
    centerName,
    constellation,
    rediscover,
  ]);
  const stateCounts = useMemo(() => {
    const counts = {} as Record<RelationState, number>;
    for (const node of layout)
      counts[node.grammar.state] = (counts[node.grammar.state] ?? 0) + 1;
    return counts;
  }, [layout]);
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
        {STATE_ORDER.filter((state) => stateCounts[state]).map((state) => (
          <Chip
            key={state}
            size="small"
            variant="outlined"
            icon={
              <Box
                component="span"
                sx={{
                  width: 8,
                  height: 8,
                  borderRadius: "50%",
                  bgcolor: STATE_META[state].tone,
                  ml: "9px!important",
                }}
              />
            }
            label={`${STATE_META[state].label} ${stateCounts[state]}`}
          />
        ))}
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
