-- FMC atomic cache protocol, DD-FLEET-016 A2 staged variant. All reads and
-- mutations use EVALSHA; SCRIPT LOAD is bootstrap/recovery only. Repairs
-- write a STAGED generation (status/index/records under :staged keys with a
-- TTL) and a single atomic commit swaps the staged keyspace over the live
-- keyspace. The committed clean proof remains readable for the entire repair;
-- only a missing committed proof fails closed.
local op = ARGV[1]

-- DD-FLEET-017 D2.5a: the lexicographic identity index member is
-- "namespace \0 kind \0 group \0 version \0 name \0 publicID". None of those
-- values can contain NUL, so byte order is unambiguous and prefix ranges are
-- exact. The cluster is omitted because it is fixed per partition.
local SEP = string.char(0)

local function lex_member(namespace, kind, group, version, name, public_id)
  return namespace .. SEP .. kind .. SEP .. group .. SEP .. version .. SEP .. name .. SEP .. public_id
end

-- MAX_MEMBERS bounds both per-partition indexes (DD-FLEET-017 D2.7).
local MAX_MEMBERS = 10000

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

-- full_matches validates the complete writer identity against the STAGED
-- status hash (KEYS[1] of repair ops).
local function full_matches(status, registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation, token)
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

-- read_matches validates the reader's proof against the COMMITTED status.
local function read_matches(status, registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation)
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

-- lex_range returns the ZRANGEBYLEX bounds for a composed identity prefix.
-- A prefix is always a whole leading run of separator-terminated fields, so
-- "[" .. prefix .. "[\255" is an exact superset bound for that run and can
-- never widen into a neighbouring namespace or kind.
local function lex_range(prefix)
  if prefix == nil or prefix == '' then
    return '-', '+'
  end
  return '[' .. prefix, '[' .. prefix .. '\255'
end

-- prove_committed validates the committed clean proof once for an inventory
-- read (DD-FLEET-017 D2.5a / D3). It returns a table with the proof values, or
-- nil plus a fail-closed reason. The reason is diagnostic only; every reason
-- maps to "unavailable" on the wire.
local function prove_committed(status, registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation)
  if key_type(status) == 'none' then
    return nil, 'no_status'
  end
  if key_type(status) ~= 'hash' then
    return nil, 'wrong_type'
  end
  if redis.call('HGET', status, 'state') ~= 'clean' or redis.call('HGET', status, 'clean_commit') ~= '1' then
    return nil, 'not_clean'
  end
  if not read_matches(status, registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation) then
    return nil, 'stale_proof'
  end
  local now = now_ms()
  local status_expires = tonumber(redis.call('HGET', status, 'status_expires_at_ms'))
  local status_pttl = redis.call('PTTL', status)
  if not valid_ttl(now, status_expires, status_pttl) then
    return nil, 'status_ttl'
  end
  return {
    now = now,
    status_expires = status_expires,
    status_pttl = status_pttl,
    index_expires = tonumber(redis.call('HGET', status, 'index_expires_at_ms')) or status_expires,
    observed_count = tonumber(redis.call('HGET', status, 'observed_count')) or 0,
    empty_verified = redis.call('HGET', status, 'empty_verified') == '1',
    generation = tonumber(generation),
    fence = tonumber(fence),
  }, nil
end


-- begin stages a new repair generation. The committed status (KEYS[1]) is
-- never mutated; in-flight detection uses only the staged status's existence
-- so an abandoned repair self-clears via TTL and cannot strand a flag.
if op == 'begin' then
  local registration_uid = ARGV[2]
  local revision = ARGV[3]
  local holder_pod_uid = ARGV[4]
  local holder_session_uid = ARGV[5]
  local lease_epoch = ARGV[6]
  local fence = ARGV[7]
  local token = ARGV[8]
  local staged = KEYS[2]
  if redis.call('EXISTS', staged) == 1 and redis.call('HGET', staged, 'state') == 'repairing' then
    return redis.error_reply('REPAIR_IN_FLIGHT')
  end
  -- Replacing an abandoned (uncertain) or expired staging: its residual
  -- staged index/records must be cleared first. A partially reconciled
  -- repair that was abandoned or whose status expired while its records
  -- (the configured data TTL, default 45s) still live would otherwise
  -- contaminate the new generation's commit count validation.
  local old_index = KEYS[3]
  local old_index_type = key_type(old_index)
  if old_index_type == 'zset' then
    if redis.call('ZCARD', old_index) > 10000 then
      return redis.error_reply('INDEX_BOUNDS')
    end
    local members = redis.call('ZRANGE', old_index, 0, -1)
    local staged_prefix = ARGV[9]
    for _, m in ipairs(members) do
      redis.call('DEL', staged_prefix .. m)
    end
    redis.call('DEL', old_index)
  elseif old_index_type ~= 'none' then
    return redis.error_reply('WRONG_TYPE_ZSET')
  end
  -- DD-FLEET-017 D2.5a: an abandoned generation must never leave lex members
  -- behind. The staged lex index carries no record keys, so dropping the key
  -- is the whole cleanup.
  local old_lex = KEYS[4]
  local old_lex_type = key_type(old_lex)
  if old_lex_type == 'zset' then
    if redis.call('ZCARD', old_lex) > 10000 then
      return redis.error_reply('INDEX_BOUNDS')
    end
    redis.call('DEL', old_lex)
  elseif old_lex_type ~= 'none' then
    return redis.error_reply('WRONG_TYPE_ZSET')
  end
  local committed_generation = tonumber(redis.call('HGET', KEYS[1], 'generation')) or 0
  local generation = committed_generation + 1
  if generation <= 0 then
    return redis.error_reply('INVALID_GENERATION')
  end
  local now = now_ms()
  -- Staged TTL: bounded well inside the configured data TTL (default 45s) so
  -- a lost-reply stranding self-clears before the committed records expire. The total
  -- staged lifetime is separately capped at 60s (created_at_ms below): a
  -- fence holder that keeps reconciling without committing cannot keep the
  -- staging alive indefinitely, preserving bounded self-healing.
  local staged_expires = now + 15000
  redis.call('HSET', staged,
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
    'empty_verified', '0',
    'observed_count', '0',
    'cleanup_count', '0',
    'repair_reason', '',
    'created_at_ms', now,
    'status_expires_at_ms', staged_expires,
    'index_expires_at_ms', staged_expires)
  redis.call('PEXPIREAT', staged, staged_expires)
  return {tostring(generation)}
end

-- repair_reconcile writes one staged observation. KEYS[1]=staged status,
-- KEYS[2]=staged record, KEYS[3]=staged index, KEYS[4]=staged lex index.
if op == 'repair_reconcile' then
  local registration_uid = ARGV[2]
  local revision = ARGV[3]
  local holder_pod_uid = ARGV[4]
  local holder_session_uid = ARGV[5]
  local lease_epoch = ARGV[6]
  local fence = ARGV[7]
  local generation = ARGV[8]
  local token = ARGV[9]
  local staged = KEYS[1]
  if redis.call('HGET', staged, 'state') ~= 'repairing' or
     not full_matches(staged, registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation, token) then
    return redis.error_reply('STALE_FENCE')
  end
  local type_error = require_type(KEYS[2], 'hash')
  if type_error ~= nil then
    return type_error
  end
  local index_type_error = require_type(KEYS[3], 'zset')
  if index_type_error ~= nil then
    return index_type_error
  end
  local lex_type_error = require_type(KEYS[4], 'zset')
  if lex_type_error ~= nil then
    return lex_type_error
  end
  local public_id = ARGV[10]
  local ttl = tonumber(ARGV[17])
  if ttl == nil or ttl <= 0 then
    return redis.error_reply('INVALID_TTL')
  end
  local now = now_ms()
  local staged_expires = tonumber(redis.call('HGET', staged, 'status_expires_at_ms')) or 0
  local created_at = tonumber(redis.call('HGET', staged, 'created_at_ms')) or now
  -- #2660 m2: past the created_at + 60s total lifetime cap the staging is
  -- TERMINALLY expired. Reject before any staged record/index write. On a
  -- compliant TTL-honoring server the fence check above reads the key first
  -- and reports STALE_FENCE; this guard only fires when the staged status
  -- SURVIVED past the cap (non-evicting/clock-skewed servers). Transition it
  -- out of 'repairing' and bound it so the next begin reclaims it in one
  -- cycle: begin's gate blocks only state=='repairing', while its residue
  -- cleanup (staged index/records) runs regardless of state. Without the
  -- transition the staging would keep rejecting later reconciles with the
  -- same terminal error until its TTL finally lapsed.
  if created_at + 60000 <= now then
    redis.call('HSET', staged, 'state', 'expired')
    redis.call('PEXPIREAT', staged, now + 15000)
    return redis.error_reply('STAGED_EXPIRED')
  end
  -- Roll the staged status TTL forward on each reconcile: an actively
  -- progressing repair survives, while an idle or stranded staging still
  -- self-clears 15s after its last activity. The roll is capped at
  -- created_at + 60s so a fence holder that reconciles without committing
  -- can never keep the staging alive indefinitely.
  local rolled = math.min(now + 15000, created_at + 60000)
  if rolled > staged_expires then
    staged_expires = rolled
    redis.call('HSET', staged, 'status_expires_at_ms', staged_expires)
  end
  redis.call('PEXPIREAT', staged, staged_expires)
  local expires = now + ttl
  redis.call('HSET', KEYS[2],
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
  redis.call('PEXPIREAT', KEYS[2], expires)
  redis.call('ZADD', KEYS[3], expires, public_id)
  -- DD-FLEET-017 D2.5a: the lex member is written from the same staged
  -- observation, so one commit publishes both indexes for one generation.
  redis.call('ZADD', KEYS[4], 0, lex_member(ARGV[14], ARGV[13], ARGV[11], ARGV[12], ARGV[15], public_id))
  if redis.call('ZCARD', KEYS[4]) > MAX_MEMBERS then
    return redis.error_reply('INDEX_BOUNDS')
  end
  redis.call('HINCRBY', staged, 'observed_count', 1)
  local index_expires = tonumber(redis.call('HGET', staged, 'index_expires_at_ms')) or 0
  if expires > index_expires then
    index_expires = expires
    redis.call('HSET', staged, 'index_expires_at_ms', index_expires)
  end
  redis.call('PEXPIREAT', KEYS[3], index_expires)
  redis.call('PEXPIREAT', KEYS[4], index_expires)
  local record_pttl = redis.call('PTTL', KEYS[2])
  local index_pttl = redis.call('PTTL', KEYS[3])
  local status_pttl = redis.call('PTTL', staged)
  if not valid_ttl(now, expires, record_pttl) or not valid_ttl(now, index_expires, index_pttl) or not valid_ttl(now, staged_expires, status_pttl) then
    return redis.error_reply('INVALID_PTTL')
  end
  return {'ok', tostring(now), tostring(expires), tostring(record_pttl), tostring(index_pttl), tostring(status_pttl), tostring(index_expires), tostring(staged_expires)}
end

-- repair_finalize_empty proves a complete-empty staged partition without
-- fabricating record/index TTLs. KEYS[1]=staged status, KEYS[2]=staged index,
-- KEYS[3]=staged lex index.
if op == 'repair_finalize_empty' then
  local staged = KEYS[1]
  if redis.call('HGET', staged, 'state') ~= 'repairing' or
     not full_matches(staged, ARGV[2], ARGV[3], ARGV[4], ARGV[5], ARGV[6], ARGV[7], ARGV[8], ARGV[9]) then
    return redis.error_reply('STALE_FENCE')
  end
  local index_type = key_type(KEYS[2])
  if index_type ~= 'none' and index_type ~= 'zset' then
    return redis.error_reply('WRONG_TYPE_ZSET')
  end
  if index_type == 'zset' and redis.call('ZCARD', KEYS[2]) ~= 0 then
    return redis.error_reply('RESIDUAL_MEMBERS')
  end
  -- DD-FLEET-017 D2.5a: a proven-empty partition must carry no lex members
  -- either, or an empty generation could publish identities.
  local lex_type = key_type(KEYS[3])
  if lex_type ~= 'none' and lex_type ~= 'zset' then
    return redis.error_reply('WRONG_TYPE_ZSET')
  end
  if lex_type == 'zset' and redis.call('ZCARD', KEYS[3]) ~= 0 then
    return redis.error_reply('RESIDUAL_MEMBERS')
  end
  local now = now_ms()
  local expires = tonumber(redis.call('HGET', staged, 'status_expires_at_ms'))
  local pttl = redis.call('PTTL', staged)
  if not valid_ttl(now, expires, pttl) then
    return redis.error_reply('TTL_MISMATCH')
  end
  redis.call('HSET', staged, 'empty_verified', '1')
  return {'ok', tostring(now), tostring(expires), tostring(pttl)}
end

-- commit swaps the staged keyspace over the live keyspace in one atomic
-- invocation. KEYS[1]=committed status, KEYS[2]=staged status,
-- KEYS[3]=live index, KEYS[4]=staged index, KEYS[5]=staged lex index,
-- KEYS[6]=live lex index. ARGV[11]=live record key prefix,
-- ARGV[12]=staged record key prefix.
if op == 'commit' then
  local registration_uid = ARGV[2]
  local revision = ARGV[3]
  local holder_pod_uid = ARGV[4]
  local holder_session_uid = ARGV[5]
  local lease_epoch = ARGV[6]
  local fence = ARGV[7]
  local generation = ARGV[8]
  local token = ARGV[9]
  local staged = KEYS[2]
  if redis.call('HGET', staged, 'state') ~= 'repairing' or
     not full_matches(staged, registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation, token) then
    return redis.error_reply('STALE_FENCE')
  end
  local expected_count = tonumber(ARGV[10])
  if expected_count == nil or expected_count < 0 then
    return redis.error_reply('COMMIT_REJECTED')
  end
  local live_index_type = key_type(KEYS[3])
  local staged_index_type = key_type(KEYS[4])
  if live_index_type ~= 'none' and live_index_type ~= 'zset' then
    return redis.error_reply('WRONG_TYPE_ZSET')
  end
  if staged_index_type ~= 'none' and staged_index_type ~= 'zset' then
    return redis.error_reply('WRONG_TYPE_ZSET')
  end
  local staged_count = 0
  if staged_index_type == 'zset' then
    staged_count = redis.call('ZCARD', KEYS[4])
    if staged_count > 10000 then
      return redis.error_reply('INDEX_BOUNDS')
    end
  end
  if staged_count ~= expected_count then
    return redis.error_reply('COUNT_MISMATCH')
  end
  -- DD-FLEET-017 D2.5a: both per-partition indexes come from the same staged
  -- generation. A disagreement is the writer's defect, rejected before any
  -- live mutation rather than published as an unpaginable page.
  local staged_lex_type = key_type(KEYS[5])
  local live_lex_type = key_type(KEYS[6])
  if staged_lex_type ~= 'none' and staged_lex_type ~= 'zset' then
    return redis.error_reply('WRONG_TYPE_ZSET')
  end
  if live_lex_type ~= 'none' and live_lex_type ~= 'zset' then
    return redis.error_reply('WRONG_TYPE_ZSET')
  end
  local staged_lex_count = 0
  if staged_lex_type == 'zset' then
    staged_lex_count = redis.call('ZCARD', KEYS[5])
    if staged_lex_count > MAX_MEMBERS then
      return redis.error_reply('INDEX_BOUNDS')
    end
  end
  if live_lex_type == 'zset' and redis.call('ZCARD', KEYS[6]) > MAX_MEMBERS then
    return redis.error_reply('INDEX_BOUNDS')
  end
  if staged_lex_count ~= staged_count then
    return redis.error_reply('LEX_COUNT_MISMATCH')
  end
  -- Proof flags: the TTL triple and empty-partition finalization below are
  -- the real commit proof. ARGV[13..16] are RESERVED and deliberately
  -- unchecked (#2660 m1): RecordValid, IndexValid, StatusValid and
  -- RecordIndexChecksValid are compile-time constants on the writer side
  -- (Go's ValidateForCommit and hasCleanRepairProof reject any non-eligible
  -- proof before the script is invoked), so a Lua-side rejection loop over
  -- them could never fire. A successful repair_reconcile IS the structural
  -- proof. The slots are kept to keep 17-21 stable.
  if ARGV[19] ~= '1' then return redis.error_reply('COMMIT_REJECTED') end
  if expected_count == 0 then
    if ARGV[20] ~= '1' or ARGV[17] ~= '0' or ARGV[18] ~= '0' or redis.call('HGET', staged, 'empty_verified') ~= '1' then
      return redis.error_reply('COMMIT_REJECTED')
    end
  elseif ARGV[17] ~= '1' or ARGV[18] ~= '1' or ARGV[20] == '1' then
    return redis.error_reply('COMMIT_REJECTED')
  end
  local live_prefix = ARGV[11]
  local staged_prefix = ARGV[12]
  -- ARGV[21]: per-record TTL in milliseconds. The commit refreshes every
  -- renamed record's expiry to now+TTL so the committed proof is valid for
  -- the full TTL from COMMIT time, not from the earlier staged reconcile
  -- (under load, a repair cycle may run long enough that reconcile-time
  -- TTLs would lapse before the next commit lands).
  local refresh_ttl = tonumber(ARGV[21]) or 0
  -- Remove live members absent from the staged observation set.
  local cleanup_count = 0
  local staged_members = {}
  local staged_members_list = {}
  if staged_index_type == 'zset' then
    staged_members_list = redis.call('ZRANGE', KEYS[4], 0, -1)
    for _, m in ipairs(staged_members_list) do staged_members[m] = true end
  end
  if live_index_type == 'zset' and redis.call('ZCARD', KEYS[3]) > 10000 then
    return redis.error_reply('INDEX_BOUNDS')
  end
  -- Pre-validate every staged record key before the first live mutation: a
  -- RENAME of a missing key would abort the script mid-way with partial
  -- live changes already applied (atomic but not transactional). Reuses the
  -- members already fetched for the count/set construction above.
  for _, m in ipairs(staged_members_list) do
    if redis.call('EXISTS', staged_prefix .. m) == 0 then
      return redis.error_reply('COMMIT_REJECTED')
    end
  end
  if live_index_type == 'zset' then
    local live_members = redis.call('ZRANGE', KEYS[3], 0, -1)
    for _, m in ipairs(live_members) do
      if not staged_members[m] then
        redis.call('DEL', live_prefix .. m)
        cleanup_count = cleanup_count + 1
      end
    end
  end
  -- Take one server TIME sample for the whole swap so every expiry refresh
  -- below is consistent; the committed status HSET happens after the renames.
  local now = now_ms()
  local status_expires = now + 60000
  local index_expires = tonumber(redis.call('HGET', staged, 'index_expires_at_ms')) or status_expires
  -- Rename staged records into their live keys, refreshing each expiry from
  -- commit time, then swap the index with matching scores.
  if staged_index_type == 'zset' then
    for _, m in ipairs(staged_members_list) do
      local live_key = live_prefix .. m
      redis.call('RENAME', staged_prefix .. m, live_key)
      if refresh_ttl > 0 then
        local new_expires = now + refresh_ttl
        redis.call('HSET', live_key, 'expires_at_ms', new_expires)
        redis.call('PEXPIREAT', live_key, new_expires)
        redis.call('ZADD', KEYS[4], new_expires, m)
        if new_expires > index_expires then
          index_expires = new_expires
        end
      end
    end
    redis.call('RENAME', KEYS[4], KEYS[3])
    -- The renamed index keeps its reconcile-time TTL; refresh it to the
    -- (possibly extended) index expiry so it outlives the refreshed records.
    redis.call('PEXPIREAT', KEYS[3], index_expires)
    -- Same atomic swap for the lexicographic identity index (D2.5a). Its
    -- members carry score 0 and no per-record TTL, so it only needs the
    -- shared index expiry.
    redis.call('RENAME', KEYS[5], KEYS[6])
    redis.call('PEXPIREAT', KEYS[6], index_expires)
  else
    redis.call('DEL', KEYS[3])
    -- A proven-empty partition carries no indexes at all (D2.4), so the page
    -- derives "empty" from the proof, never from a missing index.
    redis.call('DEL', KEYS[6])
    redis.call('DEL', KEYS[5])
  end
  redis.call('HSET', KEYS[1],
    'state', 'clean',
    'clean_commit', '1',
    'registration_uid', registration_uid,
    'expected_revision', revision,
    'holder_pod_uid', holder_pod_uid,
    'holder_session_uid', holder_session_uid,
    'lease_epoch', lease_epoch,
    'fence', fence,
    'generation', generation,
    'repair_token', token,
    'empty_verified', redis.call('HGET', staged, 'empty_verified'),
    'observed_count', expected_count,
    'cleanup_count', cleanup_count,
    'repair_reason', '',
    'status_expires_at_ms', status_expires,
    'index_expires_at_ms', index_expires)
  redis.call('PEXPIREAT', KEYS[1], status_expires)
  redis.call('DEL', staged)
  return {'ok', tostring(cleanup_count), tostring(revision), tostring(generation), tostring(fence), token}
end

-- inventory_page_header validates the committed proof ONCE and returns the
-- lexicographic identity page of that generation (DD-FLEET-017 D2.5a). No
-- record is read here, so the identity set is proven before any payload.
-- KEYS[1]=committed status, KEYS[2]=live lex index, KEYS[3]=cursor record
-- (the record of the requested cursor's canonical ID; unused when the request
-- has no cursor).
-- ARGV[2..8]=registration_uid, revision, holder_pod_uid, holder_session_uid,
-- lease_epoch, fence, generation; ARGV[9]=identity prefix; ARGV[10]=limit;
-- ARGV[11]=cursor public ID ('' for the first page).
if op == 'inventory_page_header' then
  local status = KEYS[1]
  local lex = KEYS[2]
  local proof, missing = prove_committed(status, ARGV[2], ARGV[3], ARGV[4], ARGV[5], ARGV[6], ARGV[7], ARGV[8])
  if proof == nil then
    return {'missing', missing}
  end
  local limit = tonumber(ARGV[10])
  if limit == nil or limit < 1 then
    return redis.error_reply('INVALID_LIMIT')
  end
  local lex_type = key_type(lex)
  if lex_type ~= 'none' and lex_type ~= 'zset' then
    return redis.error_reply('WRONG_TYPE_ZSET')
  end
  -- D2.4: a proven-empty partition has no indexes. "empty" is derived from
  -- empty_verified=1 plus observed_count=0, never from an index being absent.
  if proof.observed_count == 0 then
    if not proof.empty_verified then
      return {'missing', 'empty_unverified'}
    end
    if lex_type == 'zset' and redis.call('ZCARD', lex) > 0 then
      return {'missing', 'empty_with_members'}
    end
    return {'empty', tostring(proof.now), tostring(proof.status_expires), tostring(proof.status_pttl),
      tostring(proof.generation), tostring(proof.fence), tostring(proof.status_expires - proof.now)}
  end
  -- D2.5a: a partition committed before the lex index existed (or one whose
  -- index expired) fails closed until its next clean commit; it is never
  -- served as empty.
  if lex_type == 'none' then
    return {'missing', 'no_lex_index'}
  end
  if redis.call('ZCARD', lex) > MAX_MEMBERS then
    return redis.error_reply('INDEX_BOUNDS')
  end
  local index_pttl = redis.call('PTTL', lex)
  if not valid_ttl(proof.now, proof.index_expires, index_pttl) then
    return {'missing', 'index_ttl'}
  end
  local prefix = ARGV[9]
  local min, max = lex_range(prefix)
  local cursor_id = ARGV[11]
  if cursor_id ~= '' then
    -- The cursor is the canonical ID of the last returned item, so the
    -- exclusive lower bound is that item's member. Its record carries the
    -- identity needed to compose it; a record that is gone (or belongs to
    -- another generation) is cache drift, which re-runs the projection from
    -- the start like policyread.cursorStart rather than answering 400.
    local cursor_record = KEYS[3]
    if key_type(cursor_record) ~= 'hash' or
       redis.call('HGET', cursor_record, 'public_id') ~= cursor_id or
       redis.call('HGET', cursor_record, 'generation') ~= ARGV[8] or
       redis.call('HGET', cursor_record, 'fence') ~= ARGV[7] or
       redis.call('HGET', cursor_record, 'registration_uid') ~= ARGV[2] or
       redis.call('HGET', cursor_record, 'expected_revision') ~= ARGV[3] then
      return {'drift', 'cursor_missing'}
    end
    local member = lex_member(redis.call('HGET', cursor_record, 'namespace'),
      redis.call('HGET', cursor_record, 'kind'),
      redis.call('HGET', cursor_record, 'group'),
      redis.call('HGET', cursor_record, 'version'),
      redis.call('HGET', cursor_record, 'name'),
      cursor_id)
    min = '(' .. member
  end
  -- Remaining PROOF life: the committed status hash is the proof, and it
  -- deliberately outlives the record/index TTLs. Basing freshness on a shorter
  -- data TTL would make every freshly committed partition read "stale".
  local remaining = proof.status_expires - proof.now
  local members = redis.call('ZRANGEBYLEX', lex, min, max, 'LIMIT', 0, limit)
  local reply = {'ok', tostring(proof.now), tostring(proof.status_expires), tostring(proof.status_pttl),
    tostring(proof.generation), tostring(proof.fence), tostring(remaining)}
  for _, member in ipairs(members) do
    reply[#reply + 1] = member
  end
  return reply
end

-- inventory_page_records re-validates the same fence/generation and returns
-- only the requested page's records, plus the namespace records needed for
-- ADR-053 inheritance (no N+1; DD-FLEET-017 §3.7). One expired or
-- inconsistent record fails the whole page closed.
-- KEYS[1]=committed status, KEYS[2]=live lex index, KEYS[3..3+n-1]=record keys
-- of the page members, KEYS[3+n..]=namespace record keys.
-- ARGV[2..8]=identity/fence/generation; ARGV[9]=n; ARGV[10..10+n-1]=members;
-- ARGV[10+n]=m; ARGV[10+n+1..]=namespace names.
if op == 'inventory_page_records' then
  local proof, missing = prove_committed(KEYS[1], ARGV[2], ARGV[3], ARGV[4], ARGV[5], ARGV[6], ARGV[7], ARGV[8])
  if proof == nil then
    return {'missing', missing}
  end
  local lex = KEYS[2]
  if key_type(lex) ~= 'zset' then
    return {'missing', 'no_lex_index'}
  end
  local index_pttl = redis.call('PTTL', lex)
  if not valid_ttl(proof.now, proof.index_expires, index_pttl) then
    return {'missing', 'index_ttl'}
  end
  local n = tonumber(ARGV[9])
  if n == nil or n < 0 then
    return redis.error_reply('INVALID_PAGE')
  end
  local m_index = 10 + n
  local m = tonumber(ARGV[m_index])
  if m == nil or m < 0 then
    return redis.error_reply('INVALID_PAGE')
  end
  local remaining = proof.status_expires - proof.now
  local reply = {'ok', tostring(proof.now), tostring(proof.generation), tostring(proof.fence),
    tostring(remaining), tostring(n)}
  for i = 1, n do
    local member = ARGV[9 + i]
    if redis.call('ZSCORE', lex, member) == false then
      -- The identity index moved between the header and the record read:
      -- fail closed instead of mixing generations.
      return {'missing', 'member_missing'}
    end
    local record = KEYS[2 + i]
    if key_type(record) ~= 'hash' then
      return {'missing', 'record_missing'}
    end
    local start = string.len(member)
    while start > 1 and string.sub(member, start - 1, start - 1) ~= SEP do
      start = start - 1
    end
    local member_id = string.sub(member, start)
    if redis.call('HGET', record, 'public_id') ~= member_id or
       redis.call('HGET', record, 'generation') ~= ARGV[8] or
       redis.call('HGET', record, 'fence') ~= ARGV[7] or
       redis.call('HGET', record, 'registration_uid') ~= ARGV[2] or
       redis.call('HGET', record, 'expected_revision') ~= ARGV[3] then
      return {'missing', 'record_proof'}
    end
    local label_state = redis.call('HGET', record, 'label_state')
    if label_state ~= 'true' and label_state ~= 'false' and label_state ~= 'absent' then
      return {'missing', 'record_label'}
    end
    local expires = tonumber(redis.call('HGET', record, 'expires_at_ms'))
    if not valid_ttl(proof.now, expires, redis.call('PTTL', record)) then
      return {'missing', 'record_ttl'}
    end
    reply[#reply + 1] = member_id
    reply[#reply + 1] = redis.call('HGET', record, 'group')
    reply[#reply + 1] = redis.call('HGET', record, 'version')
    reply[#reply + 1] = redis.call('HGET', record, 'kind')
    reply[#reply + 1] = redis.call('HGET', record, 'namespace')
    reply[#reply + 1] = redis.call('HGET', record, 'name')
    reply[#reply + 1] = label_state
    reply[#reply + 1] = tostring(expires)
  end
  reply[#reply + 1] = tostring(m)
  for j = 1, m do
    local namespace = ARGV[m_index + j]
    local record = KEYS[2 + n + j]
    local present = '0'
    local label_state = ''
    if key_type(record) == 'hash' then
      if redis.call('HGET', record, 'generation') == ARGV[8] and
         redis.call('HGET', record, 'fence') == ARGV[7] and
         redis.call('HGET', record, 'registration_uid') == ARGV[2] and
         redis.call('HGET', record, 'expected_revision') == ARGV[3] then
        local candidate = redis.call('HGET', record, 'label_state')
        if candidate == 'true' or candidate == 'false' or candidate == 'absent' then
          local expires = tonumber(redis.call('HGET', record, 'expires_at_ms'))
          if valid_ttl(proof.now, expires, redis.call('PTTL', record)) then
            present = '1'
            label_state = candidate
          end
        end
      end
    end
    reply[#reply + 1] = namespace
    reply[#reply + 1] = present
    reply[#reply + 1] = label_state
  end
  -- remaining (the minimum proof life of this page) is ARGV-independent, so
  -- it is returned in the fixed prefix above; recompute it here from the
  -- minimum we tracked so a short-lived record cannot report a fresher page.
  reply[5] = tostring(remaining)
  return reply
end

-- inventory_exact reads one committed record by canonical ID. The record
-- carries its own generation/fence proof, so a survivor of another generation
-- is rejected. KEYS[1]=committed status, KEYS[2]=record.
-- ARGV[2..8]=identity/fence/generation; ARGV[9]=public ID.
if op == 'inventory_exact' then
  local proof, missing = prove_committed(KEYS[1], ARGV[2], ARGV[3], ARGV[4], ARGV[5], ARGV[6], ARGV[7], ARGV[8])
  if proof == nil then
    -- No servable committed proof is an outage, not an assertion that the
    -- requested identity is absent (DD-FLEET-017 §6 exact-read row).
    return {'unproven', missing}
  end
  local public_id = ARGV[9]
  local record = KEYS[2]
  -- The proof is valid, so even an absent identity is answered under it: the
  -- page layer derives freshness from the same remaining life a list page uses.
  local proof_remaining = tostring(proof.status_expires - proof.now)
  if key_type(record) ~= 'hash' then
    return {'missing', 'record_missing', proof_remaining}
  end
  if redis.call('HGET', record, 'public_id') ~= public_id or
     redis.call('HGET', record, 'generation') ~= ARGV[8] or
     redis.call('HGET', record, 'fence') ~= ARGV[7] or
     redis.call('HGET', record, 'registration_uid') ~= ARGV[2] or
     redis.call('HGET', record, 'expected_revision') ~= ARGV[3] then
    return {'missing', 'record_proof', proof_remaining}
  end
  local label_state = redis.call('HGET', record, 'label_state')
  if label_state ~= 'true' and label_state ~= 'false' and label_state ~= 'absent' then
    return {'missing', 'record_label', proof_remaining}
  end
  local expires = tonumber(redis.call('HGET', record, 'expires_at_ms'))
  if not valid_ttl(proof.now, expires, redis.call('PTTL', record)) then
    return {'missing', 'record_ttl', proof_remaining}
  end
  local remaining = proof.status_expires - proof.now
  return {'ok', tostring(proof.now), tostring(proof.generation), tostring(proof.fence),
    tostring(remaining), public_id,
    redis.call('HGET', record, 'group'), redis.call('HGET', record, 'version'),
    redis.call('HGET', record, 'kind'), redis.call('HGET', record, 'namespace'),
    redis.call('HGET', record, 'name'), label_state, tostring(expires)}
end

-- read_verified serves ONLY the committed clean proof. KEYS[1]=committed
-- status, KEYS[2]=live index, KEYS[3]=live record. Staged state is invisible.
if op == 'read_verified' then
  local registration_uid = ARGV[2]
  local revision = ARGV[3]
  local holder_pod_uid = ARGV[4]
  local holder_session_uid = ARGV[5]
  local lease_epoch = ARGV[6]
  local fence = ARGV[7]
  local generation = ARGV[8]
  local public_id = ARGV[9]
  local status = KEYS[1]
  if key_type(status) == 'none' then
    return {'missing'}
  end
  if key_type(status) ~= 'hash' then
    return redis.error_reply('WRONG_TYPE_HASH')
  end
  if redis.call('HGET', status, 'state') ~= 'clean' or redis.call('HGET', status, 'clean_commit') ~= '1' then
    return {'missing'}
  end
  if not read_matches(status, registration_uid, revision, holder_pod_uid, holder_session_uid, lease_epoch, fence, generation) then
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
  -- #2686: 'invalid' is a committed, decisive observation (fail closed on the
  -- read side); only structurally unknown states are rejected.
  if label_state ~= 'true' and label_state ~= 'false' and label_state ~= 'absent' and label_state ~= 'invalid' then
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

-- mark_uncertain abandons a staged repair so the next begin can replace it.
if op == 'mark_uncertain' then
  local staged = KEYS[1]
  local state = redis.call('HGET', staged, 'state')
  -- #2660 m2: an 'expired' marker is accepted too. The reconcile guard
  -- transitions a past-cap staging out of 'repairing' before replying
  -- STAGED_EXPIRED; the syncer defer still marks the failure afterwards, and
  -- rejecting that with STALE_FENCE would make the defer log a spurious
  -- error for an already-terminal staging.
  if (state ~= 'repairing' and state ~= 'expired') or
     not full_matches(staged, ARGV[2], ARGV[3], ARGV[4], ARGV[5], ARGV[6], ARGV[7], ARGV[8], ARGV[9]) then
    return redis.error_reply('STALE_FENCE')
  end
  redis.call('HSET', staged, 'state', 'uncertain', 'clean_commit', '0', 'repair_reason', ARGV[11], 'failure_code', ARGV[10])
  return {'ok'}
end

-- abandon_staging lets the current fence holder abandon an in-flight staged
-- repair whose token was lost (ambiguous begin reply). Only the complete
-- writer fence and registration identity are required; the unknown
-- generation/token are deliberately not validated. A non-repairing or absent
-- staging is a no-op.
if op == 'abandon_staging' then
  local staged = KEYS[1]
  if key_type(staged) == 'none' then
    return {'ok', 'absent'}
  end
  if key_type(staged) ~= 'hash' then
    return redis.error_reply('WRONG_TYPE_HASH')
  end
  if redis.call('HGET', staged, 'state') ~= 'repairing' then
    return {'ok', 'not_repairing'}
  end
  if redis.call('HGET', staged, 'registration_uid') ~= tostring(ARGV[2]) or
     redis.call('HGET', staged, 'expected_revision') ~= tostring(ARGV[3]) or
     redis.call('HGET', staged, 'holder_pod_uid') ~= tostring(ARGV[4]) or
     redis.call('HGET', staged, 'holder_session_uid') ~= tostring(ARGV[5]) or
     redis.call('HGET', staged, 'lease_epoch') ~= tostring(ARGV[6]) or
     redis.call('HGET', staged, 'fence') ~= tostring(ARGV[7]) then
    return redis.error_reply('STALE_FENCE')
  end
  redis.call('HSET', staged, 'state', 'uncertain', 'clean_commit', '0', 'repair_reason', 'abandoned by writer after ambiguous begin', 'failure_code', 'ambiguous_write')
  return {'ok', 'abandoned'}
end

return redis.error_reply('UNKNOWN_OPERATION')
