import type { Commit } from "@/mygo";

/** Height of one commit row in pixels; the graph is drawn to fill it. */
export const graphRowHeight = 48;
/** Horizontal distance between two lanes, in pixels. */
export const graphLaneWidth = 14;
/** Number of colors the renderer cycles through. */
export const graphColorCount = 8;

/** A vertical line in one lane, from `from` to `to` in row heights (0..1). */
export type GraphSegment = { lane: number; color: number; from: number; to: number };

/** A line from the commit's node to one of its parents' lanes. */
export type GraphEdge = { from: number; to: number; color: number };

export type GraphRow = {
  /** The lane holding this commit's node. */
  lane: number;
  color: number;
  segments: GraphSegment[];
  edges: GraphEdge[];
  /** Lanes this row spans, before and after the commit. */
  lanes: number;
};

export type GraphLayout = {
  rows: GraphRow[];
  /** Widest row, so every row can share one column width. */
  lanes: number;
};

/**
 * Lays out commit lanes the way IDEA's log does: a lane is a promise to draw a
 * particular object next, so the lane that expects this commit receives it,
 * its first parent continues there, and every extra parent opens or joins
 * another lane. Rows are returned in commit order, newest first.
 *
 * The layout only depends on the commits given, so appending a page can simply
 * recompute the whole list.
 */
export function layoutGraph(commits: Commit[]): GraphLayout {
  const lanes: (string | null)[] = [];
  const colors: number[] = [];
  const rows: GraphRow[] = [];
  let nextColor = 0;
  let width = 0;

  const freeLane = (state: (string | null)[], skip: number) => {
    for (let i = 0; i < state.length; i++) {
      if (i !== skip && state[i] === null) return i;
    }
    state.push(null);
    return state.length - 1;
  };

  for (const commit of commits) {
    const before = lanes.slice();

    // The lane waiting for this commit, or a free one when the commit is a
    // branch tip that no lane has reached yet.
    let lane = lanes.indexOf(commit.oid);
    if (lane === -1) {
      lane = freeLane(lanes, -1);
      colors[lane] = nextColor++ % graphColorCount;
    }
    lanes[lane] = commit.oid;

    const after = lanes.slice();
    after[lane] = null;
    const parents = commit.parents ?? [];
    const [first, ...rest] = parents;
    const targets: number[] = [];

    if (first) {
      // An older lane may already wait for this parent: join it rather than
      // opening a duplicate lane for the same commit.
      const existing = before.indexOf(first);
      if (existing !== -1 && existing !== lane) {
        targets.push(existing);
      } else {
        after[lane] = first;
        targets.push(lane);
      }
    }
    for (const parent of rest) {
      const existing = after.indexOf(parent);
      if (existing !== -1) {
        targets.push(existing);
        continue;
      }
      const extra = freeLane(after, lane);
      after[extra] = parent;
      colors[extra] = nextColor++ % graphColorCount;
      targets.push(extra);
    }

    const colorOf = (i: number) => colors[i] ?? 0;
    const count = Math.max(before.length, after.length);
    const segments: GraphSegment[] = [];
    for (let i = 0; i < count; i++) {
      const existedBefore = i < before.length && before[i] !== null;
      const existsAfter = i < after.length && after[i] !== null;
      if (i === lane) {
        if (existedBefore) {
          segments.push({ lane: i, color: colorOf(i), from: 0, to: 0.5 });
        }
        continue;
      }
      if (existedBefore && existsAfter) {
        segments.push({ lane: i, color: colorOf(i), from: 0, to: 1 });
      } else if (existedBefore) {
        // Two lanes were waiting for the same commit; this one ends here.
        segments.push({ lane: i, color: colorOf(i), from: 0, to: 0.5 });
      }
    }
    const edges: GraphEdge[] = targets.map((to) => ({
      from: lane,
      to,
      color: colorOf(to),
    }));

    rows.push({ lane, color: colorOf(lane), segments, edges, lanes: count });
    width = Math.max(width, count);

    lanes.length = 0;
    lanes.push(...after);
    // Trailing free lanes are not drawn; dropping them keeps the column from
    // growing with every finished branch.
    while (lanes.length > 0 && lanes[lanes.length - 1] === null) lanes.pop();
  }

  return { rows, lanes: width };
}
