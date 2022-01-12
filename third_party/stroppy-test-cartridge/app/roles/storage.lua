local log = require('log')
local errors = require('errors')
local err_storage = errors.new_class("Storage error")
local uuid = require("uuid")


local function account_add(account)
       log.info(account)
       -- Проверяем на дубликаты
       local exist = box.space.accounts:get({account.bic, account.ban})
       if exist ~= nil then
           return {ok = false, error = err_storage:new("Account already exist")}
       end

       box.space.accounts:insert(box.space.accounts:frommap(account))

       return {ok = true, error = nil}
end

local function transfer_add(transfer)
    log.debug(transfer)
    -- Проверяем на дубликаты
    local exist = box.space.transfers:get({uuid.fromstr(transfer.transfer_id)})
    if exist ~= nil then
        return {ok = false, error = err_storage:new("Transfer already exist")}
    end

    box.space.transfers:insert({uuid.fromstr(transfer.transfer_id), transfer.src_bic,transfer.src_ban, transfer.dest_bic, transfer.dest_ban, transfer.amount, 
        transfer.bucket_id})
    
    return {ok = true, error = nil}
end

local function account_update(account)
   -- Проверяем на дубликаты
    local exist = box.space.accounts:get({account.bic, account.ban})
    if exist == nil then
        return {ok = false, error = err_storage:new("Account not found")}
    end

    box.space.accounts:replace(box.space.accounts:frommap(account))

    return {ok = true, error = nil}
end

local function init(opts)
    if opts.is_master then
        -- cоздаем спейсы, если не созданы
        local accounts = box.schema.space.create('accounts', { if_not_exists = true })
        accounts:format({
            { name = "bic", type = "string" },
            { name = "ban", type = "string" },
            {name="balance", type="number"},
            {name="bucket_id", type="unsigned"},
        })
        accounts:create_index('primary', { parts={{field='bic'}, {field='ban'}},
            if_not_exists=true })
        accounts:create_index('bucket_id', { parts={{field="bucket_id"}},
            unique=false,
            if_not_exists=true })   

        local transfers = box.schema.space.create('transfers', { if_not_exists=true })
        transfers:format({
                {name="transfer_id", type="uuid"},
                {name="src_bic", type="string"},
                {name="src_ban", type="string"},
                {name="dest_bic", type="string"},
                {name="dest_ban", type="string"},
                {name="balance", type="number"},
                {name="bucket_id", type="unsigned"},
            })
        transfers:create_index('primary', { parts={{field='transfer_id'}},
                if_not_exists=true })
        transfers:create_index('bucket_id', { parts={{field="bucket_id"}},
                unique=false,
                if_not_exists=true })

        local settings = box.schema.space.create('settings', { if_not_exists=true })
        settings:format({
            {name="key", type="string"},
            {name="value", type="number"},
        })
        settings:create_index('primary', { parts={{field='key'}},
        if_not_exists=true })

        local settings = box.schema.space.create('checksum', { if_not_exists=true })
        settings:format({
            {name="name", type="string"},
            {name="amount", type="number"},
        })
        settings:create_index('primary', { parts={{field='name'}},
        if_not_exists=true })

        box.schema.func.create('account_add', {if_not_exists = true})
        box.schema.func.create('account_update', {if_not_exists = true})
        box.schema.func.create('transfer_add', {if_not_exists = true})
        rawset(_G, 'account_add', account_add)
        rawset(_G, 'account_update', account_update)
        rawset(_G, 'transfer_add', transfer_add)
    end
end

--[[
    Роль не пользуется кластерным конфигом
]]
local function validate_config(new_conf, old_conf)
    return true
end
local function apply_config(conf, opts)
    return true
end

local function stop()
end

return {
    role_name="storage",

    init=init,
    validate_config=validate_config,
    apply_config=apply_config,
    stop=stop,
    utils = {
        account_add = account_add,
        account_update = account_update,
        transfer_add = transfer_add,
    },
    dependencies={'cartridge.roles.vshard-storage'}
}