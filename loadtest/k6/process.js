// k6 load test for the alpha_proxy /process endpoint.
//
// Reads the dataset from DATA_DIR (manifest.json, profiles.json, corpus.jsonl,
// requests.jsonl) inside the SharedArray callback. Only the selected PROFILE is
// loaded. No payload, payload_id or personal data is ever printed.

import { SharedArray } from "k6/data";
import http from "k6/http";
import { check } from "k6";
import execution from "k6/execution";
import { Rate, Counter, Trend } from "k6/metrics";

const DATA_DIR = __ENV.DATA_DIR;
if (!DATA_DIR) {
  throw new Error("DATA_DIR is required: point it at the directory containing manifest.json, profiles.json, corpus.jsonl and requests.jsonl");
}

const PROFILE = __ENV.PROFILE || "mixed";
const BASE_URL = __ENV.BASE_URL || "http://127.0.0.1:8080";
const API_KEY = __ENV.API_KEY || "";
const RUN_ID = __ENV.RUN_ID || "run";
const RPS = Number(__ENV.RPS || 1000);
const DURATION = __ENV.DURATION || "30s";
const PRE_ALLOCATED_VUS = Number(__ENV.PRE_ALLOCATED_VUS || 500);
const MAX_VUS = Number(__ENV.MAX_VUS || 3000);
const REQUEST_TIMEOUT = __ENV.REQUEST_TIMEOUT || "30s";
const MAX_ERROR_RATE = Number(__ENV.MAX_ERROR_RATE || 0.01);
const P95_MS = Number(__ENV.P95_MS || 2000);
const P99_MS = Number(__ENV.P99_MS || 5000);
const RETRY_RATE = Number(__ENV.RETRY_RATE || 0.15);
const DETOKENIZE_RATE = Number(__ENV.DETOKENIZE_RATE || 0.05);

if (RETRY_RATE < 0 || RETRY_RATE > 1 || DETOKENIZE_RATE < 0 || DETOKENIZE_RATE > 1 || RETRY_RATE + DETOKENIZE_RATE > 1) {
  throw new Error("invalid rates: RETRY_RATE and DETOKENIZE_RATE must be in [0,1] and their sum must not exceed 1");
}

const transportErrors = new Rate("transport_errors");
const non200 = new Rate("non_200");
const invalidJSON = new Rate("invalid_json");
const processFailed = new Rate("process_failed");
const processAttempts = new Counter("process_attempts");
const processSuccess = new Counter("process_success");
const newRequests = new Counter("new_requests");
const retryRequests = new Counter("retry_requests");
const detokenizeRequests = new Counter("detokenize_requests");
const payloadChars = new Trend("payload_chars");

function fail(check, count) {
  // Only the check name and the record count are printed, never payload data.
  throw new Error(`dataset validation failed: ${check} (records: ${count})`);
}

function parseJSONL(text) {
  const lines = text.split("\n").filter((l) => l.trim() !== "");
  return lines.map((l) => JSON.parse(l));
}

const dataset = new SharedArray("dataset", function () {
  const manifest = JSON.parse(open(`${DATA_DIR}/manifest.json`));
  const profiles = JSON.parse(open(`${DATA_DIR}/profiles.json`));
  const corpus = parseJSONL(open(`${DATA_DIR}/corpus.jsonl`));
  const requests = parseJSONL(open(`${DATA_DIR}/requests.jsonl`));

  if (corpus.length !== manifest.rows) {
    fail("corpus row count mismatch", corpus.length);
  }
  if (requests.length !== manifest.rows) {
    fail("requests row count mismatch", requests.length);
  }

  for (let i = 0; i < requests.length; i++) {
    const r = requests[i];
    if (typeof r.payload !== "string" || typeof r.payload_id !== "string") {
      fail("requests entry missing string payload/payload_id", i);
    }
    const c = corpus[i];
    if (!c || typeof c.request !== "object" || c.request === null) {
      fail("corpus entry missing request", i);
    }
    if (c.request.payload !== r.payload || c.request.payload_id !== r.payload_id) {
      fail("corpus request does not match requests entry", i);
    }
  }

  if (!Array.isArray(profiles[PROFILE])) {
    fail("profile not found", PROFILE);
  }

  const profileIDs = new Set(profiles[PROFILE]);

  // Build the selected dataset: payload, index, bucket, content_profile, chars.
  // The original payload_id is intentionally not stored.
  const selected = [];
  for (let i = 0; i < requests.length; i++) {
    if (!profileIDs.has(requests[i].payload_id)) {
      continue;
    }
    const c = corpus[i];
    selected.push({
      payload: requests[i].payload,
      index: i,
      bucket: c.bucket,
      content_profile: c.content_profile,
      chars: c.chars,
    });
  }

  if (selected.length === 0) {
    fail("selected profile is empty", PROFILE);
  }

  return selected;
});

export const options = {
  scenarios: {
    load: {
      executor: "constant-arrival-rate",
      rate: RPS,
      timeUnit: "1s",
      duration: DURATION,
      preAllocatedVUs: PRE_ALLOCATED_VUS,
      maxVUs: MAX_VUS,
    },
  },
  summaryTrendStats: ["avg", "min", "p(50)", "p(90)", "p(95)", "p(99)", "max"],
  thresholds: {
    http_req_failed: [`rate<${MAX_ERROR_RATE}`],
    process_failed: [`rate<${MAX_ERROR_RATE}`],
    http_req_duration: [`p(95)<${P95_MS}`, `p(99)<${P99_MS}`],
    dropped_iterations: ["count==0"],
  },
};

// Per-VU mutable session list (not SharedArray). Keyed by VU id.
const sessionsByVU = {};

export default function () {
  processAttempts.add(1);

  const rec = dataset[execution.scenario.iterationInTest % dataset.length];

  let sessions = sessionsByVU[__VU];
  if (!sessions) {
    sessions = [];
    sessionsByVU[__VU] = sessions;
  }

  // Decide the operation: new / retry / detokenize.
  const r = Math.random();
  let operation = "new";
  if (sessions.length > 0 && r < RETRY_RATE) {
    operation = "retry";
  } else if (sessions.length > 0 && r < RETRY_RATE + DETOKENIZE_RATE) {
    operation = "detokenize";
  }

  let payload;
  let payloadID;
  if (operation === "retry") {
    const s = sessions[Math.floor(Math.random() * sessions.length)];
    payload = s.payload;
    payloadID = s.payloadID;
  } else if (operation === "detokenize") {
    const s = sessions[Math.floor(Math.random() * sessions.length)];
    payload = `Ответ ${s.result}`;
    payloadID = s.payloadID;
  } else {
    payload = rec.payload;
    payloadID = `${RUN_ID}-${__VU}-${execution.scenario.iterationInTest}-${rec.index}`;
  }

  const body = JSON.stringify({ payload: payload, payload_id: payloadID });

  const params = {
    headers: { "Content-Type": "application/json" },
    timeout: REQUEST_TIMEOUT,
    tags: {
      profile: PROFILE,
      bucket: rec.bucket,
      content_profile: rec.content_profile,
      operation: operation,
    },
  };
  if (API_KEY) {
    params.headers["X-API-Key"] = API_KEY;
  }

  payloadChars.add(rec.chars, params.tags);

  const res = http.post(`${BASE_URL}/process`, body, params);

  // Each Rate below receives exactly one value per iteration.
  const hasTransportError = !!res.error;
  transportErrors.add(hasTransportError ? 1 : 0, params.tags);

  const hasNon200 = !hasTransportError && res.status !== 200;
  non200.add(hasNon200 ? 1 : 0, params.tags);

  let parsed = null;
  let hasInvalidJSON = false;
  if (!hasTransportError) {
    try {
      parsed = res.json();
    } catch (e) {
      hasInvalidJSON = true;
    }
  }
  invalidJSON.add(hasInvalidJSON ? 1 : 0, params.tags);

  const contentTypeOK = (res.headers["Content-Type"] || "").includes("application/json");
  const resultIsString = parsed !== null && typeof parsed.result === "string";
  const fullyOK = !hasTransportError && res.status === 200 && !hasInvalidJSON && contentTypeOK && resultIsString;

  processFailed.add(fullyOK ? 0 : 1, params.tags);

  if (fullyOK) {
    processSuccess.add(1, params.tags);
    if (operation === "new") {
      newRequests.add(1, params.tags);
      sessions.push({ payloadID: payloadID, payload: payload, result: parsed.result });
      if (sessions.length > 100) {
        sessions.shift();
      }
    } else if (operation === "retry") {
      retryRequests.add(1, params.tags);
    } else {
      detokenizeRequests.add(1, params.tags);
    }
  }
}