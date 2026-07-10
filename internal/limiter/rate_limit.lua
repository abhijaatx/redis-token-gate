-- Atomic token-bucket update. Values are strings because Redis/Lua converts
-- returned floating-point numbers to integers unless they are formatted first.
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_per_second = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])
local now_ms = tonumber(ARGV[4])
local ttl_ms = tonumber(ARGV[5])

local state = redis.call("HMGET", key, "tokens", "updated_at")
local tokens = tonumber(state[1]) or capacity
local updated_at = tonumber(state[2]) or now_ms
local elapsed_ms = math.max(0, now_ms - updated_at)

tokens = math.min(capacity, tokens + (elapsed_ms / 1000) * refill_per_second)

local allowed = 0
local retry_after_ms = 0
if tokens >= cost then
  tokens = tokens - cost
  allowed = 1
else
  retry_after_ms = math.ceil(((cost - tokens) / refill_per_second) * 1000)
end

local reset_after_ms = math.ceil(((capacity - tokens) / refill_per_second) * 1000)

redis.call("HSET", key, "tokens", string.format("%.6f", tokens), "updated_at", now_ms)
redis.call("PEXPIRE", key, ttl_ms)

return { allowed, string.format("%.6f", tokens), retry_after_ms, reset_after_ms }
