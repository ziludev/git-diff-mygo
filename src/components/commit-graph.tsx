import {
  graphColorCount,
  graphLaneWidth,
  graphRowHeight,
  type GraphRow,
} from "@/lib/graph";

// Lane colors: saturated enough to separate on the sidebar, and distinct in
// both themes. Indexed by the layout's color number.
const firstColor = "#3b82f6";
const palette = [
  firstColor,
  "#22c55e",
  "#f59e0b",
  "#a855f7",
  "#ef4444",
  "#14b8a6",
  "#eab308",
  "#ec4899",
];

function laneColor(index: number): string {
  return palette[index % graphColorCount] ?? firstColor;
}

/**
 * The lane column of one history row: pass-through lines, elbows to the
 * commit's parents, and the commit's node. Width is fixed per list so the
 * subject of every row starts at the same x.
 */
export function CommitGraph({ row, lanes }: { row: GraphRow; lanes: number }) {
  const height = graphRowHeight;
  const width = Math.max(lanes, 1) * graphLaneWidth;
  const x = (lane: number) => lane * graphLaneWidth + graphLaneWidth / 2;

  return (
    <svg width={width} height={height} className="shrink-0" aria-hidden="true">
      {row.segments.map((segment, index) => (
        <line
          key={`segment-${index}`}
          x1={x(segment.lane)}
          x2={x(segment.lane)}
          y1={segment.from * height}
          y2={segment.to * height}
          stroke={laneColor(segment.color)}
          strokeWidth={2}
          strokeLinecap="round"
          opacity={0.55}
        />
      ))}
      {row.edges.map((edge, index) =>
        edge.from === edge.to ? (
          <line
            key={`edge-${index}`}
            x1={x(edge.from)}
            x2={x(edge.to)}
            y1={height / 2}
            y2={height}
            stroke={laneColor(edge.color)}
            strokeWidth={2}
            opacity={0.55}
          />
        ) : (
          <path
            key={`edge-${index}`}
            d={`M ${x(edge.from)} ${height / 2} Q ${x(edge.from)} ${height} ${x(edge.to)} ${height}`}
            fill="none"
            stroke={laneColor(edge.color)}
            strokeWidth={2}
            opacity={0.55}
          />
        ),
      )}
      <circle
        cx={x(row.lane)}
        cy={height / 2}
        r={4}
        fill={laneColor(row.color)}
        stroke="var(--background)"
        strokeWidth={1.5}
      />
    </svg>
  );
}
