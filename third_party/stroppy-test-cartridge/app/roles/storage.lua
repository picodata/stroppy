local log = require("log")
local uuid = require("uuid")
local decimal = require("decimal")
local custom_errors = require("app.custom_errors")
local fiber = require("fiber")

local function account_add(account)
	log.debug(account)
	-- Проверяем на дубликаты
	local exist = box.space.accounts:get({ account.bic, account.ban })
	if exist ~= nil then
		return { ok = false, error = custom_errors.storageConflictErrors.AccountAlReadyExist }
	end

	account.balance = decimal.new(account.balance)
	account.pending_amount = decimal.new(account.pending_amount)

	box.space.accounts:insert({
		account.bic,
		account.ban,
		account.balance,
		nil, --при загрузке счета должен быть пустой
		account.pending_amount,
		account.bucket_id,
	})

	return { ok = true, error = nil }
end

local function account_balance_update(new_account)
	-- Проверяем, есть ли счет
	local current_account = box.space.accounts:get({ new_account.bic, new_account.ban })
	if current_account == nil then
		return { ok = false, error = custom_errors.storageNotFoundErrors.AccNotFound }
	end

	box.space.accounts:update(
		{ current_account.bic, current_account.ban },
		{ { "=", 3, decimal.new(new_account.balance) } }
	)

	return { ok = true, error = nil }
end

local function insert_transfer(transfer)
	log.info({ "storage: insert_transfer: got transfer:", transfer })
	-- Проверяем на дубликаты
	local exist = box.space.transfers:get({ uuid.fromstr(transfer.transfer_id) })
	if exist ~= nil then
		return { result = nil, error = custom_errors.storageConflictErrors.TransferAlReadyExist }
	end

	transfer.amount = decimal.new(transfer.amount)

	box.space.transfers:insert({
		uuid.fromstr(transfer.transfer_id),
		transfer.src_bic,
		transfer.src_ban,
		transfer.dest_bic,
		transfer.dest_ban,
		transfer.state,
		nil,
		nil,
		transfer.amount,
		transfer.bucket_id,
	})

	return { result = transfer, error = nil }
end

local function fetch_total()
	local totalBalance = box.space.checksum:select()
	log.debug(totalBalance)
	if #totalBalance < 1 then
		return { ok = false, error = custom_errors.storageNotFoundErrors.totalBalanceNotFound }
	end

	return totalBalance
end

local function persist_total(total)
	log.debug(total)
	box.space.checksum:replace({ "total", decimal.new(total.total) })
	return { ok = true, error = nil }
end

local function calculate_accounts_balance()
	local sum = decimal.new(0)
	for _, t in box.space.accounts:pairs() do
		sum = sum + decimal.new(t[3])
	end
	return decimal.new(sum)
end

local function insert_settings(settings)
	log.debug(settings)
	for key, value in pairs(settings) do
		-- Проверяем на дубликаты
		local exist = box.space.settings:get({ key })
		if exist ~= nil then
			return { ok = false, error = custom_errors.storageConflictErrors.SetingsAlreadyExist }
		end

		box.space.settings:insert({ key, value })
	end

	return { ok = true, error = nil }
end

local function fetch_settings()
	local settings = box.space.settings:select()
	log.debug(settings)
	if settings == nil then
		return { ok = false, error = custom_errors.storageNotFoundErrors.settingsNotFound }
	elseif #settings < 2 then
		return { ok = false, error = custom_errors.storageConflictErrors.SettingsIncorrectCount }
	end

	return settings
end

local function update_transfer(transfer)
	log.info({ "storage: update_transfer: got transfer:", transfer })
	-- Проверяем, есть ли счет
	local current_transfer = box.space.transfers:get({ uuid.fromstr(transfer.transfer_id) })
	if current_transfer == nil then
		return { ok = false, error = custom_errors.storageNotFoundErrors.TransferNotFound }
	end

	box.space.transfers:update(
		uuid.fromstr(transfer.transfer_id),
		{ { "=", 7, uuid.fromstr(transfer.client_id) }, { "=", 8, fiber.time64() / 1e4 } }
	)

	return { ok = true, error = nil }
end

local function fetch_account_balance(account_attr)
	local received_account = box.space.accounts:get({ account_attr.bic, account_attr.ban })
	log.debug(received_account)
	if received_account == nil then
		return { result = nil, error = custom_errors.storageNotFoundErrors.AccNotFound }
	end

	return { result = { received_account.balance, received_account.pending_amount }, error = nil }
end

local function lock_account(account)
	-- Проверяем, есть ли счет
	local current_account = box.space.accounts:get({ account.bic, account.ban })
	if current_account == nil then
		return { ok = false, error = custom_errors.storageNotFoundErrors.AccNotFound }
	end
	-- аналогично lockAccount в https://github.com/picodata/stroppy/blob/develop/pkg/database/cluster/pgSqlConstants.go#L105
	if current_account.pending_transfer ~= nil then
		account.pending_transfer = current_account.pending_transfer
	end

	if current_account.pending_amount ~= 0 then
		account.pending_amount = current_account.pending_amount
	end

	box.space.accounts:update(
		{ current_account.bic, current_account.ban },
		{ { "=", 4, account.pending_transfer }, { "=", 5, account.pending_amount } }
	)

	return { ok = true, error = nil }
end

local function unlock_account(account)
	-- Проверяем, есть ли счет
	local current_account = box.space.accounts:get({ account.bic, account.ban })
	if current_account == nil then
		return { ok = false, error = custom_errors.storageNotFoundErrors.AccNotFound }
	end
	-- аналогично lockAccount в https://github.com/picodata/stroppy/blob/develop/pkg/database/cluster/pgSqlConstants.go#L105
	if current_account.pending_transfer == account.pending_transfer then
		box.space.accounts:update(
			{ current_account.bic, current_account.ban },
			{ { "=", 4, nil }, { "=", 5, 0 } }
		)
	end

	return { ok = true, error = nil }
end

local function init(opts)
	if opts.is_master then
		-- cоздаем спейсы, если не созданы
		local accounts = box.schema.space.create("accounts", { if_not_exists = true })
		accounts:format({
			{ name = "bic", type = "string" },
			{ name = "ban", type = "string" },
			{ name = "balance", type = "decimal" },
			{ name = "pending_transfer", type = "uuid", is_nullable = true },
			{ name = "pending_amount", type = "decimal" },
			{ name = "bucket_id", type = "unsigned" },
		})
		accounts:create_index("primary", { parts = { { field = "bic" }, { field = "ban" } }, if_not_exists = true })
		accounts:create_index(
			"bucket_id",
			{ parts = { { field = "bucket_id" } }, unique = false, if_not_exists = true }
		)

		local transfers = box.schema.space.create("transfers", { if_not_exists = true })
		transfers:format({
			{ name = "transfer_id", type = "uuid" },
			{ name = "src_bic", type = "string" },
			{ name = "src_ban", type = "string" },
			{ name = "dest_bic", type = "string" },
			{ name = "dest_ban", type = "string" },
			{ name = "state", type = "string" },
			{ name = "client_id", type = "uuid", is_nullable = true },
			{ name = "client_timestamp", type = "scalar", is_nullable = true },
			{ name = "amount", type = "decimal" },

			{ name = "bucket_id", type = "unsigned" },
		})
		transfers:create_index("primary", { parts = { { field = "transfer_id" } }, if_not_exists = true })
		transfers:create_index(
			"bucket_id",
			{ parts = { { field = "bucket_id" } }, unique = false, if_not_exists = true }
		)

		local settings = box.schema.space.create("settings", { if_not_exists = true })
		settings:format({
			{ name = "key", type = "string" },
			{ name = "value", type = "number" },
		})
		settings:create_index("primary", { parts = { { field = "key" } }, if_not_exists = true })

		local checksum = box.schema.space.create("checksum", { if_not_exists = true })
		checksum:format({
			{ name = "name", type = "string" },
			{ name = "amount", type = "decimal" },
		})
		checksum:create_index("primary", { parts = { { field = "amount" } }, if_not_exists = true })

		box.schema.func.create("account_add", { if_not_exists = true })
		box.schema.func.create("account_balance_update", { if_not_exists = true })
		box.schema.func.create("transfer_add", { if_not_exists = true })
		box.schema.func.create("fetch_total", { if_not_exists = true })
		box.schema.func.create("persist_total", { if_not_exists = true })
		box.schema.func.create("calculate_accounts_balance", { if_not_exists = true })
		box.schema.func.create("insert_settings", { if_not_exists = true })
		box.schema.func.create("fetch_settings", { if_not_exists = true })
		box.schema.func.create("insert_transfer", { if_not_exists = true })
		box.schema.func.create("update_transfer", { if_not_exists = true })
		box.schema.func.create("fetch_account_balance", { if_not_exists = true })
		box.schema.func.create("lock_account", { if_not_exists = true })
        box.schema.func.create("unlock_account", { if_not_exists = true })
		rawset(_G, "account_add", account_add)
		rawset(_G, "account_balance_update", account_balance_update)
		rawset(_G, "transfer_add", insert_transfer)
		rawset(_G, "fetch_total", fetch_total)
		rawset(_G, "persist_total", persist_total)
		rawset(_G, "calculate_accounts_balance", calculate_accounts_balance)
		rawset(_G, "insert_settings", insert_settings)
		rawset(_G, "fetch_settings", fetch_settings)
		rawset(_G, "insert_transfer", insert_transfer)
		rawset(_G, "update_transfer", update_transfer)
		rawset(_G, "fetch_account_balance", fetch_account_balance)
		rawset(_G, "lock_account", lock_account)
        rawset(_G, "unlock_account", lock_account)
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

local function stop() end

return {
	role_name = "storage",

	init = init,
	validate_config = validate_config,
	apply_config = apply_config,
	stop = stop,
	utils = {
		account_add = account_add,
		account_balance_update = account_balance_update,
		fetch_total = fetch_total,
		persist_total = persist_total,
		calculate_accounts_balance = calculate_accounts_balance,
		insert_settings = insert_settings,
		fetch_settings = fetch_settings,
		insert_transfer = insert_transfer,
		update_transfer = update_transfer,
		fetch_account_balance = fetch_account_balance,
		lock_account = lock_account,
        unlock_account = unlock_account
	},
	dependencies = { "cartridge.roles.vshard-storage" },
}
