const BASE_URL = "http://localhost:3001";
const CONCURRENCY = 10;
const TOTAL_SUBMISSIONS = 10;

const LANGUAGE = "python";
const CODE = "print('Hello from batch')";
const INPUT = "";
const EXPECTED = "Hello from batch";
const TIME_LIMIT = 2.0;
const MEMORY_LIMIT = 128000;
const STACK_LIMIT = 64000;

const CHECK_INTERVAL_MS = 500;
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

async function submitJob(index: number): Promise<CreateResponse> {
  const payload: CreateRequest = {
    code: CODE,
    language: LANGUAGE,
    input: INPUT,
    expected: EXPECTED,
    time_limit: TIME_LIMIT,
    memory_limit: MEMORY_LIMIT,
    stack_limit: STACK_LIMIT,
  };

  const response = await fetch(`${BASE_URL}/create`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

  if (!response.ok) {
    const text = await response.text();
    throw new Error(`submit ${index} failed: ${response.status} ${text}`);
  }

  return (await response.json()) as CreateResponse;
}

async function fetchResult(jobId: string): Promise<CheckResponse> {
  const response = await fetch(`${BASE_URL}/check/${jobId}`);
  if (!response.ok) {
    const text = await response.text();
    throw new Error(`check ${jobId} failed: ${response.status} ${text}`);
  }
  return (await response.json()) as CheckResponse;
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitForCompletion(jobId: string): Promise<CheckResponse> {
  const deadline = Date.now() + MAX_WAIT_MS;
  for (;;) {
    const result = await fetchResult(jobId);
    console.log(result);
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

async function submitAndTrack(index: number): Promise<void> {
  const clientStartMs = Date.now();
  const created = await submitJob(index);
  const result = await waitForCompletion(created.id);
  const elapsedMs = Date.now() - clientStartMs;
  const serverQueueDelayMs = (result.started_at - result.created_at) / 1e6;
  const clientToStartMs = result.started_at / 1e6 - clientStartMs;

  console.log(
    `time taken (server create -> worker start): ${serverQueueDelayMs.toFixed(
      3
    )}ms`
  );
  console.log(
    `time taken (client start -> worker start, approx): ${clientToStartMs.toFixed(
      3
    )}ms`
  );
  console.log(
    `job ${created.id} done in ${elapsedMs}ms (${result.status.description})`
  );
}

async function main(): Promise<void> {
  const tasks = Array.from({ length: TOTAL_SUBMISSIONS }, (_, index) => {
    return () => submitAndTrack(index + 1);
  });

  await runWithConcurrency(tasks, CONCURRENCY);
}

main().catch((err) => {
  console.error(err);
});

