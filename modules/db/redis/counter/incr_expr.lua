-- Atomic increment with TTL for new counters.
-- KEYS[1] = full key
-- ARGV[1] = ttl in milliseconds

local key = KEYS[1]
local ttl_ms = tonumber(ARGV[1])
local new_count = redis.call("INCR", key)

if new_count == 1 and ttl_ms and ttl_ms > 0 then
    redis.call("PEXPIRE", key, ttl_ms)
elseif ttl_ms < 0 then
    redis.call("PEXPIRE", key, 1)
end

return new_count
