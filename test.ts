import { cppCode, expected, pythonCode, stdin } from "./data";

const BASE_URL_FLASH_RS = "http://localhost:3002";
const BASE_URL_FLASH_GO = "http://localhost:3001";
const CONCURRENCY = 60;
const TOTAL_SUBMISSIONS = 100;
const RUNS = 10;
const WARMUP_RUNS = 1;

const LANGUAGE = 54;
const CODE = cppCode;
const INPUT = stdin;
const EXPECTED = expected;
const TIME_LIMIT = 1.0;
const MEMORY_LIMIT = 100000;
const STACK_LIMIT = 64000;

const CHECK_INTERVAL_MS = 100;
const MAX_WAIT_MS = 30_000;

type CreateResponse = {
  status: string;
  id: string;
};

type CreateRequest = {
  code: string;
  language: string;
  input: string;
  expected: string;
  time_limit?: number;
  memory_limit?: number;
  stack_limit?: number;
};

type CheckResponse = {
  created_at: number;
  started_at: number;
  finished_at: number;
  stdout: string;
  time: number;
  memory: number;
  stderr: string;
  token: number;
  compile_output: string;
  message: string;
  status: { id: number; description: string };
};

function getHeaders(baseUrl: string): Record<string, string> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    "Accept": "application/json",
  };

  return headers;
}

async function submitJob(
  index: number,
  baseUrl: string
): Promise<CreateResponse> {
  const payload = {
    source_code: btoa(CODE),
    language_id: LANGUAGE,
    stdin: btoa(INPUT),
    expected_output: btoa(EXPECTED),
    cpu_time_limit: TIME_LIMIT,
    memory_limit: MEMORY_LIMIT,
  };

  const response = await fetch(`${baseUrl}/submissions/batch?base64_encoded=true`, {
    method: "POST",
    headers: getHeaders(baseUrl),
    body: JSON.stringify({ submissions: [payload], free: false }),
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(`submit ${index} failed: ${response.status} ${text}`);
  }

  const json = (await response.json()) as Array<{ token: string }>;
  if (!Array.isArray(json) || json.length === 0 || !json[0]?.token) {
    throw new Error(`submit ${index} failed: invalid batch response`);
  }
  return { status: "created", id: json[0].token };
}

async function fetchResult(
  jobId: string,
  baseUrl: string
): Promise<CheckResponse> {
  const response = await fetch(
    `${baseUrl}/submissions/batch?tokens=${encodeURIComponent(jobId)}&base64_encoded=true`,
    {
      headers: getHeaders(baseUrl),
    }
  );
  if (!response.ok) {
    const text = await response.text();
    throw new Error(`check ${jobId} failed: ${response.status} ${text}`);
  }
  const json = (await response.json()) as { submissions?: CheckResponse[] };

  const result = json.submissions?.[0];
  if (!result) {
    throw new Error(`check ${jobId} failed: invalid batch response`);
  }
  return result;
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function average(values: number[]): number {
  if (values.length === 0) {
    return 0;
  }
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function percentile(values: number[], p: number): number {
  if (values.length === 0) {
    return 0;
  }
  const sorted = [...values].sort((a, b) => a - b);
  const index = Math.max(
    0,
    Math.min(sorted.length - 1, Math.floor((p / 100) * (sorted.length - 1)))
  );
  return sorted[index];
}

function formatTable(
  headers: string[],
  rows: string[][],
  columnWidths?: number[]
): string {
  const widths =
    columnWidths ||
    headers.map((_, i) =>
      Math.max(
        headers[i].length,
        ...rows.map((row) => (row[i] || "").length)
      )
    );

  const pad = (text: string, width: number, align: "left" | "right" = "left") => {
    const str = String(text);
    if (align === "right") {
      return str.padStart(width);
    }
    return str.padEnd(width);
  };

  const separator = "─".repeat(widths.reduce((sum, w) => sum + w + 3, 1));

  const headerRow =
    "│ " + headers.map((h, i) => pad(h, widths[i])).join(" │ ") + " │";
  const separatorRow = "├" + widths.map((w) => "─".repeat(w + 2)).join("┼") + "┤";
  const dataRows = rows.map(
    (row) => "│ " + row.map((cell, i) => pad(cell, widths[i], "right")).join(" │ ") + " │"
  );
  const bottomRow = "└" + widths.map((w) => "─".repeat(w + 2)).join("┴") + "┘";
  const topRow = "┌" + widths.map((w) => "─".repeat(w + 2)).join("┬") + "┐";

  return [
    topRow,
    headerRow,
    separatorRow,
    ...dataRows,
    bottomRow,
  ].join("\n");
}

async function waitForCompletion(
  jobId: string,
  baseUrl: string
): Promise<CheckResponse> {
  const deadline = Date.now() + MAX_WAIT_MS;
  for (;;) {
    const result = await fetchResult(jobId, baseUrl);
    if (result.status.id !== 1 && result.status.id !== 2) {
      return result;
    }
    if (Date.now() > deadline) {
      throw new Error(`job ${jobId} timed out waiting for completion`);
    }
    await delay(CHECK_INTERVAL_MS);
  }
}

async function runWithConcurrency<T>(
  tasks: Array<() => Promise<T>>,
  limit: number
): Promise<T[]> {
  if (limit <= 0) {
    throw new Error("CONCURRENCY must be >= 1");
  }

  const results: T[] = new Array(tasks.length);
  let cursor = 0;

  async function worker(): Promise<void> {
    for (;;) {
      const index = cursor;
      cursor += 1;
      if (index >= tasks.length) {
        return;
      }
      results[index] = await tasks[index]();
    }
  }

  const workers = new Array(Math.min(limit, tasks.length))
    .fill(null)
    .map(() => worker());

  await Promise.all(workers);
  return results;
}

async function submitAndTrack(
  index: number,
  baseUrl: string
): Promise<CheckResponse> {
  const created = await submitJob(index, baseUrl);
  const result = await waitForCompletion(created.id, baseUrl);
  return result;
}

type BenchmarkSummary = {
  label: string;
  baseUrl: string;
  avgMs: number;
  minMs: number;
  maxMs: number;
  totalMs: number;
  avgQueuedNs: number;
  avgQueuedP95Ns: number;
  avgProcessNs: number;
  avgProcessP95Ns: number;
};

async function runBenchmark(
  label: string,
  baseUrl: string,
  multiplier: number
): Promise<BenchmarkSummary> {
  const timingsMs: number[] = [];
  const avgQueuedNsPerRun: number[] = [];
  const p95QueuedNsPerRun: number[] = [];
  const avgProcessNsPerRun: number[] = [];
  const p95ProcessNsPerRun: number[] = [];

  console.log(`\n=== ${label} (${baseUrl}) ===`);

  for (let run = 1; run <= RUNS; run += 1) {
    const tasks = Array.from({ length: TOTAL_SUBMISSIONS * multiplier }, (_, index) => {
      return () => submitAndTrack(index + 1, baseUrl);
    });
    const startTime = Date.now();
    const results = await runWithConcurrency(tasks, CONCURRENCY);
    const durationMs = Date.now() - startTime;
    timingsMs.push(durationMs);

    if (results.length === 0) {
      continue;
    }

    const queuedNs = results.map((r) => Math.max(0, r.started_at - r.created_at));
    const processNs = results.map((r) => Math.max(0, r.finished_at - r.started_at));

    const queuedAvgNs = average(queuedNs);
    const queuedP95Ns = percentile(queuedNs, 95);
    const processAvgNs = average(processNs);
    const processP95Ns = percentile(processNs, 95);

    avgQueuedNsPerRun.push(queuedAvgNs);
    p95QueuedNsPerRun.push(queuedP95Ns);
    avgProcessNsPerRun.push(processAvgNs);
    p95ProcessNsPerRun.push(processP95Ns);
    console.log(
      `Run ${run}/${RUNS}: ${TOTAL_SUBMISSIONS} submissions @ concurrency ${CONCURRENCY} in ${durationMs}ms (queued avg=${(queuedAvgNs / 1_000_000_000).toFixed(2)}s p95=${(queuedP95Ns / 1_000_000_000).toFixed(2)}s, in-process avg=${(processAvgNs / 1_000_000_000).toFixed(2)}s p95=${(processP95Ns / 1_000_000_000).toFixed(2)}s)`
    );
  }

  const effectiveWarmup = Math.min(WARMUP_RUNS, Math.max(0, timingsMs.length - 1));
  const measured = timingsMs.slice(effectiveWarmup);
  const total = measured.reduce((sum, value) => sum + value, 0);
  const avg = measured.length > 0 ? total / measured.length : 0;
  const min = measured.length > 0 ? Math.min(...measured) : 0;
  const max = measured.length > 0 ? Math.max(...measured) : 0;
  const queuedAvgMeasured = avgQueuedNsPerRun.slice(effectiveWarmup);
  const queuedP95Measured = p95QueuedNsPerRun.slice(effectiveWarmup);
  const processAvgMeasured = avgProcessNsPerRun.slice(effectiveWarmup);
  const processP95Measured = p95ProcessNsPerRun.slice(effectiveWarmup);
  const avgQueued = average(queuedAvgMeasured);
  const avgQueuedP95 = average(queuedP95Measured);
  const avgProcess = average(processAvgMeasured);
  const avgProcessP95 = average(processP95Measured);

  console.log(`\nSummary (warmup ${effectiveWarmup}, runs ${measured.length}):`);
  console.log(
    formatTable(
      ["Metric", "Value"],
      [
        ["Total Time", `${total.toFixed(1)} ms`],
        ["Average Time", `${avg.toFixed(1)} ms`],
        ["Min Time", `${min} ms`],
        ["Max Time", `${max} ms`],
        ["Avg Queued", `${(avgQueued / 1_000_000_000).toFixed(3)} s`],
        ["P95 Queued", `${(avgQueuedP95 / 1_000_000_000).toFixed(3)} s`],
        ["Avg In-Process", `${(avgProcess / 1_000_000_000).toFixed(3)} s`],
        ["P95 In-Process", `${(avgProcessP95 / 1_000_000_000).toFixed(3)} s`],
      ],
      [20, 15]
    )
  );

  return {
    label,
    baseUrl,
    avgMs: avg,
    minMs: min,
    maxMs: max,
    totalMs: total,
    avgQueuedNs: avgQueued,
    avgQueuedP95Ns: avgQueuedP95,
    avgProcessNs: avgProcess,
    avgProcessP95Ns: avgProcessP95,
  };
}

async function main(): Promise<void> {
  const benchmarks = [
    { label: "flash-rs", baseUrl: BASE_URL_FLASH_RS, multiplier: 1 },
    { label: "flash-go", baseUrl: BASE_URL_FLASH_GO, multiplier: 1 },

  ];

  const summaries: BenchmarkSummary[] = [];

  for (const benchmark of benchmarks) {
    new Promise((resolve) => setTimeout(resolve, 5000));
    summaries.push(await runBenchmark(benchmark.label, benchmark.baseUrl, benchmark.multiplier));
  }

  if (summaries.length > 1) {
    console.log("\n=== Comparison ===");
    const headers = ["Metric", ...summaries.map((s) => s.label)];
    const rows = [
      [
        "Total Time",
        ...summaries.map((s) => `${s.totalMs.toFixed(1)} ms`),
      ],
      [
        "Average Time",
        ...summaries.map((s) => `${s.avgMs.toFixed(1)} ms`),
      ],
      [
        "Min Time",
        ...summaries.map((s) => `${s.minMs} ms`),
      ],
      [
        "Max Time",
        ...summaries.map((s) => `${s.maxMs} ms`),
      ],
      [
        "Avg Queued",
        ...summaries.map((s) => `${(s.avgQueuedNs / 1_000_000_000).toFixed(3)} s`),
      ],
      [
        "P95 Queued",
        ...summaries.map((s) => `${(s.avgQueuedP95Ns / 1_000_000_000).toFixed(3)} s`),
      ],
      [
        "Avg In-Process",
        ...summaries.map((s) => `${(s.avgProcessNs / 1_000_000_000).toFixed(3)} s`),
      ],
      [
        "P95 In-Process",
        ...summaries.map((s) => `${(s.avgProcessP95Ns / 1_000_000_000).toFixed(3)} s`),
      ],
    ];
    console.log(formatTable(headers, rows));
  }
}

main().catch((err) => {
  console.error(err);
});

