local cartridge = require('cartridge')
local log = require('log')
local errors = require('errors')
local decimal = require('decimal')

local err_vshard_router = errors.new_class("Vshard routing error")
local err_httpd = errors.new_class("httpd error")

local function isNotFoundError(err)
    if err.class_name == "NotFoundError" then
        return true
    else 
        return false
    end 
end

local function isConflictError(err)
    if err.class_name == "ConflictError" then
        return true
    else 
        return false
    end 
end

local function json_response(req, json, status) 
    local resp = req:render({json = json})
    resp.status = status
    return resp
end

local function internal_error_response(req, error)
    local resp = json_response(req, {
        info = "Internal error",
        error = error
    }, 500)
    return resp
end

local function entity_not_found_response(req, error)
    local resp = json_response(req, {
        info = error.class_name,
        error = error.err
    }, 404)
    return resp
end

local function entity_conflict_response(req, error)
    local resp = json_response(req, {
        info = error.class_name,
        error = error.err
    }, 409)
    return resp
end


local function storage_error_response(req, error)
    if isConflictError(error) then
        return entity_conflict_response(req, error)
    elseif isNotFoundError(error) 
    then
        return entity_not_found_response(req, error)
    else
        return internal_error_response(req, error)
    end
end

local function http_account_add(req)
    local account = req:json()
    log.debug(account)
    local router = cartridge.service_get('vshard-router').get()
    account.bucket_id = router:bucket_id_mpcrc32(account.bic..account.ban)

    local resp, error = err_vshard_router:pcall(
        router.call,
        router,
        account.bucket_id,
        'write',
        'account_add',
        {account}
    )
    
    if error then
        log.info(error)
        return internal_error_response(req, error)
    end

    if resp ~= nil and resp.error then
        return storage_error_response(req, resp.error)
    end
    
    return json_response(req, {info = "Successfully created"}, 201)
end

local function http_account_balance_update(req)
    local account = req:json()
    local router = cartridge.service_get('vshard-router').get()
    account.bucket_id = router:bucket_id_mpcrc32(account.bic..account.ban)

    local resp, error = err_vshard_router:pcall(
        router.call,
        router,
        account.bucket_id,
        'write',
        'account_balance_update',
        {account}
    )

    if error then
        log.info(error)
        return internal_error_response(req, error)
    end

    if resp ~= nil and resp.error then
        return storage_error_response(req, resp.error)
    end
    
    return json_response(req, {info = "Successfully updated"}, 200)
end


local function http_transfer_add(req)
    local transfer = req:json()
    local router = cartridge.service_get('vshard-router').get()
    transfer.bucket_id = router:bucket_id_mpcrc32(transfer.transfer_id)

    local resp, error = err_vshard_router:pcall(
        router.call,
        router,
        transfer.bucket_id,
        'write',
        'transfer_add',
        {transfer}
    )

    if error then
        log.info(error)
        return internal_error_response(req, error)
    end

    if resp ~= nil and resp.error then
        return storage_error_response(req, resp.error)
    end
    
    return json_response(req, {info = "Successfully created"}, 201)
end

local function http_fetch_total(req)
    local total = {}

    local router = cartridge.service_get('vshard-router').get()
    local resp, error = err_vshard_router:pcall(
        router.call,
        router,
        1,
        'read',
        'fetch_total',
        {}
    )

    log.debug("response of fetch total: %s", resp)

    if error then
        log.error(error)
        return internal_error_response(req, error)
    end

    if resp ~= nil and resp.error then
        return storage_error_response(req, resp.error)
    end

    log.debug(resp)

    total.total = resp[1][2]
    
    return json_response(req, {info = decimal.new(total.total)}, 200)
end

local function http_persist_total(req)
    local total = req:json()
    local router = cartridge.service_get('vshard-router').get()

    local _, error = err_vshard_router:pcall(
        router.call,
        router,
        1,
        'write',
        'persist_total',
        {total}
    )

    if error then
        log.info(error)
        return internal_error_response(req, error)
    end
    
    return json_response(req, {info = "Succesfully persist total DB"}, 200)
end

local function http_calculate_balance(req)
    local router = cartridge.service_get('vshard-router').get()
    local totalBalance = 0
    local shards, err = router:routeall()
    if err then
        log.err("failed to call routecall(): %s", err)
        return internal_error_response(req, error)
    end
    log.debug("shards info: %s", shards)
    for _, replica in pairs(shards) do
        local set = replica:callrw('calculate_accounts_balance')
        totalBalance = totalBalance+set
    end
    
    return json_response(req, {info = totalBalance}, 200)

end

local function http_fetch_settings(req)
    local settings = {}

    local router = cartridge.service_get('vshard-router').get()
    local resp, error = err_vshard_router:pcall(
        router.call,
        router,
        1,
        'read',
        'fetch_settings',
        {}
    )

    log.debug("response: %s", resp)

    if error then
        log.info(error)
        return internal_error_response(req, error)
    end

    if resp ~= nil and resp.error then
        return storage_error_response(req, resp.error)
    end

    settings.count = resp[1][2]
    settings.seed =  resp[2][2]
    
    return json_response(req, {info = settings}, 200)
end

local function http_bootstrap_db(req)
    local settings = req:json()
    log.debug(settings)
    local router = cartridge.service_get('vshard-router').get()
    local shards, err = router:routeall()
    if err then
        log.err("failed to call routecall(): %s", err)
        return internal_error_response(req, error)
    end
    log.debug("shards info: %s", shards)

     -- чистим таблицы аналогично логике stroppy
    for _, replica in pairs(shards) do
        replica:callrw('box.space.accounts:truncate')
        replica:callrw('box.space.transfers:truncate')
        replica:callrw('box.space.settings:truncate')
        replica:callrw('box.space.checksum:truncate')
    end

    local _, error = err_vshard_router:pcall(
        router.call,
        router,
        1,
        'write',
        'insert_settings',
        {settings}
    )

    if error then
        log.info(error)
        return internal_error_response(req, error)
    end
    
    return json_response(req, {info = "Succesfully bootstraping DB"}, 200)
    
end


local function init(opts)
    if opts.is_master then
        box.schema.user.create('stroppy', {if_not_exists = true})
        box.schema.user.grant('stroppy', 'super', nil, nil, { if_not_exists = true })
        box.schema.user.passwd('stroppy', 'stroppy')
    end

    local httpd = cartridge.service_get('httpd')

    if not httpd then
        return nil, err_httpd:new("not found")
    end

    log.info("Starting httpd")
    -- Навешиваем функции-обработчики
    httpd:route(
        { path = '/insert_account', method = 'POST', public = true },
        http_account_add
    )
    httpd:route(
        { path = '/account_balance_update', method = 'PUT', public = true },
        http_account_balance_update
        )
    httpd:route(
        { path = '/insert_transfer', method = 'POST', public = true },
        http_transfer_add
        )
    httpd:route(
        { path = '/fetch_total', method = 'GET', public = true },
        http_fetch_total
        )
    httpd:route(
        { path = '/persist_total', method = 'POST', public = true },
        http_persist_total
        )
    httpd:route(
        { path = '/check_balance', method = 'GET', public = true },
        http_calculate_balance
    )

   httpd:route(
        { path = '/fetch_settings', method = 'GET', public = true },
        http_fetch_settings
    )
    httpd:route(
        { path = '/bootstrap_db', method = 'POST', public = true },
        http_bootstrap_db

    )

    log.info("Created httpd")
    return true
end

return {
    role_name = 'api',
    init = init,
    dependencies = {
        'cartridge.roles.vshard-router'
    }
}

