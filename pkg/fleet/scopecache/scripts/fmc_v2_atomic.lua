-- FMC atomic cache protocol. All reads and mutations use EVALSHA; SCRIPT LOAD
-- is bootstrap/recovery only. Every successful repair carries one full writer
-- fence, registration revision, generation, and opaque token.
local op = ARGV[1]
local status = KEYS[1]

local function now_ms()
  local t = redis.call('TIME')
  return (tonumber(t[1]) * 1000) + math.floor(tonumber(t[2]) / 1000)
end

local function key_type(key)
  local reply = redis.call('TYPE', key)
  return reply.ok or reply
end

local function require_type(key, expected)
  local actual = key_type(key)
  if actual ~= 'none' and actual ~= expected then
    return redis.error_reply('WRONG_TYPE_' .. expected)
  end
  return nil
end

local function full_matches(registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation, token)
  return key_type(status) == 'hash'
    and redis.call('HGET', status, 'registration_uid') == tostring(registration_uid)
    and redis.call('HGET', status, 'expected_revision') == tostring(revision)
    and redis.call('HGET', status, 'holder_pod_uid') == tostring(holder_pod_uid)
    and redis.call('HGET', status, 'holder_session_uid') == tostring(holder_session_uid)
    and redis.call('HGET', status, 'lease_epoch') == tostring(lease_epoch)
    and redis.call('HGET', status, 'fence') == tostring(fence)
    and redis.call('HGET', status, 'generation') == tostring(generation)
    and redis.call('HGET', status, 'repair_token') == tostring(token)
end

local function read_matches(registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation)
  return key_type(status) == 'hash'
    and redis.call('HGET', status, 'registration_uid') == tostring(registration_uid)
    and redis.call('HGET', status, 'expected_revision') == tostring(revision)
    and redis.call('HGET', status, 'holder_pod_uid') == tostring(holder_pod_uid)
    and redis.call('HGET', status, 'lease_epoch') == tostring(lease_epoch)
    and redis.call('HGET', status, 'holder_session_uid') == tostring(holder_session_uid)
    and redis.call('HGET', status, 'fence') == tostring(fence)
    and redis.call('HGET', status, 'generation') == tostring(generation)
end

local function valid_ttl(now, expires, pttl)
  return expires ~= nil and pttl ~= nil and expires > now and pttl > 0 and math.abs((expires - now) - pttl) <= 2
end

local function update_expiry(key, field, expires)
  local current = tonumber(redis.call('HGET', key, field)) or 0
  if expires > current then
    redis.call('HSET', key, field, expires)
    redis.call('PEXPIREAT', key, expires)
    return expires
  end
  local pttl = redis.call('PTTL', key)
  if pttl <= 0 then
    redis.call('PEXPIREAT', key, current)
  end
  return current
end

if op == 'begin' then
  local registration_uid = ARGV[2]
  local revision = ARGV[3]
  local holder_pod_uid = ARGV[4]
  local holder_session_uid = ARGV[5]
  local lease_epoch = ARGV[6]
  local fence = ARGV[7]
  local token = ARGV[8]
  local type_error = require_type(status, 'hash')
  if type_error ~= nil then
    return type_error
  end
  local generation = redis.call('HINCRBY', status, 'generation', 1)
  if generation <= 0 then
    return redis.error_reply('INVALID_GENERATION')
  end
  local now = now_ms()
  local status_expires = now + 60000
  redis.call('HSET', status,
    'state', 'repairing',
    'registration_uid', registration_uid,
    'expected_revision', revision,
    'holder_pod_uid', holder_pod_uid,
    'holder_session_uid', holder_session_uid,
    'lease_epoch', lease_epoch,
    'fence', fence,
    'generation', generation,
    'repair_token', token,
    'clean_commit', '0',
    'observed_count', '0',
    'cleanup_count', '0',
    'repair_reason', '',
    'status_expires_at_ms', status_expires,
    'index_expires_at_ms', status_expires)
  redis.call('PEXPIREAT', status, status_expires)
  return {tostring(generation)}
end

if op == 'repair_reconcile' then
  local registration_uid = ARGV[2]
  local revision = ARGV[3]
  local holder_pod_uid = ARGV[4]
  local holder_session_uid = ARGV[5]
  local lease_epoch = ARGV[6]
  local fence = ARGV[7]
  local generation = ARGV[8]
  local token = ARGV[9]
  if not full_matches(registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation, token) then
    return redis.error_reply('STALE_FENCE')
  end
  local record = KEYS[2]
  local index = KEYS[3]
  local record_type_error = require_type(record, 'hash')
  if record_type_error ~= nil then
    return record_type_error
  end
  local index_type_error = require_type(index, 'zset')
  if index_type_error ~= nil then
    return index_type_error
  end
  local public_id = ARGV[10]
  local ttl = tonumber(ARGV[17])
  if ttl == nil or ttl <= 0 then
    return redis.error_reply('INVALID_TTL')
  end
  local now = now_ms()
  local expires = now + ttl
  redis.call('HSET', record,
    'public_id', public_id,
    'registration_uid', registration_uid,
    'expected_revision', revision,
    'generation', generation,
    'holder_pod_uid', holder_pod_uid,
    'holder_session_uid', holder_session_uid,
    'lease_epoch', lease_epoch,
    'fence', fence,
    'group', ARGV[11],
    'version', ARGV[12],
    'kind', ARGV[13],
    'namespace', ARGV[14],
    'name', ARGV[15],
    'label_state', ARGV[16],
    'expires_at_ms', expires)
  redis.call('PEXPIREAT', record, expires)
  redis.call('ZADD', index, expires, public_id)
  local status_expires = update_expiry(status, 'status_expires_at_ms', expires)
  local index_expires = tonumber(redis.call('HGET', status, 'index_expires_at_ms')) or 0
  if expires > index_expires then
    index_expires = expires
    redis.call('HSET', status, 'index_expires_at_ms', index_expires)
  end
  redis.call('PEXPIREAT', index, index_expires)
  local record_pttl = redis.call('PTTL', record)
  local index_pttl = redis.call('PTTL', index)
  local status_pttl = redis.call('PTTL', status)
  if not valid_ttl(now, expires, record_pttl) or not valid_ttl(now, index_expires, index_pttl) or not valid_ttl(now, status_expires, status_pttl) then
    return redis.error_reply('INVALID_PTTL')
  end
  return {'ok', tostring(now), tostring(expires), tostring(record_pttl), tostring(index_pttl), tostring(status_pttl), tostring(index_expires), tostring(status_expires)}
end

if op == 'repair_list_members' then
  local registration_uid = ARGV[2]
  local revision = ARGV[3]
  local holder_pod_uid = ARGV[4]
  local holder_session_uid = ARGV[5]
  local lease_epoch = ARGV[6]
  local fence = ARGV[7]
  local generation = ARGV[8]
  local token = ARGV[9]
  local cursor = tonumber(ARGV[10]) or 0
  if not full_matches(registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation, token) then
    return redis.error_reply('STALE_FENCE')
  end
  if key_type(KEYS[2]) == 'none' then
    return {'-1'}
  end
  if key_type(KEYS[2]) ~= 'zset' then
    return redis.error_reply('WRONG_TYPE_ZSET')
  end
  local members = redis.call('ZRANGE', KEYS[2], 0, -1)
  if #members > 10000 then
    return redis.error_reply('INDEX_BOUNDS')
  end
  table.sort(members)
  if cursor < 0 then
    cursor = 0
  end
  if cursor >= #members then
    return {'-1'}
  end
  local next_cursor = cursor + 256
  if next_cursor >= #members then
    next_cursor = -1
  end
  local reply = {tostring(next_cursor)}
  local last = cursor + 255
  if last >= #members then
    last = #members - 1
  end
  for i = cursor, last do
    table.insert(reply, members[i + 1])
  end
  return reply
end

if op == 'cleanup_member' then
  local registration_uid = ARGV[2]
  local revision = ARGV[3]
  local holder_pod_uid = ARGV[4]
  local holder_session_uid = ARGV[5]
  local lease_epoch = ARGV[6]
  local fence = ARGV[7]
  local generation = ARGV[8]
  local token = ARGV[9]
  local member = ARGV[10]
  if not full_matches(registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation, token) then
    return redis.error_reply('STALE_FENCE')
  end
  local removed = redis.call('ZREM', KEYS[2], member)
  if key_type(KEYS[3]) ~= 'none' then
    if key_type(KEYS[3]) ~= 'hash' then
      return redis.error_reply('WRONG_TYPE_HASH')
    end
    redis.call('DEL', KEYS[3])
  end
  return {'ok', tostring(removed)}
end

if op == 'read_verified' then
  local registration_uid = ARGV[2]
  local revision = ARGV[3]
  local holder_pod_uid = ARGV[4]
  local holder_session_uid = ARGV[5]
  local lease_epoch = ARGV[6]
  local fence = ARGV[7]
  local generation = ARGV[8]
  local public_id = ARGV[9]
  if key_type(status) == 'none' then
    return {'missing'}
  end
  if key_type(status) ~= 'hash' then
    return redis.error_reply('WRONG_TYPE_HASH')
  end
  if redis.call('HGET', status, 'state') ~= 'clean' or redis.call('HGET', status, 'clean_commit') ~= '1' then
    return {'missing'}
  end
  if not read_matches(registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation) then
    return {'missing'}
  end
  local record = KEYS[3]
  local index = KEYS[2]
  if key_type(record) == 'none' or key_type(index) == 'none' then
    return {'missing'}
  end
  if key_type(record) ~= 'hash' or key_type(index) ~= 'zset' then
    return redis.error_reply('WRONG_TYPE')
  end
  if redis.call('HGET', record, 'registration_uid') ~= registration_uid or
     redis.call('HGET', record, 'expected_revision') ~= revision or
     redis.call('HGET', record, 'generation') ~= generation or
     redis.call('HGET', record, 'holder_pod_uid') ~= holder_pod_uid or
     redis.call('HGET', record, 'holder_session_uid') ~= holder_session_uid or
     redis.call('HGET', record, 'lease_epoch') ~= lease_epoch or
     redis.call('HGET', record, 'fence') ~= fence or
     redis.call('HGET', record, 'public_id') ~= public_id then
    return {'missing'}
  end
  local label_state = redis.call('HGET', record, 'label_state')
  if label_state ~= 'true' and label_state ~= 'false' and label_state ~= 'absent' then
    return redis.error_reply('INVALID_LABEL_STATE')
  end
  local now = now_ms()
  local expires = tonumber(redis.call('HGET', record, 'expires_at_ms'))
  local record_pttl = redis.call('PTTL', record)
  local status_expires = tonumber(redis.call('HGET', status, 'status_expires_at_ms'))
  local status_pttl = redis.call('PTTL', status)
  local index_expires = tonumber(redis.call('HGET', status, 'index_expires_at_ms'))
  local index_pttl = redis.call('PTTL', index)
  local score = redis.call('ZSCORE', index, public_id)
  if score == false or tostring(score) ~= tostring(expires) then
    return {'missing'}
  end
  if not valid_ttl(now, expires, record_pttl) or not valid_ttl(now, status_expires, status_pttl) or not valid_ttl(now, index_expires, index_pttl) then
    return redis.error_reply('TTL_MISMATCH')
  end
  return {'ok', label_state, tostring(now), tostring(expires), tostring(record_pttl), tostring(status_expires), tostring(status_pttl), tostring(index_expires), tostring(index_pttl), tostring(generation), tostring(fence), tostring(revision)}
end

if op == 'commit' then
  local registration_uid = ARGV[2]
  local revision = ARGV[3]
  local holder_pod_uid = ARGV[4]
  local holder_session_uid = ARGV[5]
  local lease_epoch = ARGV[6]
  local fence = ARGV[7]
  local generation = ARGV[8]
  local token = ARGV[9]
  local expected_count = tonumber(ARGV[10])
  local observed_count = tonumber(ARGV[11])
  local cleanup_count = tonumber(ARGV[12])
  if not full_matches(registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation, token) then
    return redis.error_reply('STALE_FENCE')
  end
  if redis.call('HGET', status, 'state') ~= 'repairing' or expected_count == nil or observed_count == nil or cleanup_count == nil or expected_count ~= observed_count or cleanup_count < 0 then
    return redis.error_reply('COMMIT_REJECTED')
  end
  for i = 13, 19 do
    if ARGV[i] ~= '1' then
      return redis.error_reply('COMMIT_REJECTED')
    end
  end
  local now = now_ms()
  local status_expires = tonumber(redis.call('HGET', status, 'status_expires_at_ms'))
  local status_pttl = redis.call('PTTL', status)
  if not valid_ttl(now, status_expires, status_pttl) then
    return redis.error_reply('TTL_MISMATCH')
  end
  redis.call('HSET', status,
    'state', 'clean',
    'clean_commit', '1',
    'observed_count', observed_count,
    'cleanup_count', cleanup_count,
    'repair_reason', '')
  return {'ok', tostring(revision), tostring(generation), tostring(fence), token}
end

if op == 'mark_uncertain' then
  local registration_uid = ARGV[2]
  local revision = ARGV[3]
  local holder_pod_uid = ARGV[4]
  local holder_session_uid = ARGV[5]
  local lease_epoch = ARGV[6]
  local fence = ARGV[7]
  local generation = ARGV[8]
  local token = ARGV[9]
  if not full_matches(registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation, token) then
    return redis.error_reply('STALE_FENCE')
  end
  redis.call('HSET', status, 'state', 'uncertain', 'clean_commit', '0', 'repair_reason', ARGV[11], 'failure_code', ARGV[10])
  return {'ok'}
end

return redis.error_reply('UNKNOWN_OPERATION')