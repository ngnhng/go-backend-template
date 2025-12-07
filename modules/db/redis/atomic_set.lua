local key = KEYS[1]
local value = ARGV[1]
local ttl = tonumber(ARGV[2])

local prev = redis.call("GET", key)

if ttl and ttl > 0 then
  redis.call("SET", key, value, "EX", ttl)
else
  redis.call("SET", key, value)
end

return prev