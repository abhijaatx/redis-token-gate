#!/usr/bin/env node

// A Node 20+ load demonstration using only the built-in fetch API. It makes
// concurrent decisions for one identity and verifies the expected 200/429 mix.
const baseURL = process.env.RATE_LIMITER_URL ?? "http://localhost:8080";
const identity = process.env.RATE_LIMIT_IDENTITY ?? "demo-account-42";
const count = Number.parseInt(process.env.REQUEST_COUNT ?? "12", 10);
const token = process.env.API_TOKEN;

if (!Number.isInteger(count) || count < 1) {
  throw new Error("REQUEST_COUNT must be a positive integer");
}

const headers = { "content-type": "application/json" };
if (token) headers.authorization = `Bearer ${token}`;

const results = await Promise.all(
  Array.from({ length: count }, async (_, index) => {
    const response = await fetch(`${baseURL}/v1/check`, {
      method: "POST",
      headers,
      body: JSON.stringify({ identity }),
    });
    const body = await response.json();
    return {
      request: index + 1,
      status: response.status,
      allowed: body.allowed,
      remaining: response.headers.get("ratelimit-remaining"),
      retryAfter: response.headers.get("retry-after") ?? "-",
    };
  }),
);

console.table(results);
const allowed = results.filter((result) => result.status === 200).length;
const denied = results.filter((result) => result.status === 429).length;
console.log(`Allowed: ${allowed}; limited: ${denied}`);

if (results.some((result) => ![200, 429].includes(result.status))) {
  process.exitCode = 1;
}
