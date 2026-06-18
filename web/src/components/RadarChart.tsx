'use client';

interface Dimension {
  label: string;
  value: number;
  max: number;
}

interface Props {
  dimensions: Dimension[];
  size?: number;
}

export function RadarChart({ dimensions, size = 280 }: Props) {
  const cx = size / 2;
  const cy = size / 2;
  const radius = size * 0.38;
  const levels = 5;
  const angleSlice = (Math.PI * 2) / dimensions.length;

  const getPoint = (angle: number, r: number) => ({
    x: cx + r * Math.cos(angle - Math.PI / 2),
    y: cy + r * Math.sin(angle - Math.PI / 2),
  });

  const gridLines = Array.from({ length: levels }, (_, i) => {
    const r = radius * ((i + 1) / levels);
    const points = dimensions.map((_, j) => {
      const p = getPoint(angleSlice * j, r);
      return `${p.x},${p.y}`;
    });
    return points.join(' ');
  });

  const dataPoints = dimensions.map((d, i) => {
    const r = (d.value / d.max) * radius;
    return getPoint(angleSlice * i, r);
  });

  const dataPath = dataPoints.map((p, i) => (i === 0 ? `M ${p.x},${p.y}` : `L ${p.x},${p.y}`)).join(' ') + ' Z';

  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="drop-shadow-sm">
      {/* Grid */}
      {gridLines.map((points, i) => (
        <polygon
          key={i}
          points={points}
          fill="none"
          stroke="#e2e8f0"
          strokeWidth={i === levels - 1 ? 1.5 : 0.8}
          opacity={0.8}
        />
      ))}

      {/* Axes */}
      {dimensions.map((_, i) => {
        const p = getPoint(angleSlice * i, radius);
        return (
          <line
            key={i}
            x1={cx}
            y1={cy}
            x2={p.x}
            y2={p.y}
            stroke="#e2e8f0"
            strokeWidth={0.8}
          />
        );
      })}

      {/* Data area */}
      <path
        d={dataPath}
        fill="rgba(14, 165, 233, 0.12)"
        stroke="#0ea5e9"
        strokeWidth={2}
      />

      {/* Data points */}
      {dataPoints.map((p, i) => (
        <circle
          key={i}
          cx={p.x}
          cy={p.y}
          r={4}
          fill="white"
          stroke="#0ea5e9"
          strokeWidth={2}
        />
      ))}

      {/* Labels */}
      {dimensions.map((d, i) => {
        const labelRadius = radius + 28;
        const p = getPoint(angleSlice * i, labelRadius);
        return (
          <text
            key={i}
            x={p.x}
            y={p.y}
            textAnchor="middle"
            dominantBaseline="middle"
            className="text-[11px] font-medium"
            fill="#475569"
          >
            {d.label}
          </text>
        );
      })}

      {/* Score labels */}
      {dimensions.map((d, i) => {
        const scoreRadius = radius + 12;
        const p = getPoint(angleSlice * i, scoreRadius);
        return (
          <text
            key={`score-${i}`}
            x={p.x}
            y={p.y + 12}
            textAnchor="middle"
            dominantBaseline="middle"
            className="text-[10px]"
            fill="#94a3b8"
          >
            {d.value}/{d.max}
          </text>
        );
      })}
    </svg>
  );
}
