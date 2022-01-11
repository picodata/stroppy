local cartridge = require('cartridge')
local log = require('log')
local errors = require('errors')

local err_vshard_router = errors.new_class("Vshard routing error")
local err_httpd = errors.new_class("httpd error")

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

local function profile_not_found_response(req)
    local resp = json_response(req, {
        info = "Profile not found"
    }, 404)
    return resp
end

local function profile_conflict_response(req)
    local resp = json_response(req, {
        info = "Profile already exist"
    }, 409)
    return resp
end

local function profile_unauthorized(req)
    local resp = json_response(req, {
        info = "Unauthorized"
    }, 401)
    return resp
end


local function http_account_add(req)
    local account = req:json()
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

    log.info(error)

    if error then
        return internal_error_response(req, error)
    end
    
    return json_response(req, {info = "Successfully created"}, 201)
end

local function  http_account_get(req)
    log.info('test')
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
        { path = '/account', method = 'POST', public = true },
        http_account_add
    )
    httpd:route(
        { path = '/account/:bic/:ban', method = 'GET', public = true },
        http_account_get
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

