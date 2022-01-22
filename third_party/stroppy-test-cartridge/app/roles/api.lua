local cartridge = require("cartridge")
local log = require("log")
local errors = require("errors")
local decimal = require("decimal")
local uuid = require("uuid")
local fiber = require("fiber")
local custom_errors = require("app.custom_errors")

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

local function isTransientError(error)
	-- Timeout exceeded
	if error.code == 78 then
		return true
	end
end

local function json_response(req, json, status)
	local resp = req:render({ json = json })
	resp.status = status
	return resp
end

local function internal_error_response(req, error)
	local resp
    -- для корректной передачи разных форматов в одном общем виде
	if isTransientError(error) then
		resp = json_response(req, {
			info = error.class_name,
			error = error.err,
		}, 500)
	else
		resp = json_response(req, {
			info = error.class_name,
			error = error.message,
		}, 500)
	end

	return resp
end

local function entity_not_found_response(req, error)
	local resp = json_response(req, {
		info = error.class_name,
		error = error.err,
	}, 404)
	return resp
end

local function entity_conflict_response(req, error)
	local resp = json_response(req, {
		info = error.class_name,
		error = error.err,
	}, 409)
	return resp
end

local function storage_error_response(req, error)
	if isConflictError(error) then
		return entity_conflict_response(req, error)
	elseif isNotFoundError(error) then
		return entity_not_found_response(req, error)
	else
		return internal_error_response(req, error)
	end
end

local function http_account_add(req)
	local account = req:json()
	log.debug(account)
	local router = cartridge.service_get("vshard-router").get()
	account.bucket_id = router:bucket_id_mpcrc32(account.bic .. account.ban)

	local resp, error = err_vshard_router:pcall(
		router.call,
		router,
		account.bucket_id,
		"write",
		"account_add",
		{ account }
	)

	if error then
		log.debug({ "http_account_add: request execution error:", error })
		return internal_error_response(req, error)
	end

	if resp ~= nil and resp.error then
		log.debug({ "http_account_add: storage error:", resp.error })
		return storage_error_response(req, resp.error)
	end

	return json_response(req, { info = "Successfully created" }, 201)
end

local function http_account_balance_update(req)
	local account = req:json()
	local resp, error = account_balance_update(account)
	if error then
		log.error(error)
		return internal_error_response(req, error)
	end

	if resp ~= nil and resp.error then
		log.error(resp.error)
		return storage_error_response(req, resp.error)
	end

	return json_response(req, { info = "Successfully updated" }, 200)
end

local function account_balance_update(account)
	log.debug({ "account_balance_update: got account: ", account })
	local router = cartridge.service_get("vshard-router").get()
	account.bucket_id = router:bucket_id_mpcrc32(account.bic .. account.ban)

	local resp, error = err_vshard_router:pcall(
		router.call,
		router,
		account.bucket_id,
		"write",
		"account_balance_update",
		{ account }
	)

	if error then
		log.error(error)
		return nil, error
	end

	if resp ~= nil and resp.error then
		log.error(resp.error)
		return resp, nil
	end
end

local function insert_transfer(transfer)
	log.debug(transfer)
	local router = cartridge.service_get("vshard-router").get()
	transfer.bucket_id = router:bucket_id_mpcrc32(transfer.transfer_id)

	local resp, error = err_vshard_router:pcall(
		router.call,
		router,
		transfer.bucket_id,
		"write",
		"insert_transfer",
		{ transfer }
	)

	if error then
		log.debug({ "insert_transfer: request execution error:", error })
		return nil, error
	end

	if resp ~= nil and resp.error then
		log.debug({ "insert_transfer: storage error:", resp.error })
		return resp, nil
	end

	return resp, nil
end

local function http_fetch_total(req)
	local total = {}

	local router = cartridge.service_get("vshard-router").get()
	local resp, error = err_vshard_router:pcall(router.call, router, 1, "read", "fetch_total", {})

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

	return json_response(req, { info = decimal.new(total.total) }, 200)
end

local function http_persist_total(req)
	local total = req:json()
	local router = cartridge.service_get("vshard-router").get()

	local _, error = err_vshard_router:pcall(router.call, router, 1, "write", "persist_total", { total })

	if error then
		log.error(error)
		return internal_error_response(req, error)
	end

	return json_response(req, { info = "Succesfully persist total DB" }, 200)
end

local function http_calculate_balance(req)
	local router = cartridge.service_get("vshard-router").get()
	local totalBalance = 0
	local shards, err = router:routeall()
	if err then
		log.err("failed to call routecall(): %s", err)
		return internal_error_response(req, error)
	end
	log.debug("shards info: %s", shards)
	for _, replica in pairs(shards) do
		local set = replica:callrw("calculate_accounts_balance")
		totalBalance = totalBalance + set
	end

	return json_response(req, { info = totalBalance }, 200)
end

local function http_fetch_settings(req)
	local settings = {}

	local router = cartridge.service_get("vshard-router").get()
	local resp, error = err_vshard_router:pcall(router.call, router, 1, "read", "fetch_settings", {})

	log.debug("response: %s", resp)

	if error then
		log.error(error)
		return internal_error_response(req, error)
	end

	if resp ~= nil and resp.error then
		return storage_error_response(req, resp.error)
	end

	settings.count = resp[1][2]
	settings.seed = resp[2][2]

	return json_response(req, { info = settings }, 200)
end

local function http_bootstrap_db(req)
	local settings = req:json()
	log.debug(settings)
	local router = cartridge.service_get("vshard-router").get()
	local shards, err = router:routeall()
	if err then
		log.err("failed to call routecall(): %s", err)
		return internal_error_response(req, error)
	end
	log.debug("shards info: %s", shards)

	-- чистим таблицы аналогично логике stroppy
	for _, replica in pairs(shards) do
		replica:callrw("box.space.accounts:truncate")
		replica:callrw("box.space.transfers:truncate")
		replica:callrw("box.space.settings:truncate")
		replica:callrw("box.space.checksum:truncate")
	end

	local _, error = err_vshard_router:pcall(router.call, router, 1, "write", "insert_settings", { settings })

	if error then
		log.error(error)
		return internal_error_response(req, error)
	end

	return json_response(req, { info = "Succesfully bootstraping DB" }, 201)
end

local function set_transfer_client(transfer)
	log.debug({ "set_transfer_client: got transfer: ", transfer })
	local router = cartridge.service_get("vshard-router").get()
	transfer.bucket_id = router:bucket_id_mpcrc32(transfer.transfer_id)

	local resp, error = err_vshard_router:pcall(
		router.call,
		router,
		transfer.bucket_id,
		"write",
		"set_storage_transfer_client",
		{ transfer }
	)

	if error then
		log.debug({ "set_transfer_client: request execution error:", error })
		return nil, error
	end

	if resp ~= nil and resp.error then
		log.debug({ "set_transfer_client: storage error:", resp.error })
		return resp, nil
	end

	return resp, nil
end

local function set_transfer_state(transfer)
	log.debug({ "set_transfer_state: got transfer: ", transfer })
	local router = cartridge.service_get("vshard-router").get()
	transfer.bucket_id = router:bucket_id_mpcrc32(transfer.transfer_id)

	local resp, error = err_vshard_router:pcall(
		router.call,
		router,
		transfer.bucket_id,
		"write",
		"set_storage_transfer_state",
		{ transfer }
	)

	if error then
		log.debug({ "set_transfer_state: request execution error:", error })
		return nil, error
	end

	if resp ~= nil and resp.error then
		log.debug({ "set_transfer_state: storage error:", resp.error })
		return resp, nil
	end

	return resp, nil
end

local function check_transfer_status(transfer)
	log.debug({ "check_transfer_status: got transfer: ", transfer })
	local router = cartridge.service_get("vshard-router").get()
	transfer.bucket_id = router:bucket_id_mpcrc32(transfer.transfer_id)

	local resp, error = err_vshard_router:pcall(
		router.call,
		router,
		transfer.bucket_id,
		"read",
		"box.space.transfers:get",
		{ uuid.fromstr(transfer.transfer_id) }
	)

	if error then
		log.debug({ "check_transfer_status: request execution error:", error })
		return nil, error
	end

	if resp ~= nil and resp.error then
		log.debug({ "check_transfer_status: storage error:", resp.error })
		return resp, nil
	end

	return resp, nil
end

local function get_account_balance(account_attr)
	log.debug({ "get_account_balance: got bic and ban: ", { account_attr.bic, account_attr.ban } })
	local router = cartridge.service_get("vshard-router").get()
	local bucket_id = router:bucket_id_mpcrc32(account_attr.bic .. account_attr.ban)

	local resp, error = err_vshard_router:pcall(
		router.call,
		router,
		bucket_id,
		"read",
		"box.space.accounts:get",
		{ account_attr }
	)

	if error then
		log.debug({ "get_account_balance: request execution error:", error })
		return nil, error
	end

	if resp ~= nil and resp.error then
		log.debug({ "get_account_balance: storage error:", resp.error })
		return resp, nil
	end

	return resp, nil
end

local function set_account_lock(account)
	log.debug({ "set_account_lock: got account: ", account })
	local router = cartridge.service_get("vshard-router").get()
	local bucket_id = router:bucket_id_mpcrc32(account.bic .. account.ban)

	local resp, error = err_vshard_router:pcall(router.call, router, bucket_id, "write", "lock_account", { account })

	if error then
		log.debug({ "set_account_lock: request execution error:", error })
		return nil, error
	end

	if resp ~= nil and resp.error then
		log.debug({ "set_account_lock: storage error:", resp.error })
		return resp, nil
	end

	return resp, nil
end

local function set_account_unlock(account)
	log.debug({ "set_account_unlock: got account: ", account })
	local router = cartridge.service_get("vshard-router").get()
	local bucket_id = router:bucket_id_mpcrc32(account.bic .. account.ban)

	local resp, error = err_vshard_router:pcall(router.call, router, bucket_id, "write", "unlock_account", { account })

	if error then
		log.debug({ "set_account_unlock: request execution error:", error })
		return nil, error
	end

	if resp ~= nil and resp.error then
		log.debug({ "set_account_unlock: storage error:", resp.error })
		return resp, nil
	end

	return resp, nil
end

local function http_fetch_transfer(transfer)
	log.debug({ "http_fetch_transfer: got transfer: ", { transfer } })
	local router = cartridge.service_get("vshard-router").get()
	local bucket_id = router:bucket_id_mpcrc32(transfer.transfer_id)

	local resp, error = err_vshard_router:pcall(router.call, router, bucket_id, "read", "fetch_transfer", { transfer })

	if error then
		log.debug({ "http_fetch_transfer: request execution error:", error })
		return nil, error
	end

	if resp ~= nil and resp.error then
		log.debug({ "http_fetch_transfer: storage error:", resp.error })
		return resp, nil
	end

	return resp, nil
end

local function http_make_atomic_transfer(req)
	local transfer = req:json()
	local maxTimeout = 10
	--делаем милисекунды
	local current_timeout = math.random(maxTimeout) / 1000

	--1. Регистрация транзакции
	local resp, error = insert_transfer(transfer)
	if error then
		log.debug({ "http_make_atomic_transfer: request execution error:", error })
		return internal_error_response(req, error)
	end

	if resp ~= nil and resp.error then
		log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
		return storage_error_response(req, resp.error)
	end

	log.debug({ "success insert transfer with transfer_id: ", transfer.transfer_id })

	-- 2. Обновление трансфера, добавление id клиента и timestamp
	local resp, error = set_transfer_client(transfer)
	if error then
		log.debug({ "http_make_atomic_transfer: request execution error:", error })
		return internal_error_response(req, error)
	end

	if resp ~= nil and resp.error then
		log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
		return storage_error_response(req, resp.error)
	end

	log.debug({ "success update transfer with transfer_id: ", transfer.transfer_id })

	-- 3. Блокировка счетов
	-- возможно, излишне, но кажется, что нет
	local resp, error = check_transfer_status(transfer)
	if error then
		log.debug({ "http_make_atomic_transfer: request execution error:", error })
		return internal_error_response(req, error)
	end
	if resp ~= nil and resp.error then
		log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
		return storage_error_response(req, resp.error)
	end

	--делаем массив из bic и bac счета-источника и счета-приемника для удобства обхода
	local account_array = {
		{ bic = transfer.src_bic, ban = transfer.src_ban },
		{ bic = transfer.dest_bic, ban = transfer.dest_ban },
	}
	-- проверяем статус трансфера
	if resp[7] == "complete" then
		return json_response(req, { info = "Succesfully complete transfer" }, 200)
		-- если "locked", пробуем получить и сохранить балансы по обоим счетам
	elseif resp[7] == "locked" then
		for i = 1, 2 do
			local resp, error = get_account_balance(account_array[i])
			if error then
				log.debug({ "http_make_atomic_transfer: request execution error:", error })
				return internal_error_response(req, error)
			end
			if resp ~= nil and resp.error then
				log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
				return storage_error_response(req, resp.error)
			end
			account_array[i]["balance"] = resp["balance"]
			account_array[i]["pending_amount"] = resp["pending_amount"]
			account_array[i]["Found"] = true
		end
	end

	local previosAccount = {}
	for i = 1, 2 do
		account_array[i]["pending_transfer"] = uuid.fromstr(transfer.transfer_id)
		account_array[i]["pending_amount"] = decimal.new(transfer.amount)
		local resp, error = set_account_lock(account_array[i])
		if error or (resp ~= nil and resp.error) then
			if i == 2 and previosAccount ~= nil then
				local resp, error = set_account_unlock(previosAccount)
				if error then
					log.debug({ "http_make_atomic_transfer: request execution error:", error })
					return internal_error_response(req, error)
				end
				if resp ~= nil and resp.error then
					log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
					return storage_error_response(req, resp.error)
				end
			end

			if resp ~= nil and resp.error then
				log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
				if resp.error.err == custom_errors.storageNotFoundErrors.AccNotFound.err then
					log.debug("мы тут")
					transfer.state = "locked"
					local resp_state, err = set_transfer_state(transfer)
					if err then
						log.debug({ "http_make_atomic_transfer: request execution error:", err })
						return internal_error_response(req, err)
					end
					if resp_state ~= nil and resp_state.error then
						log.debug({ "http_make_atomic_transfer: storage error:", resp_state.error })
						return storage_error_response(req, resp_state.error)
					end
				end
				return storage_error_response(req, resp.error)
			elseif isTransientError(error) then
				log.debug("transfer_id %s Retrying after error: %s %s", transfer.transfer_id, error)
			else
				return internal_error_response(req, errors:new("failed to execute lock accounts request: %s", error))
			end

			i = 1
			fiber.sleep(current_timeout)
			current_timeout = current_timeout * 2
			if current_timeout > maxTimeout then
				current_timeout = maxTimeout
			end
			account_array[i]["Found"] = false
			account_array[i + 1]["Found"] = false
			previosAccount = nil

			resp, error = set_transfer_client(transfer)
			if error then
				log.debug({ "http_make_atomic_transfer: request execution error:", error })
				return internal_error_response(req, error)
			end
			if resp ~= nil and resp.error then
				log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
				return storage_error_response(req, resp.error)
			end
		else
			--если все ок
			account_array[i]["balance"] = resp["result"][3]
			account_array[i]["pending_transfer"] = resp["result"][4]
			account_array[i]["pending_amount"] = resp["result"][5]
			account_array[i]["Found"] = true
			previosAccount = account_array[i]
		end
	end

	-- меняем статус трансфера на "locked"
	transfer.state = "locked"
	resp, error = set_transfer_state(transfer)
	if error then
		log.debug({ "http_make_atomic_transfer: request execution error:", error })
		return internal_error_response(req, error)
	end
	if resp ~= nil and resp.error then
		log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
		return storage_error_response(req, resp.error)
	end

	resp, error = http_fetch_transfer(transfer)
	if error then
		log.debug({ "http_make_atomic_transfer: request execution error:", error })
		return internal_error_response(req, error)
	end
	if resp ~= nil and resp.error then
		log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
		return storage_error_response(req, resp.error)
	end

	log.debug({ "текущее состояние трансфера", resp })

	if resp["result"][6] ~= "locked" and resp["result"][6] ~= "complete" then
		return storage_error_response(req, custom_errors.storageConflictErrors.TransferIncorrectState)
	end
	log.debug({ "счета перед обновлением", account_array })
	-- обновляем балансы сначала в переменных, потом в БД, если проходим по условиям
	if resp["result"][6] == "locked" then
		if account_array[1]["Found"] == true and account_array[2]["Found"] then
			for i = 1, 2 do
				if account_array[i]["bic"] == transfer.src_bic and account_array[i]["ban"] == transfer.src_ban then
					account_array[i]["balance"] = account_array[i]["balance"] - account_array[i]["pending_amount"]
				elseif
					account_array[i]["bic"] == transfer.dest_bic and account_array[i]["ban"] == transfer.dest_ban
				then
					account_array[i]["balance"] = account_array[i]["balance"] + account_array[i]["pending_amount"]
				end
			end
			log.debug({ "счета перед обновлением #2", account_array })
			if account_array[1]["balance"] > 0 then
				for i = 1, 2 do
					resp, error = account_balance_update(account_array[i])
					if error then
						log.debug({ "http_make_atomic_transfer: request execution error:", error })
						return internal_error_response(req, error)
					end
					if resp ~= nil and resp.error then
						log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
						return storage_error_response(req, resp.error)
					end
				end
			else
				return internal_error_response(req, custom_errors.storageConflictErrors.AccErrInsufficientFunds)
			end
		end
	end

	transfer.state = "complete"
	resp, error = set_transfer_state(transfer)
	if error then
		log.debug({ "http_make_atomic_transfer: request execution error:", error })
		return internal_error_response(req, error)
	end
	if resp ~= nil and resp.error then
		log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
		return storage_error_response(req, resp.error)
	end

	for i = 1, 2 do
		resp, error = set_account_unlock(account_array[i])
		if error then
			log.debug({ "http_make_atomic_transfer: request execution error:", error })
			return internal_error_response(req, error)
		end
		if resp ~= nil and resp.error then
			log.debug({ "http_make_atomic_transfer: storage error:", resp.error })
			return storage_error_response(req, resp.error)
		end
	end

	return json_response(req, { info = "Successfully transfer execution" }, 200)
end

local function init(opts)
	if opts.is_master then
		box.schema.user.create("stroppy", { if_not_exists = true })
		box.schema.user.grant("stroppy", "super", nil, nil, { if_not_exists = true })
		box.schema.user.passwd("stroppy", "stroppy")
	end

	local httpd = cartridge.service_get("httpd")

	if not httpd then
		return nil, err_httpd:new("not found")
	end

	log.debug("Starting httpd")
	-- Навешиваем функции-обработчики
	httpd:route({ path = "/account/insert", method = "POST", public = true }, http_account_add)
	httpd:route({ path = "/account/update_balance", method = "PUT", public = true }, http_account_balance_update)
	httpd:route({ path = "/total_balance/fetch", method = "GET", public = true }, http_fetch_total)
	httpd:route({ path = "/total_balance/persist", method = "POST", public = true }, http_persist_total)
	httpd:route({ path = "/balance/check", method = "GET", public = true }, http_calculate_balance)

	httpd:route({ path = "/settings/fetch", method = "GET", public = true }, http_fetch_settings)
	httpd:route({ path = "/db/bootstrap", method = "POST", public = true }, http_bootstrap_db)

	httpd:route({ path = "/transfer/custom/create", method = "POST", public = true }, http_make_atomic_transfer)

	httpd:route({ path = "/transfer/custom/fetch", method = "POST", public = true }, http_fetch_transfer)

	log.debug("Created httpd")
	return true
end

return {
	role_name = "api",
	init = init,
	dependencies = {
		"cartridge.roles.vshard-router",
	},
}
