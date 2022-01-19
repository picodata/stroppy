local t = require('luatest')
local g = t.group('unit_storage_utils')
local helper = require('test.helper.unit')
local uuid = require("uuid")
local decimal = require("decimal")

require('test.helper.unit')

local storage = require('app.roles.storage')
local utils = storage.utils
local deepcopy = helper.shared.deepcopy

-- взято из https://pastebin.com/CYNn4bfs
local function rand_str(len)
    len = tonumber(len) or 1
    local function rand_char()
        return math.random() > 0.5
            and string.char(math.random(65, 90))
            or string.char(math.random(97, 122))
    end
    local function rand_num()
        return string.char(math.random(48, 57))
    end

    local str = ""
    for i = 1, len do
        str = str .. (math.random() > 0.5 and rand_char() or rand_num())
    end
    return str
end

local test_account = {
    bic = rand_str(10),
    ban = rand_str(10),
    balance = decimal.new(123456),
    pending_amount = 0,
    bucket_id = 1
}

local test_transfer = {
    transfer_id = uuid.str(),
    src_bic = rand_str(10),
    src_ban = rand_str(10),
    dest_bic = rand_str(10),
    dest_ban = rand_str(10),
    state = "new",
	client_id = uuid.str(),
	client_timestamp = 123456789,
    amount = decimal.new(555),
    bucket_id = 1
}

g.test_account_add_ok = function ()
    local to_insert = deepcopy(test_account)
    t.assert_equals(utils.account_add(to_insert), {ok = true, error = nil})
end

g.test_account_balance_update_ok = function ()
    local to_update = deepcopy(test_account)
    to_update.balance = to_update.balance+10
    t.assert_equals(utils.account_balance_update(to_update), {ok = true, error = nil})
end

g.test_transfer_add_ok = function ()
    local to_insert = deepcopy(test_transfer)
    t.assert_equals(utils.insert_transfer(to_insert), {ok = true, error = nil})
end

g.before_all(function()
    storage.init({is_master = true})
    box.space.accounts:truncate()
    box.space.transfers:truncate()
    box.space.settings:truncate()
    box.space.checksum:truncate()
end)
